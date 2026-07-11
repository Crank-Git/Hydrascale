package main

import (
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDaemon serves canned API JSON over a unix socket, matching the daemon's
// real endpoint shapes, so we can verify socketSource's mapping on any OS.
func fakeDaemon(t *testing.T) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "api.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"config":{"version":1,"tailnets":[
			{"id":"alpha","exit_node":"","host_access":true},
			{"id":"beta","exit_node":"us-nyc","host_access":false}]}}`))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error_states":{"beta":true},"paused_states":{},
			"actual":{"alpha":{"NsName":"mars-1","DaemonHealthy":true,"Routes":[]},
			          "beta":{"NsName":"mars-2","DaemonHealthy":false,"Routes":[]}}}`))
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"events":[
			{"Time":"2026-07-09T16:44:00Z","Type":"namespace_created","TailnetID":"alpha","Message":"ns up"},
			{"Time":"2026-07-09T16:47:22Z","Type":"reconcile_complete","TailnetID":"","Message":"reconcile complete"}]}`))
	})
	mux.HandleFunc("/api/tailnet/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/tailnet/alpha/detail") {
			w.Write([]byte(`{"tailscale_ips":["100.64.1.5","fd7a::5"],"magic_dns_name":"alpha.tail.ts.net",
				"magic_dns_suffix":"tail.ts.net","peer_count":2,"online_peers":1,"peers":[
				{"host_name":"zeta","os":"linux","tailscale_ips":["100.64.1.9"],"allowed_ips":["100.64.1.9/32","192.168.1.0/24"],"online":true},
				{"host_name":"aardvark","os":"macOS","tailscale_ips":["100.64.1.2"],"online":false}]}`))
			return
		}
		// beta: daemon starting / no live status
		w.Write([]byte(`{"tailscale_ips":[],"peer_count":0,"online_peers":0,"error":"daemon starting"}`))
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return sock
}

func TestSocketSource_Dashboard(t *testing.T) {
	src := newSocketSource(fakeDaemon(t))
	d, err := src.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if len(d.Tailnets) != 2 {
		t.Fatalf("rows: got %d, want 2", len(d.Tailnets))
	}
	alpha := d.Tailnets[0]
	if alpha.ID != "alpha" || alpha.Status != "connected" || alpha.Address != "100.64.1.5" ||
		alpha.Peers != 2 || alpha.HostAccess != "enabled" {
		t.Errorf("alpha row wrong: %+v", alpha)
	}
	beta := d.Tailnets[1]
	if beta.Status != "reconnecting" || beta.HostAccess != "off" || beta.ExitNode != "us-nyc" {
		t.Errorf("beta row wrong: %+v", beta)
	}
	if d.Host != "mars" {
		t.Errorf("host: got %q, want mars", d.Host)
	}
	m := d.Metrics
	if m.Tailnets != 2 || m.Peers != 2 || m.HostAccessOn != 1 || m.Reconnecting != 1 {
		t.Errorf("metrics wrong: %+v", m)
	}
	if len(d.Events) != 2 || d.Events[0].Time != "16:47:22" || d.Events[0].Kind != "ok" {
		t.Errorf("events wrong: %+v", d.Events)
	}
	if d.Events[1].Kind != "ns" {
		t.Errorf("event kind mapping: got %q, want ns", d.Events[1].Kind)
	}
}

func TestSocketSource_TailnetDetail(t *testing.T) {
	src := newSocketSource(fakeDaemon(t))
	d, err := src.TailnetDetail("alpha")
	if err != nil {
		t.Fatalf("TailnetDetail: %v", err)
	}
	if d.Error != "" {
		t.Fatalf("unexpected error: %s", d.Error)
	}
	if d.Address != "100.64.1.5" || d.PeerCount != 2 {
		t.Errorf("detail top-line wrong: %+v", d)
	}
	if len(d.Peers) != 2 || d.Peers[0].HostName != "aardvark" || d.Peers[1].HostName != "zeta" {
		t.Fatalf("peers not sorted/mapped: %+v", d.Peers)
	}
	zeta := d.Peers[1]
	if zeta.OS != "linux" || zeta.Address != "100.64.1.9" || zeta.Routes != "100.64.1.9/32, 192.168.1.0/24" || zeta.LastSeen != "now" {
		t.Errorf("zeta peer wrong: %+v", zeta)
	}
	// host_access:true → an enabled card, not the disabled placeholder.
	if d.HostAccess[0].Value != "enabled" {
		t.Errorf("host access card: %+v", d.HostAccess)
	}
}

func TestSocketSource_DetailError(t *testing.T) {
	src := newSocketSource(fakeDaemon(t))
	d, _ := src.TailnetDetail("beta")
	if d.Error == "" {
		t.Error("expected error surfaced for beta (daemon starting)")
	}
}

// cachedPair builds a live source (against a fake daemon) and a "dead" source
// (dialing a socket that doesn't exist) that share one on-disk cache and key —
// modelling a reconnect after the daemon went away.
func cachedPair(t *testing.T, key string) (live, dead *socketSource) {
	t.Helper()
	cache := newDiskCache(filepath.Join(t.TempDir(), "cache.json"))
	live = newSocketSource(fakeDaemon(t))
	live.cache, live.key = cache, key
	dead = newSocketSource(filepath.Join(t.TempDir(), "gone.sock"))
	dead.cache, dead.key = cache, key
	return live, dead
}

func TestStaleDashboard(t *testing.T) {
	live, dead := cachedPair(t, "k1")
	if _, err := live.Dashboard(); err != nil {
		t.Fatalf("live Dashboard: %v", err)
	}
	d, err := dead.Dashboard()
	if err != nil {
		t.Fatalf("stale dashboard should not error: %v", err)
	}
	if !d.Stale || d.Connected {
		t.Errorf("want stale && !connected, got stale=%v connected=%v", d.Stale, d.Connected)
	}
	if len(d.Tailnets) != 2 {
		t.Errorf("stale dashboard should carry cached rows, got %d", len(d.Tailnets))
	}
	if d.LastSeen == "" {
		t.Error("stale dashboard needs a LastSeen label")
	}
}

func TestStaleDetail(t *testing.T) {
	live, dead := cachedPair(t, "k1")
	if _, err := live.TailnetDetail("alpha"); err != nil {
		t.Fatalf("live detail: %v", err)
	}
	d, _ := dead.TailnetDetail("alpha")
	if !d.Stale || d.Error != "" {
		t.Errorf("want stale detail with no error, got stale=%v err=%q", d.Stale, d.Error)
	}
	if len(d.Peers) != 2 || d.LastSeen == "" {
		t.Errorf("stale detail should carry cached peers + LastSeen: %+v", d)
	}
}

func TestNoCacheError(t *testing.T) {
	dead := newSocketSource(filepath.Join(t.TempDir(), "gone.sock")) // cache == nil
	d, err := dead.Dashboard()
	if err == nil {
		t.Fatal("unreachable daemon with no cache must return an error")
	}
	if d.Connected || d.Stale || len(d.Tailnets) != 0 {
		t.Errorf("no-cache failure must be empty+disconnected, got %+v", d)
	}
}

func TestCacheKeyMismatch(t *testing.T) {
	live, dead := cachedPair(t, "k1")
	if _, err := live.Dashboard(); err != nil {
		t.Fatalf("live Dashboard: %v", err)
	}
	dead.key = "different-daemon" // e.g. user re-pointed the app at another host
	_, err := dead.Dashboard()
	if err == nil {
		t.Error("cache from a different daemon key must not be served; expected error")
	}
}
