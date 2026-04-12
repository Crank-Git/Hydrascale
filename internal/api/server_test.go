package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/reconciler"
	"hydrascale/internal/routing"
)

// --- Mock implementations for testing ---

type mockNS struct {
	mu         sync.Mutex
	namespaces map[string]bool
}

func newMockNS() *mockNS {
	return &mockNS{namespaces: make(map[string]bool)}
}

func (m *mockNS) Create(tailnetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.namespaces[m.GetName(tailnetID)] = true
	return nil
}

func (m *mockNS) Delete(nsName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.namespaces, nsName)
	return nil
}

func (m *mockNS) List() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []string
	for ns := range m.namespaces {
		result = append(result, ns)
	}
	return result, nil
}

func (m *mockNS) GetName(tailnetID string) string {
	return namespaces.GetNamespaceName(tailnetID)
}

func (m *mockNS) GetTailnetID(nsName string) string {
	return namespaces.GetTailnetFromNamespace(nsName)
}

func (m *mockNS) SetupVeth(nsName string, index int) error { return nil }
func (m *mockNS) TeardownVeth(nsName string) error         { return nil }

type mockDaemon struct {
	mu           sync.Mutex
	healthy      map[string]bool
	statusResult *daemon.TailscaleStatus // returned by GetStatus; nil = daemon starting
	statusErr    error                   // returned by GetStatus; non-nil = error
}

func newMockDaemon() *mockDaemon {
	return &mockDaemon{healthy: make(map[string]bool)}
}

func (m *mockDaemon) Start(tailnetID, nsName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy[tailnetID] = true
	return nil
}

func (m *mockDaemon) Stop(nsName, tailnetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.healthy, tailnetID)
	return nil
}

func (m *mockDaemon) CheckHealth(nsName, tailnetID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.healthy[tailnetID], nil
}

func (m *mockDaemon) GetSocketPath(tailnetID string) string {
	return "/tmp/test-" + tailnetID + ".sock"
}

func (m *mockDaemon) AuthorizeDaemon(tailnetID, nsName, authKey, controlURL string) error { return nil }

func (m *mockDaemon) GetStatus(ctx context.Context, nsName, tailnetID string) (*daemon.TailscaleStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusResult, m.statusErr
}

type mockRouting struct{}

func (m *mockRouting) PollStatus(nsName, socketPath string) ([]routing.Route, error) {
	return nil, nil
}
func (m *mockRouting) SyncRoutes(nsName string, routes []routing.Route) error { return nil }
func (m *mockRouting) ListRoutes(nsName string) ([]string, error)             { return nil, nil }

// --- Helpers ---

func writeTestConfig(t *testing.T, tailnets ...string) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.DefaultConfig()
	for _, id := range tailnets {
		cfg.Tailnets = append(cfg.Tailnets, config.Tailnet{ID: id})
	}
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	return cfgPath
}

func newTestReconciler(cfgPath string) *reconciler.Reconciler {
	return reconciler.New(cfgPath, newMockNS(), newMockDaemon(), &mockRouting{}, 1*time.Second, nil)
}

// startTestServer starts a Server on a temp socket and returns the server, client, and cleanup func.
func startTestServer(t *testing.T, r *reconciler.Reconciler) (*Server, *Client, func()) {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "test-api.sock")
	srv := NewServer(socketPath, r)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	client := NewClient(socketPath)
	return srv, client, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}
}

// --- Tests ---

func TestStatusEndpoint(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha", "beta")
	r := newTestReconciler(cfgPath)
	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	resp, err := client.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// Desired state should have the two tailnets from config.
	if len(resp.Desired) != 2 {
		t.Errorf("len(Desired) = %d, want 2", len(resp.Desired))
	}
	if _, ok := resp.Desired["alpha"]; !ok {
		t.Error("missing 'alpha' in Desired")
	}
	if _, ok := resp.Desired["beta"]; !ok {
		t.Error("missing 'beta' in Desired")
	}
}

func TestEventsEndpoint(t *testing.T) {
	cfgPath := writeTestConfig(t, "gamma")
	r := newTestReconciler(cfgPath)

	// Trigger a reconcile to generate some events.
	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	resp, err := client.Events()
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.Events) == 0 {
		t.Error("expected at least one event after reconcile")
	}
}

