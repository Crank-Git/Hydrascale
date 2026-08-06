package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/dns"
	"hydrascale/internal/reconciler"
)

// unixHTTPClient returns an HTTP client that reaches the control socket of srv.
func unixHTTPClient(srv *Server) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", srv.socketPath)
		},
	}
	return &http.Client{Transport: transport}
}

// getDNS calls GET /api/dns and returns the decoded body.
func getDNS(t *testing.T, srv *Server) DNSResponse {
	t.Helper()
	resp, err := unixHTTPClient(srv).Get("http://localhost/api/dns")
	if err != nil {
		t.Fatalf("GET /api/dns: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body DNSResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return body
}

// namespaceState returns the entry for id, and false when the body holds no such entry.
func namespaceState(body DNSResponse, id string) (DNSNamespaceState, bool) {
	for _, ns := range body.Namespaces {
		if ns.ID == id {
			return ns, true
		}
	}
	return DNSNamespaceState{}, false
}

// writeHostFile writes a resolv.conf file into a temporary directory and returns the path.
func writeHostFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write host file: %v", err)
	}
	return path
}

func TestDNSEndpoint_returns_the_checksum_and_the_protected_state_of_each_namespace(t *testing.T) {
	hostPath := writeHostFile(t, "nameserver 127.0.0.53\n")
	cfgPath := writeTestConfig(t, "jbones", "corp")
	r := reconciler.New(cfgPath, newMockNS(), newMockDaemon(), &mockRouting{}, time.Second, nil, "10.200.0.0/16")
	r.SetHostFileMonitor(dns.NewHostFileMonitor(hostPath))

	if err := r.Reconcile(); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	// The second reconcile follows a change, so the monitor records a time of change.
	if err := os.WriteFile(hostPath, []byte("nameserver 100.100.100.100\n"), 0o644); err != nil {
		t.Fatalf("rewrite host file: %v", err)
	}
	if err := r.Reconcile(); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}

	srv, _, cleanup := startTestServer(t, r)
	defer cleanup()

	body := getDNS(t, srv)

	state, _ := r.HostFileMonitor().State()
	if body.HostResolvSHA256 != state.Checksum {
		t.Errorf("host_resolv_sha256 = %q, want %q", body.HostResolvSHA256, state.Checksum)
	}
	if body.HostResolvSHA256 == "" {
		t.Error("host_resolv_sha256 is empty, want the checksum of the host file")
	}
	if body.HostResolvChangedAt == "" {
		t.Error("host_resolv_changed_at is empty, want the time of the change")
	} else if _, err := time.Parse(time.RFC3339, body.HostResolvChangedAt); err != nil {
		t.Errorf("host_resolv_changed_at = %q, want an RFC 3339 time", body.HostResolvChangedAt)
	}

	if body.BindAddress == "" {
		t.Error("bind_address is empty, want the address of the resolver")
	}
	if body.Mode != "unified" {
		t.Errorf("mode = %q, want %q", body.Mode, "unified")
	}
	if body.Upstreams == nil {
		t.Error("upstreams is null, want a list")
	}

	if len(body.Namespaces) != 2 {
		t.Fatalf("len(namespaces) = %d, want 2", len(body.Namespaces))
	}
	for _, id := range []string{"jbones", "corp"} {
		ns, ok := namespaceState(body, id)
		if !ok {
			t.Fatalf("namespaces holds no entry for %q", id)
		}
		if !ns.Protected {
			t.Errorf("protected of %q = false, want true", id)
		}
		if ns.Error != "" {
			t.Errorf("error of %q = %q, want an empty string", id, ns.Error)
		}
	}
}

func TestDNSEndpoint_reports_protected_false_and_the_error_of_a_failed_overlay_mount(t *testing.T) {
	const reason = "overlay /etc failed: invalid argument"

	hostPath := writeHostFile(t, "nameserver 127.0.0.53\n")
	cfgPath := writeTestConfig(t, "jbones", "corp")
	dm := newMockDaemon()
	dm.unprotected["corp"] = daemon.UnprotectedRecord{Reason: reason}
	r := reconciler.New(cfgPath, newMockNS(), dm, &mockRouting{}, time.Second, nil, "10.200.0.0/16")
	r.SetHostFileMonitor(dns.NewHostFileMonitor(hostPath))

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	srv, _, cleanup := startTestServer(t, r)
	defer cleanup()

	body := getDNS(t, srv)

	corp, ok := namespaceState(body, "corp")
	if !ok {
		t.Fatal("namespaces holds no entry for \"corp\"")
	}
	if corp.Protected {
		t.Error("protected of \"corp\" = true, want false")
	}
	if corp.Error != reason {
		t.Errorf("error of \"corp\" = %q, want %q", corp.Error, reason)
	}

	jbones, ok := namespaceState(body, "jbones")
	if !ok {
		t.Fatal("namespaces holds no entry for \"jbones\"")
	}
	if !jbones.Protected {
		t.Error("protected of \"jbones\" = false, want true")
	}
	if jbones.Error != "" {
		t.Errorf("error of \"jbones\" = %q, want an empty string", jbones.Error)
	}
}

func TestDNSEndpoint_returns_a_valid_body_when_no_namespace_runs(t *testing.T) {
	cfgPath := writeTestConfig(t)
	r := newTestReconciler(cfgPath)
	srv, _, cleanup := startTestServer(t, r)
	defer cleanup()

	body := getDNS(t, srv)

	if body.Namespaces == nil {
		t.Error("namespaces is null, want an empty list")
	}
	if len(body.Namespaces) != 0 {
		t.Errorf("len(namespaces) = %d, want 0", len(body.Namespaces))
	}
	if body.Upstreams == nil {
		t.Error("upstreams is null, want a list")
	}
	if body.BindAddress == "" {
		t.Error("bind_address is empty, want the address of the resolver")
	}
	if body.HostResolvChangedAt != "" {
		t.Errorf("host_resolv_changed_at = %q, want an empty string", body.HostResolvChangedAt)
	}
}

func TestTheDNSRouteReportsTheHostFilePathAndTheAllowUnprotectedKey(t *testing.T) {
	// The DNS view states the path that the daemon watches, and it states whether the
	// operator opted out of protection. Issue #76 settled that an unprotected namespace is
	// an error state only when dns.allow_unprotected is false, so the view cannot read the
	// protection state alone.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.DefaultConfig()
	cfg.DNS.AllowUnprotected = true
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	r := newTestReconciler(cfgPath)
	hostPath := writeHostFile(t, "nameserver 127.0.0.53\n")
	monitor := dns.NewHostFileMonitor(hostPath)
	if _, err := monitor.Start(); err != nil {
		t.Fatalf("Start the host file monitor: %v", err)
	}
	r.SetHostFileMonitor(monitor)

	srv, _, cleanup := startTestServer(t, r)
	defer cleanup()

	body := getDNS(t, srv)
	if !body.AllowUnprotected {
		t.Error("allow_unprotected = false, want true")
	}
	if body.HostResolvPath != hostPath {
		t.Errorf("host_resolv_path = %q, want %q", body.HostResolvPath, hostPath)
	}
}

func TestDNSEndpoint_rejects_a_method_other_than_get(t *testing.T) {
	cfgPath := writeTestConfig(t)
	r := newTestReconciler(cfgPath)
	srv, _, cleanup := startTestServer(t, r)
	defer cleanup()

	resp, err := unixHTTPClient(srv).Post("http://localhost/api/dns", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/dns: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}
