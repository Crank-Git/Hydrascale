package hostaccess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/daemon"
)

func makeTestStatus() *daemon.TailscaleStatus {
	return &daemon.TailscaleStatus{
		MagicDNSSuffix: "example.ts.net",
		Peer: map[string]daemon.StatusNode{
			"nodeA": {
				HostName:     "mars",
				TailscaleIPs: []string{"100.64.0.1", "fd7a:115c:a1e0::1"},
				Online:       true,
			},
			"nodeB": {
				HostName:     "venus",
				TailscaleIPs: []string{"100.64.0.2"},
				Online:       true,
			},
		},
	}
}

// TestSync_FullFlow creates a manager in hosts mode, syncs a real TailscaleStatus,
// and verifies that the hosts file contains entries for the peers.
func TestSync_FullFlow(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")

	// Write a minimal pre-existing hosts file
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1  localhost\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager("hosts", hostsPath, "10.200.0.0/16")
	status := makeTestStatus()

	m.Sync("havoc", status, "10.0.0.1", "veth0", "ns-havoc")

	got, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}
	content := string(got)

	if !strings.Contains(content, hostsBeginMarker) {
		t.Error("missing begin marker")
	}
	if !strings.Contains(content, hostsEndMarker) {
		t.Error("missing end marker")
	}
	if !strings.Contains(content, "100.64.0.1") {
		t.Error("missing peer v4 address 100.64.0.1")
	}
	if !strings.Contains(content, "fd7a:115c:a1e0::1") {
		t.Error("missing peer v6 address fd7a:115c:a1e0::1")
	}
	if !strings.Contains(content, "havoc-mars") {
		t.Error("missing peer hostname havoc-mars")
	}
	if !strings.Contains(content, "127.0.0.1  localhost") {
		t.Error("original content lost")
	}
}

// TestSync_NilStatus verifies that syncing with a nil status is a no-op
// and leaves the hosts file unchanged.
func TestSync_NilStatus(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")

	initial := "127.0.0.1  localhost\n"
	if err := os.WriteFile(hostsPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}
	info1, _ := os.Stat(hostsPath)

	m := NewManager("hosts", hostsPath, "10.200.0.0/16")
	m.Sync("havoc", nil, "10.0.0.1", "veth0", "ns-havoc")

	info2, _ := os.Stat(hostsPath)
	if info2.ModTime() != info1.ModTime() {
		t.Error("hosts file was modified for nil status sync")
	}

	got, _ := os.ReadFile(hostsPath)
	if string(got) != initial {
		t.Error("hosts file content changed unexpectedly")
	}
}

// TestSync_PartialFailure verifies that even when route sync fails (no root
// privileges), the hosts file is still updated with peer entries.
func TestSync_PartialFailure(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")

	m := NewManager("hosts", hostsPath, "10.200.0.0/16")
	status := makeTestStatus()

	// Routes will fail (no root), but should not block DNS update
	m.Sync("havoc", status, "10.0.0.1", "veth0", "ns-havoc")

	got, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("hosts file not created: %v", err)
	}
	content := string(got)

	if !strings.Contains(content, hostsBeginMarker) {
		t.Error("missing begin marker after partial failure")
	}
	if !strings.Contains(content, "100.64.0.1") {
		t.Error("missing peer entry after partial failure")
	}
}

// mockForwarder records the most recent SetDomainRoutes call.
type mockForwarder struct {
	lastRoutes map[string]string
	callCount  int
}

func (f *mockForwarder) SetDomainRoutes(routes map[string]string) {
	f.lastRoutes = routes
	f.callCount++
}

// TestSyncDNS_SetsDomainRoutes verifies that syncDNS wires each tailnet's
// MagicDNS suffix to its veth gateway when a forwarder is configured.
func TestSyncDNS_SetsDomainRoutes(t *testing.T) {
	dir := t.TempDir()
	hostsPath := dir + "/hosts"

	fwd := &mockForwarder{}
	m := NewManager("hosts", hostsPath, "10.200.0.0/16")
	m.SetForwarder(fwd)

	status1 := &daemon.TailscaleStatus{MagicDNSSuffix: "corp.ts.net"}
	status2 := &daemon.TailscaleStatus{MagicDNSSuffix: "home.ts.net"}

	m.Sync("corp", status1, "10.200.0.2", "vh001", "ns-corp")
	m.Sync("home", status2, "10.200.0.6", "vh002", "ns-home")

	if fwd.callCount == 0 {
		t.Fatal("SetDomainRoutes was never called")
	}
	got := fwd.lastRoutes
	if got["corp.ts.net"] != "10.200.0.2" {
		t.Errorf("corp.ts.net route = %q, want %q", got["corp.ts.net"], "10.200.0.2")
	}
	if got["home.ts.net"] != "10.200.0.6" {
		t.Errorf("home.ts.net route = %q, want %q", got["home.ts.net"], "10.200.0.6")
	}
}

// TestSyncDNS_NoForwarder verifies that Sync does not panic when no forwarder
// is configured.
func TestSyncDNS_NoForwarder(t *testing.T) {
	dir := t.TempDir()
	hostsPath := dir + "/hosts"

	m := NewManager("hosts", hostsPath, "10.200.0.0/16")
	status := &daemon.TailscaleStatus{MagicDNSSuffix: "corp.ts.net"}

	// Must not panic
	m.Sync("corp", status, "10.200.0.2", "vh001", "ns-corp")
}

// TestSyncDNS_EmptySuffix verifies that a tailnet with no MagicDNSSuffix is
// excluded from the domain routes map sent to the forwarder.
func TestSyncDNS_EmptySuffix(t *testing.T) {
	dir := t.TempDir()
	hostsPath := dir + "/hosts"

	fwd := &mockForwarder{}
	m := NewManager("hosts", hostsPath, "10.200.0.0/16")
	m.SetForwarder(fwd)

	// Tailnet with no MagicDNSSuffix
	status := &daemon.TailscaleStatus{MagicDNSSuffix: ""}
	m.Sync("corp", status, "10.200.0.2", "vh001", "ns-corp")

	if fwd.callCount == 0 {
		t.Fatal("SetDomainRoutes was never called")
	}
	for suffix := range fwd.lastRoutes {
		if suffix == "" {
			t.Error("empty suffix should not appear in domain routes map")
		}
	}
}

// TestTeardown_Idempotent verifies that calling Teardown with nothing set up
// does not panic or error.
func TestTeardown_Idempotent(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")

	m := NewManager("hosts", hostsPath, "10.200.0.0/16")

	// Should not panic
	m.Teardown("nonexistent-tailnet")
	m.Teardown("nonexistent-tailnet")
	m.TeardownAll()
}