func TestReconcileEndpoint(t *testing.T) {
	cfgPath := writeTestConfig(t, "delta")
	r := newTestReconciler(cfgPath)
	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	resp, err := client.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if !resp.OK {
		t.Errorf("expected OK=true, got message: %s", resp.Message)
	}
}

func TestStaleSocketCleanup(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "stale.sock")

	// Create a plain file at the socket path to simulate a stale socket.
	// net.Dial("unix", ...) on a regular file returns "connection refused".
	f, err := os.Create(socketPath)
	if err != nil {
		t.Fatalf("create stale file: %v", err)
	}
	f.Close()

	cfgPath := writeTestConfig(t)
	r := newTestReconciler(cfgPath)
	srv := NewServer(socketPath, r)

	if err := srv.Start(); err != nil {
		t.Fatalf("Start with stale socket: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	// Server should be up and responding.
	client := NewClient(socketPath)
	if !client.IsAvailable() {
		t.Error("expected server to be available after stale socket cleanup")
	}
}

func TestClientConnectionRefused(t *testing.T) {
	client := NewClient("/tmp/hydrascale-nonexistent-99999.sock")

	if client.IsAvailable() {
		t.Error("expected IsAvailable=false for non-existent socket")
	}

	_, err := client.Status()
	if err == nil {
		t.Error("expected error from Status with bad socket path")
	}

	_, err = client.Events()
	if err == nil {
		t.Error("expected error from Events with bad socket path")
	}

	_, err = client.Reconcile()
	if err == nil {
		t.Error("expected error from Reconcile with bad socket path")
	}
}

func TestStatusEndpointJSON(t *testing.T) {
	cfgPath := writeTestConfig(t, "epsilon")
	r := newTestReconciler(cfgPath)
	srv, _, cleanup := startTestServer(t, r)
	defer cleanup()

	// Raw HTTP request to verify JSON content-type and structure.
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", srv.socketPath)
		},
	}
	httpClient := &http.Client{Transport: transport}

	resp, err := httpClient.Get("http://localhost/api/status")
	if err != nil {
		t.Fatalf("GET /api/status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	for _, field := range []string{"desired", "actual", "error_states", "failure_counts", "last_errors"} {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing field %q in status response", field)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	cfgPath := writeTestConfig(t)
	r := newTestReconciler(cfgPath)
	srv, _, cleanup := startTestServer(t, r)
	defer cleanup()

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", srv.socketPath)
		},
	}
	httpClient := &http.Client{Transport: transport}

	// POST to /api/status should be 405.
	resp, err := httpClient.Post("http://localhost/api/status", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/status: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/status status = %d, want 405", resp.StatusCode)
	}

	// GET to /api/reconcile should be 405.
	resp2, err := httpClient.Get("http://localhost/api/reconcile")
	if err != nil {
		t.Fatalf("GET /api/reconcile: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/reconcile status = %d, want 405", resp2.StatusCode)
	}
}

func TestServerShutdownCleanup(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "shutdown-test.sock")
	cfgPath := writeTestConfig(t)
	r := newTestReconciler(cfgPath)
	srv := NewServer(socketPath, r)

	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Verify socket exists.
	if _, err := os.Stat(socketPath); err != nil {
		t.Errorf("socket should exist after Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}

	// Socket file should be cleaned up.
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("socket file should be removed after Shutdown")
	}
}

// Verify ReconcileResponse.Message is populated on failure.
func TestReconcileEndpointError(t *testing.T) {
	// Use a non-existent config path so Reconcile() returns an error.
	r := reconciler.New("/nonexistent/config.yaml", newMockNS(), newMockDaemon(), &mockRouting{}, time.Second, nil)
	socketPath := filepath.Join(t.TempDir(), "err-api.sock")
	srv := NewServer(socketPath, r)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	client := NewClient(socketPath)
	resp, err := client.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if resp.OK {
		t.Error("expected OK=false for reconcile with bad config")
	}
	if resp.Message == "" {
		t.Error("expected non-empty Message on reconcile failure")
	}
	_ = fmt.Sprintf("message: %s", resp.Message) // use fmt
}

// --- Detail endpoint tests ---

