package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// socketSource is the live DataSource: it speaks the daemon's HTTP API over a
// unix socket. For a remote (macOS/Windows) client the socket is an ssh -L
// forward of the daemon's /var/lib/hydrascale/api.sock; locally on Linux it is
// that path directly. It maps the daemon's JSON onto the GUI's view-model DTOs.
type socketSource struct {
	http  *http.Client
	cache *diskCache // last-known-good snapshot; nil disables caching (used in mapping tests)
	key   string     // connection identity the cache is tagged with
}

func newSocketSource(socketPath string) *socketSource {
	return &socketSource{
		http: &http.Client{
			Timeout: 12 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

// The host in the URL is ignored — the transport always dials the unix socket.
const sockBase = "http://unix"

func (s *socketSource) get(path string, out interface{}) error {
	resp, err := s.http.Get(sockBase + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *socketSource) post(path string, body, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := s.http.Post(sockBase+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s: HTTP %d", path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ---- daemon JSON shapes (only the fields the GUI needs) ----

type apiConfig struct {
	Config struct {
		Tailnets []struct {
			ID         string `json:"id"`
			ExitNode   string `json:"exit_node"`
			HostAccess *bool  `json:"host_access"`
		} `json:"tailnets"`
	} `json:"config"`
}

type apiStatus struct {
	ServerVersion string          `json:"server_version"`
	ErrorStates   map[string]bool `json:"error_states"`
	PausedStates  map[string]bool `json:"paused_states"`
	// Actual/TailnetState is marshaled with Go field names (no json tags).
	Actual map[string]struct {
		NsName        string `json:"NsName"`
		DaemonHealthy bool   `json:"DaemonHealthy"`
		Routes        []struct {
			Network string `json:"Network"`
		} `json:"Routes"`
	} `json:"actual"`
}

type apiPeer struct {
	HostName     string    `json:"host_name"`
	DNSName      string    `json:"dns_name"`
	OS           string    `json:"os"`
	TailscaleIPs []string  `json:"tailscale_ips"`
	AllowedIPs   []string  `json:"allowed_ips"`
	Online       bool      `json:"online"`
	LastSeen     time.Time `json:"last_seen"`
}

type apiDetail struct {
	TailscaleIPs   []string  `json:"tailscale_ips"`
	MagicDNSName   string    `json:"magic_dns_name"`
	MagicDNSSuffix string    `json:"magic_dns_suffix"`
	PeerCount      int       `json:"peer_count"`
	OnlinePeers    int       `json:"online_peers"`
	Peers          []apiPeer `json:"peers"`
	Error          string    `json:"error"`
}

type apiEvents struct {
	Events []struct {
		Time      time.Time `json:"Time"`
		Type      string    `json:"Type"`
		TailnetID string    `json:"TailnetID"`
		Message   string    `json:"Message"`
	} `json:"events"`
}

// ---- DataSource implementation ----

func (s *socketSource) Dashboard() (Dashboard, error) {
	var cfg apiConfig
	if err := s.get("/api/config", &cfg); err != nil {
		return s.staleDashboard(err)
	}
	var st apiStatus
	_ = s.get("/api/status", &st) // health is best-effort; config drives the list

	// Fetch each tailnet's live detail (address, peer count) concurrently.
	type detres struct {
		id  string
		d   apiDetail
		err error
	}
	results := make([]detres, len(cfg.Config.Tailnets))
	var wg sync.WaitGroup
	for i, tn := range cfg.Config.Tailnets {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			var d apiDetail
			err := s.get("/api/tailnet/"+id+"/detail", &d)
			results[i] = detres{id: id, d: d, err: err}
		}(i, tn.ID)
	}
	wg.Wait()

	rows := make([]TailnetRow, 0, len(cfg.Config.Tailnets))
	peersTotal, haOn, healthy, reconnecting := 0, 0, 0, 0
	for i, tn := range cfg.Config.Tailnets {
		d := results[i].d
		status := "connected"
		switch {
		case st.PausedStates[tn.ID]:
			status = "off"
		case st.ErrorStates[tn.ID] || results[i].err != nil || d.Error != "" || !st.Actual[tn.ID].DaemonHealthy:
			status = "reconnecting"
		}
		if status == "connected" {
			healthy++
		} else if status == "reconnecting" {
			reconnecting++
		}
		addr := "—"
		if len(d.TailscaleIPs) > 0 {
			addr = d.TailscaleIPs[0]
		} else if status == "reconnecting" {
			addr = "reauthenticating…"
		}
		ha := "off"
		if tn.HostAccess != nil && *tn.HostAccess {
			ha = "enabled"
			haOn++
		}
		peersTotal += d.PeerCount
		rows = append(rows, TailnetRow{
			ID: tn.ID, Namespace: st.Actual[tn.ID].NsName, Status: status,
			Address: addr, Peers: d.PeerCount, HostAccess: ha, ExitNode: tn.ExitNode,
		})
	}

	d := Dashboard{
		Host:    hostName(st),
		Healthy: healthy,
		DNSOK:   true,
		Version: st.ServerVersion,
		Metrics: Metrics{
			Tailnets: len(rows), Reconnecting: reconnecting, Peers: peersTotal,
			HostAccessOn: haOn, ReconcileSec: 10, Uptime: "",
		},
		Tailnets:  rows,
		Events:    s.events(),
		Connected: true,
	}
	if s.cache != nil {
		s.cache.putDashboard(s.key, d, time.Now())
	}
	return d, nil
}

// staleDashboard is returned when the daemon is unreachable: the last-known
// snapshot stamped stale, or — with nothing cached — an empty "not connected"
// dashboard carrying the underlying error so the UI can show it.
func (s *socketSource) staleDashboard(cause error) (Dashboard, error) {
	if s.cache != nil {
		if cf := s.cache.forKey(s.key); cf.Dashboard != nil {
			d := *cf.Dashboard
			d.Connected = false
			d.Stale = true
			d.LastSeen = lastSeenLabel(cf.DashAt)
			return d, nil
		}
	}
	return Dashboard{Connected: false}, cause
}

// hostName derives the host label from a namespace name like "mars-1" → "mars".
func hostName(st apiStatus) string {
	for _, a := range st.Actual {
		if i := strings.LastIndex(a.NsName, "-"); i > 0 {
			return a.NsName[:i]
		}
	}
	return ""
}

func (s *socketSource) events() []Event {
	var ev apiEvents
	if err := s.get("/api/events", &ev); err != nil {
		return nil
	}
	out := make([]Event, 0, len(ev.Events))
	for i := len(ev.Events) - 1; i >= 0 && len(out) < 12; i-- { // newest first
		e := ev.Events[i]
		out = append(out, Event{
			Time:    e.Time.Format("15:04:05"),
			Kind:    eventKind(e.Type),
			Tailnet: e.TailnetID,
			Message: e.Message,
		})
	}
	return out
}

func eventKind(t string) string {
	t = strings.ToLower(t)
	switch {
	case strings.Contains(t, "fail"), strings.Contains(t, "skip"), strings.Contains(t, "error"):
		return "skip"
	case strings.Contains(t, "dns"):
		return "dns"
	case strings.Contains(t, "route"):
		return "route"
	case strings.Contains(t, "ns"), strings.Contains(t, "namespace"):
		return "ns"
	case strings.Contains(t, "boot"), strings.Contains(t, "start"):
		return "boot"
	default:
		return "ok"
	}
}

func (s *socketSource) TailnetDetail(id string) (TailnetDetail, error) {
	var d apiDetail
	if err := s.get("/api/tailnet/"+id+"/detail", &d); err != nil {
		return s.staleDetail(id, err), nil
	}
	if d.Error != "" {
		return TailnetDetail{ID: id, Error: d.Error}, nil
	}

	var cfg apiConfig
	_ = s.get("/api/config", &cfg)
	control, exitNode := "login.tailscale.com", ""
	var hostAccess *bool
	for _, tn := range cfg.Config.Tailnets {
		if tn.ID == id {
			exitNode = tn.ExitNode
			hostAccess = tn.HostAccess
		}
	}

	var st apiStatus
	_ = s.get("/api/status", &st)

	ipv4, ipv6 := "—", ""
	if len(d.TailscaleIPs) > 0 {
		ipv4 = d.TailscaleIPs[0]
	}
	if len(d.TailscaleIPs) > 1 {
		ipv6 = d.TailscaleIPs[1]
	}
	magic := d.MagicDNSName
	if magic == "" {
		magic = d.MagicDNSSuffix
	}

	network := []KV{
		{Key: "Tailscale IPv4", Value: ipv4},
		{Key: "Tailscale IPv6", Value: emptyDash(ipv6), Dim: ipv6 == ""},
		{Key: "MagicDNS name", Value: emptyDash(magic), Dim: magic == ""},
		{Key: "Control", Value: control},
	}
	dns := []KV{
		{Key: "Upstream", Value: "100.100.100.100"},
		{Key: "Search domain", Value: emptyDash(d.MagicDNSSuffix), Dim: d.MagicDNSSuffix == ""},
		{Key: "Accept DNS", Value: "yes"},
	}
	var ha []KV
	if hostAccess != nil && *hostAccess {
		ha = []KV{{Key: "Host access", Value: "enabled"}, {Key: "veth", Value: "provisioned"}}
	} else {
		ha = []KV{{Key: "Host access", Value: "disabled", Dim: true}}
	}
	routes := []KV{}
	for _, r := range st.Actual[id].Routes {
		routes = append(routes, KV{Key: "Route", Value: r.Network})
	}
	if len(routes) == 0 {
		routes = append(routes, KV{Key: "Accepted", Value: "none", Dim: true})
	}
	routes = append(routes, KV{Key: "Exit node", Value: emptyDash(exitNode), Dim: exitNode == ""})

	peers := make([]Peer, 0, len(d.Peers))
	for _, p := range d.Peers {
		peers = append(peers, Peer{
			HostName: p.HostName,
			Address:  first(p.TailscaleIPs),
			OS:       p.OS,
			Routes:   strings.Join(p.AllowedIPs, ", "),
			LastSeen: lastSeen(p.Online, p.LastSeen),
			Status:   peerStatus(p.Online, p.LastSeen),
		})
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].HostName < peers[j].HostName })

	nsName := st.Actual[id].NsName
	status := "connected"
	if st.ErrorStates[id] || !st.Actual[id].DaemonHealthy {
		status = "reconnecting"
	}
	detail := TailnetDetail{
		ID: id, Namespace: emptyOr(nsName, "ns-"+id), Status: status,
		Address: ipv4, PeerCount: d.PeerCount, Uptime: "",
		Network: network, DNS: dns, HostAccess: ha, Routes: routes,
		Peers: peers, Events: s.events(),
	}
	if s.cache != nil {
		s.cache.putDetail(s.key, id, detail, time.Now())
	}
	return detail, nil
}

// staleDetail is returned when the daemon is unreachable while opening a detail
// view: the last-known detail stamped stale, or the error if nothing is cached.
func (s *socketSource) staleDetail(id string, cause error) TailnetDetail {
	if s.cache != nil {
		if cf := s.cache.forKey(s.key); cf.Details != nil {
			if cd, ok := cf.Details[id]; ok {
				d := cd.Detail
				d.Stale = true
				d.LastSeen = lastSeenLabel(cd.At)
				return d
			}
		}
	}
	return TailnetDetail{ID: id, Error: cause.Error()}
}

func (s *socketSource) AddTailnet(req AddTailnetRequest) error {
	body := map[string]interface{}{
		"id":          req.ID,
		"auth_key":    req.AuthKey,
		"exit_node":   req.ExitNode,
		"control_url": req.ControlURL,
		"host_access": req.HostAccess,
	}
	var res struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	if err := s.post("/api/tailnet/add", body, &res); err != nil {
		return err
	}
	if !res.OK {
		return fmt.Errorf("%s", res.Message)
	}
	return nil
}

func (s *socketSource) RemoveTailnet(id string) error {
	var res struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	if err := s.post("/api/tailnet/remove", map[string]string{"id": id}, &res); err != nil {
		return err
	}
	if !res.OK {
		return fmt.Errorf("%s", res.Message)
	}
	return nil
}

// ---- small helpers ----

func first(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return "—"
}
func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
func emptyOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
func peerStatus(online bool, last time.Time) string {
	if online {
		return "good"
	}
	if !last.IsZero() && time.Since(last) < time.Hour {
		return "warn"
	}
	return "dim"
}
func lastSeen(online bool, last time.Time) string {
	if online {
		return "now"
	}
	if last.IsZero() {
		return "—"
	}
	d := time.Since(last)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
