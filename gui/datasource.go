package main

import (
	"fmt"
	"strings"
	"sync"
)

// DataSource supplies the GUI with data and applies mutations. Two implementations:
//   - mockSource: in-memory fixtures, so the app runs standalone (no daemon).
//   - socketSource: real HTTP over the daemon's unix socket (added with the
//     SSH-tunnel transport). Selected at startup by whether a socket is set.
type DataSource interface {
	Dashboard() (Dashboard, error)
	TailnetDetail(id string) (TailnetDetail, error)
	AddTailnet(req AddTailnetRequest) error
	RemoveTailnet(id string) error
}

// mockSource holds mutable in-memory state so add/remove actually change what
// the dashboard shows — the app is a faithful, daemon-less demo of the flow.
type mockSource struct {
	mu       sync.Mutex
	next     int
	tailnets []TailnetRow
	events   []Event
}

func newMockSource() *mockSource {
	return &mockSource{
		next: 5,
		tailnets: []TailnetRow{
			{ID: "jbones", Namespace: "mars-1", Status: "connected", Address: "100.72.247.43", Peers: 6, HostAccess: "enabled"},
			{ID: "havoc", Namespace: "mars-2", Status: "connected", Address: "100.121.171.43", Peers: 5, HostAccess: "enabled"},
			{ID: "homelab", Namespace: "mars-4", Status: "connected", Address: "100.98.12.7", Peers: 3, HostAccess: "off"},
			{ID: "corp-prod", Namespace: "mars-3", Status: "reconnecting", Address: "reauthenticating…", Peers: 0, HostAccess: "enabled", ExitNode: "us-nyc"},
		},
		events: []Event{
			{Time: "16:47:22", Kind: "ok", Message: "reconcile complete · 2 actions"},
			{Time: "16:47:20", Kind: "ok", Tailnet: "havoc", Message: "authenticated mars-2"},
			{Time: "16:47:19", Kind: "dns", Message: "host DNS synced · 14 peers"},
			{Time: "16:47:18", Kind: "skip", Tailnet: "jbones", Message: "route 192.168.1.0/24 would clobber host LAN"},
			{Time: "16:47:18", Kind: "route", Tailnet: "jbones", Message: "+ 100.114.149.115"},
			{Time: "16:47:11", Kind: "ns", Tailnet: "corp-prod", Message: "namespace created"},
			{Time: "16:47:04", Kind: "boot", Message: "daemon start · reconcile 10s"},
		},
	}
}

func (m *mockSource) Dashboard() (Dashboard, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	peers, haOn, healthy, reconnecting := 0, 0, 0, 0
	for _, t := range m.tailnets {
		peers += t.Peers
		if t.HostAccess == "enabled" {
			haOn++
		}
		if t.Status == "connected" {
			healthy++
		} else {
			reconnecting++
		}
	}
	return Dashboard{
		Host:    "mars",
		Healthy: healthy,
		DNSOK:   true,
		Version: "v0.8.2",
		Metrics: Metrics{
			Tailnets: len(m.tailnets), Reconnecting: reconnecting, Peers: peers,
			HostAccessOn: haOn, ReconcileSec: 10, Uptime: "2:59",
		},
		Tailnets: append([]TailnetRow(nil), m.tailnets...),
		Events:   append([]Event(nil), m.events...),
	}, nil
}

func (m *mockSource) AddTailnet(req AddTailnetRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		return fmt.Errorf("tailnet name is required")
	}
	for _, t := range m.tailnets {
		if t.ID == req.ID {
			return fmt.Errorf("tailnet %q already exists", req.ID)
		}
	}
	ha := "off"
	if req.HostAccess {
		ha = "enabled"
	}
	ns := fmt.Sprintf("mars-%d", m.next)
	m.next++
	m.tailnets = append(m.tailnets, TailnetRow{
		ID: req.ID, Namespace: ns, Status: "reconnecting",
		Address: "connecting…", Peers: 0, HostAccess: ha, ExitNode: req.ExitNode,
	})
	m.pushEvent(Event{Time: "now", Kind: "ns", Tailnet: req.ID, Message: "namespace " + ns + " created"})
	return nil
}