func TestDetailEndpoint_HappyPath(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha")
	md := newMockDaemon()
	md.statusResult = &daemon.TailscaleStatus{
		Self: daemon.StatusNode{
			TailscaleIPs: []string{"100.64.1.5"},
			HostName:     "myhost",
		},
		Peer: map[string]daemon.StatusNode{
			"peer1": {HostName: "peer1", Online: true},
			"peer2": {HostName: "peer2", Online: false},
		},
	}
	r := reconciler.New(cfgPath, newMockNS(), md, &mockRouting{}, 1*time.Second, nil)
	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	detail, err := client.TailnetDetail("alpha")
	if err != nil {
		t.Fatalf("TailnetDetail: %v", err)
	}
	if detail.Error != "" {
		t.Fatalf("expected no error, got: %s", detail.Error)
	}
	if len(detail.TailscaleIPs) != 1 || detail.TailscaleIPs[0] != "100.64.1.5" {
		t.Errorf("TailscaleIPs: got %v, want [100.64.1.5]", detail.TailscaleIPs)
	}
	if detail.PeerCount != 2 {
		t.Errorf("PeerCount: got %d, want 2", detail.PeerCount)
	}
	if detail.OnlinePeers != 1 {
		t.Errorf("OnlinePeers: got %d, want 1", detail.OnlinePeers)
	}
	if detail.FetchedAt.IsZero() {
		t.Error("FetchedAt should not be zero")
	}
}

func TestDetailEndpoint_DaemonStarting(t *testing.T) {
	// GetStatus returns (nil, nil) — daemon is starting up
	cfgPath := writeTestConfig(t, "alpha")
	md := newMockDaemon()
	md.statusResult = nil
	md.statusErr = nil
	r := reconciler.New(cfgPath, newMockNS(), md, &mockRouting{}, 1*time.Second, nil)
	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	detail, err := client.TailnetDetail("alpha")
	if err != nil {
		t.Fatalf("TailnetDetail: %v", err)
	}
	if detail.Error == "" {
		t.Error("expected Error to be set when status is nil")
	}
	if detail.TailscaleIPs != nil {
		t.Error("expected nil TailscaleIPs when daemon starting")
	}
}

func TestDetailEndpoint_DaemonError(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha")
	md := newMockDaemon()
	md.statusErr = fmt.Errorf("context deadline exceeded")
	r := reconciler.New(cfgPath, newMockNS(), md, &mockRouting{}, 1*time.Second, nil)
	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	detail, err := client.TailnetDetail("alpha")
	if err != nil {
		t.Fatalf("TailnetDetail: %v", err)
	}
	if detail.Error == "" {
		t.Error("expected Error to be set on daemon error")
	}
}

func TestDetailEndpoint_UnknownID(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha")
	r := newTestReconciler(cfgPath)
	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	_, err := client.TailnetDetail("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown tailnet ID")
	}
}

func TestDetailEndpoint_NoPeers(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha")
	md := newMockDaemon()
	md.statusResult = &daemon.TailscaleStatus{
		Self: daemon.StatusNode{TailscaleIPs: []string{"100.64.1.1"}},
		Peer: map[string]daemon.StatusNode{},
	}
	r := reconciler.New(cfgPath, newMockNS(), md, &mockRouting{}, 1*time.Second, nil)
	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	detail, err := client.TailnetDetail("alpha")
	if err != nil {
		t.Fatalf("TailnetDetail: %v", err)
	}
	if detail.PeerCount != 0 || detail.OnlinePeers != 0 {
		t.Errorf("expected 0 peers, got count=%d online=%d", detail.PeerCount, detail.OnlinePeers)
	}
}

// TestTailnetDetailClient_HTTP500 verifies that Client.TailnetDetail returns an
// error when the server responds with a non-200, non-404 status code.
func TestTailnetDetailClient_HTTP500(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "fake-api.sock")

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tailnet/alpha/detail", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	client := NewClient(socketPath)
	_, err = client.TailnetDetail("alpha")
	if err == nil {
		t.Fatal("expected error from TailnetDetail on HTTP 500, got nil")
	}
}

// Verify that the existing tests still compile and link correctly after mock changes.
// The mockDaemon.statusResult defaults to nil (daemon starting), which is the
// same behavior as the previous hard-coded (nil, nil) return.
func TestDetailEndpoint_BackwardsCompatMock(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha")
	r := newTestReconciler(cfgPath) // uses newMockDaemon() with nil statusResult
	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	detail, err := client.TailnetDetail("alpha")
	if err != nil {
		t.Fatalf("TailnetDetail: %v", err)
	}
	// nil statusResult → "daemon starting" error in response
	if detail.Error == "" {
		t.Error("expected Error set for default mock (nil status)")
	}
}