func (m *mockSource) RemoveTailnet(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := m.tailnets[:0]
	found := false
	for _, t := range m.tailnets {
		if t.ID == id {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		return fmt.Errorf("tailnet %q not found", id)
	}
	m.tailnets = out
	m.pushEvent(Event{Time: "now", Kind: "ns", Tailnet: id, Message: "namespace removed"})
	return nil
}

// pushEvent prepends an event; caller holds the lock.
func (m *mockSource) pushEvent(e Event) {
	m.events = append([]Event{e}, m.events...)
	if len(m.events) > 12 {
		m.events = m.events[:12]
	}
}

func (m *mockSource) TailnetDetail(id string) (TailnetDetail, error) {
	m.mu.Lock()
	var row *TailnetRow
	for i := range m.tailnets {
		if m.tailnets[i].ID == id {
			row = &m.tailnets[i]
			break
		}
	}
	m.mu.Unlock()
	if row == nil {
		return TailnetDetail{ID: id, Error: "tailnet not found"}, nil
	}

	magic := id + ".ts.net"
	ha := []KV{{Key: "Host access", Value: "disabled", Dim: true}}
	if row.HostAccess == "enabled" {
		ha = []KV{
			{Key: "veth", Value: "10.88.1.1/30"},
			{Key: "Allowed LAN", Value: "192.168.1.0/24"},
			{Key: "NAT", Value: "masquerade"},
			{Key: "Guard", Value: "host-route safe"},
		}
	}
	exit := KV{Key: "Exit node", Value: "—", Dim: true}
	if row.ExitNode != "" {
		exit = KV{Key: "Exit node", Value: row.ExitNode}
	}
	return TailnetDetail{
		ID: row.ID, Namespace: "ns-" + row.ID, Status: row.Status,
		Address: row.Address, PeerCount: row.Peers, Uptime: "2:41",
		Network: []KV{
			{Key: "Tailscale IPv4", Value: row.Address},
			{Key: "Tailscale IPv6", Value: "fd7a:1c…:a3f2", Dim: true},
			{Key: "MagicDNS name", Value: magic},
			{Key: "Control", Value: "login.tailscale.com"},
		},
		DNS: []KV{
			{Key: "Upstream", Value: "100.100.100.100"},
			{Key: "Search domain", Value: magic},
			{Key: "Host resolv", Value: "synced"},
			{Key: "Split routes", Value: "1"},
		},
		HostAccess: ha,
		Routes: []KV{
			{Key: "Advertised", Value: "—", Dim: true},
			{Key: "Accepted", Value: "100.114.149.115"},
			{Key: "Subnet", Value: "none", Dim: true},
			exit,
		},
		Peers: []Peer{
			{HostName: "orin-brain", Address: "100.114.149.115", OS: "linux", Routes: "100.114.149.115/32", LastSeen: "now", Status: "good"},
			{HostName: "phobos", Address: "100.72.19.4", OS: "linux", LastSeen: "now", Status: "good"},
			{HostName: "pixel-7", Address: "100.88.203.51", OS: "android", LastSeen: "now", Status: "good"},
			{HostName: "mbp-work", Address: "100.102.7.19", OS: "macOS", LastSeen: "3m", Status: "good"},
			{HostName: "nas-attic", Address: "100.66.140.8", OS: "linux", Routes: "192.168.1.0/24", LastSeen: "12m", Status: "warn"},
			{HostName: "old-laptop", Address: "100.75.11.2", OS: "windows", LastSeen: "2d", Status: "dim"},
		},
		Events: []Event{
			{Time: "16:47:18", Kind: "route", Message: "+ 100.114.149.115"},
			{Time: "16:47:18", Kind: "skip", Message: "route 192.168.1.0/24 would clobber host LAN"},
			{Time: "16:44:02", Kind: "dns", Message: "MagicDNS synced · " + fmt.Sprint(row.Peers) + " peers"},
			{Time: "16:44:01", Kind: "ok", Message: "authenticated " + row.Namespace},
			{Time: "16:44:00", Kind: "ns", Message: "namespace ns-" + row.ID + " up"},
		},
	}, nil
}
