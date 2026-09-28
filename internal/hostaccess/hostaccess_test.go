package hostaccess

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/daemon"
	"hydrascale/internal/dns"
	"hydrascale/internal/execx"
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

	m := NewManager("hosts", hostsPath, "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	status := makeTestStatus()

	m.Sync("havoc", status, "10.0.0.1", "veth0", "10.0.0.2", "ns-havoc")

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

	m := NewManager("hosts", hostsPath, "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	m.Sync("havoc", nil, "10.0.0.1", "veth0", "10.0.0.2", "ns-havoc")

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

	m := NewManager("hosts", hostsPath, "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	status := makeTestStatus()

	// Routes will fail (no root), but should not block DNS update
	m.Sync("havoc", status, "10.0.0.1", "veth0", "10.0.0.2", "ns-havoc")

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
	deadListeners map[string]bool
	lastRoutes    map[string]string
	lastZones     map[string]dns.AliasZone
	lastListeners []string
	callCount     int
}

func (f *mockForwarder) SetDomainRoutes(routes map[string]string) {
	f.lastRoutes = routes
	f.callCount++
}

func (f *mockForwarder) SetAliasZones(zones map[string]dns.AliasZone) {
	f.lastZones = zones
}

// SyncListeners records the addresses and answers that every one of them holds a
// listener. deadListeners names an address that fails to bind.
func (f *mockForwarder) SyncListeners(addrs []string) (map[string]bool, error) {
	f.lastListeners = addrs
	live := make(map[string]bool, len(addrs))
	var errs []error
	for _, a := range addrs {
		if f.deadListeners[a] {
			errs = append(errs, fmt.Errorf("open the UDP listener on %s: address already in use", a))
			continue
		}
		live[a] = true
	}
	return live, errors.Join(errs...)
}

// TestSyncDNS_SetsDomainRoutes verifies that syncDNS wires each tailnet's
// MagicDNS suffix to its veth gateway when a forwarder is configured.
func TestSyncDNS_SetsDomainRoutes(t *testing.T) {
	dir := t.TempDir()
	hostsPath := dir + "/hosts"

	fwd := &mockForwarder{}
	m := NewManager("hosts", hostsPath, "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	m.SetForwarder(fwd)

	status1 := &daemon.TailscaleStatus{MagicDNSSuffix: "corp.ts.net"}
	status2 := &daemon.TailscaleStatus{MagicDNSSuffix: "home.ts.net"}

	m.Sync("corp", status1, "10.200.0.2", "vh001", "10.200.0.1", "ns-corp")
	m.Sync("home", status2, "10.200.0.6", "vh002", "10.200.0.5", "ns-home")

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

	m := NewManager("hosts", hostsPath, "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	status := &daemon.TailscaleStatus{MagicDNSSuffix: "corp.ts.net"}

	// Must not panic
	m.Sync("corp", status, "10.200.0.2", "vh001", "10.200.0.1", "ns-corp")
}

// TestSyncDNS_EmptySuffix verifies that a tailnet with no MagicDNSSuffix is
// excluded from the domain routes map sent to the forwarder.
func TestSyncDNS_EmptySuffix(t *testing.T) {
	dir := t.TempDir()
	hostsPath := dir + "/hosts"

	fwd := &mockForwarder{}
	m := NewManager("hosts", hostsPath, "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	m.SetForwarder(fwd)

	// Tailnet with no MagicDNSSuffix
	status := &daemon.TailscaleStatus{MagicDNSSuffix: ""}
	m.Sync("corp", status, "10.200.0.2", "vh001", "10.200.0.1", "ns-corp")

	if fwd.callCount == 0 {
		t.Fatal("SetDomainRoutes was never called")
	}
	for suffix := range fwd.lastRoutes {
		if suffix == "" {
			t.Error("empty suffix should not appear in domain routes map")
		}
	}
}

// capturedEvent records one event of the split DNS conflict recorder. The tests call the
// recorder on the same goroutine as Sync, therefore the list needs no lock.
type capturedEvent struct{ kind, tailnetID, message string }

// eventRecorder returns the recorder of the split DNS conflict events and a getter of the
// captured events.
func eventRecorder() (func(string, string, string), func() []capturedEvent) {
	var events []capturedEvent
	return func(kind, tailnetID, message string) {
			events = append(events, capturedEvent{kind, tailnetID, message})
		}, func() []capturedEvent {
			return append([]capturedEvent(nil), events...)
		}
}

// TestSyncDNS_SplitDomainReachesTheForwarder verifies that a surviving split domain
// reaches the forwarder domain Routes with the veth gateway of its tailnet.
func TestSyncDNS_SplitDomainReachesTheForwarder(t *testing.T) {
	fwd := &mockForwarder{}
	m := NewManager("hosts", t.TempDir()+"/hosts", "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	m.SetForwarder(fwd)

	status := &daemon.TailscaleStatus{SplitDNSRoutes: []string{"acme.example.com"}}
	m.Sync("corp", status, "10.200.0.2", "vh001", "10.200.0.1", "ns-corp")

	if fwd.callCount == 0 {
		t.Fatal("SetDomainRoutes was never called")
	}
	if got := fwd.lastRoutes["acme.example.com"]; got != "10.200.0.2" {
		t.Errorf("acme.example.com route = %q, want %q", got, "10.200.0.2")
	}
}

// TestSyncDNS_SplitDomainRegistersTheDeviceWithoutAMagicDNSSuffix verifies that a tailnet
// that holds a split domain registers its veth device with a MagicDNS suffix of none.
func TestSyncDNS_SplitDomainRegistersTheDeviceWithoutAMagicDNSSuffix(t *testing.T) {
	m := NewManager("resolved", "", "10.200.0.0/16", 0)
	m.Runner = quietRunner{}

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, "systemctl", "is-active", "--quiet", "systemd-resolved")
	rec.Script(execx.Result{}, "resolvectl", "dns", "vh001", "10.200.0.2")
	rec.Script(execx.Result{}, "resolvectl", "domain", "vh001", "~acme.example.com")
	m.resolved.Runner = rec

	status := &daemon.TailscaleStatus{SplitDNSRoutes: []string{"acme.example.com"}}
	m.Sync("corp", status, "10.200.0.2", "vh001", "10.200.0.1", "ns-corp")

	want := []execx.Call{
		{Name: "systemctl", Args: []string{"is-active", "--quiet", "systemd-resolved"}},
		{Name: "resolvectl", Args: []string{"dns", "vh001", "10.200.0.2"}},
		{Name: "resolvectl", Args: []string{"domain", "vh001", "~acme.example.com"}},
	}
	got := rec.Calls()
	if len(got) != len(want) {
		t.Fatalf("the registration ran %d commands, want %d:\n%s", len(got), len(want), callList(got))
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Errorf("command %d = %q, want %q", i, got[i].String(), want[i].String())
		}
	}
}

// TestSyncDNS_SplitDomainConflictPicksADeterministicWinner verifies that two tailnets
// with one split domain produce one surviving registration, a deterministic winner, and
// one event that names both tailnets.
func TestSyncDNS_SplitDomainConflictPicksADeterministicWinner(t *testing.T) {
	fwd := &mockForwarder{}
	m := NewManager("hosts", t.TempDir()+"/hosts", "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	m.SetForwarder(fwd)
	record, captured := eventRecorder()
	m.SetEventRecorder(record)

	shared := []string{"shared.example.com"}
	m.Sync("alpha", &daemon.TailscaleStatus{SplitDNSRoutes: shared}, "10.200.0.2", "vh001", "10.200.0.1", "ns-alpha")
	m.Sync("beta", &daemon.TailscaleStatus{SplitDNSRoutes: shared}, "10.200.0.6", "vh002", "10.200.0.5", "ns-beta")

	// The first tailnet in sorted order keeps the domain.
	if got := fwd.lastRoutes["shared.example.com"]; got != "10.200.0.2" {
		t.Errorf("shared.example.com route = %q, want %q", got, "10.200.0.2")
	}

	events := captured()
	if len(events) != 1 {
		t.Fatalf("the recorder captured %d events, want 1: %v", len(events), events)
	}
	if events[0].kind != SplitDNSEventConflict {
		t.Errorf("event kind = %q, want %q", events[0].kind, SplitDNSEventConflict)
	}
	if !strings.Contains(events[0].message, "beta") || !strings.Contains(events[0].message, "alpha") {
		t.Errorf("the event message names not both tailnets: %q", events[0].message)
	}
	if !strings.Contains(events[0].message, "shared.example.com") {
		t.Errorf("the event message names not the domain: %q", events[0].message)
	}

	// The MagicDNS suffix beats a split domain of another tailnet.
	report := m.SplitDNSReport()
	if len(report) != 2 {
		t.Fatalf("report holds %d tailnets, want 2", len(report))
	}
	if report[0].TailnetID != "alpha" || report[1].TailnetID != "beta" {
		t.Fatalf("report tailnets = %v, want [alpha beta]", report)
	}
	if len(report[0].Domains) != 1 || report[0].Domains[0] != "shared.example.com" {
		t.Errorf("alpha domains = %v, want [shared.example.com]", report[0].Domains)
	}
	if len(report[1].Domains) != 0 || report[1].Conflict == "" {
		t.Errorf("beta = %+v, want no domain and a conflict", report[1])
	}
}

// TestSyncDNS_MagicDNSSuffixBeatsASplitDomain verifies that the MagicDNS suffix of one
// tailnet removes the split domain of another.
func TestSyncDNS_MagicDNSSuffixBeatsASplitDomain(t *testing.T) {
	fwd := &mockForwarder{}
	m := NewManager("hosts", t.TempDir()+"/hosts", "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	m.SetForwarder(fwd)
	record, captured := eventRecorder()
	m.SetEventRecorder(record)

	m.Sync("alpha", &daemon.TailscaleStatus{MagicDNSSuffix: "corp.example.com"}, "10.200.0.2", "vh001", "10.200.0.1", "ns-alpha")
	m.Sync("beta", &daemon.TailscaleStatus{SplitDNSRoutes: []string{"corp.example.com"}}, "10.200.0.6", "vh002", "10.200.0.5", "ns-beta")

	if got := fwd.lastRoutes["corp.example.com"]; got != "10.200.0.2" {
		t.Errorf("corp.example.com route = %q, want %q", got, "10.200.0.2")
	}
	if len(fwd.lastRoutes) != 1 {
		t.Errorf("the routes hold %d domains, want 1: %v", len(fwd.lastRoutes), fwd.lastRoutes)
	}
	if events := captured(); len(events) != 1 {
		t.Errorf("the recorder captured %d events, want 1: %v", len(events), events)
	}
}

// TestSyncDNS_InvalidSplitDomainIsDropped verifies that a split domain that is not a DNS
// name is dropped and that the valid ones survive.
func TestSyncDNS_InvalidSplitDomainIsDropped(t *testing.T) {
	fwd := &mockForwarder{}
	m := NewManager("hosts", t.TempDir()+"/hosts", "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	m.SetForwarder(fwd)

	status := &daemon.TailscaleStatus{SplitDNSRoutes: []string{"-interface=eth0", "*.example.com", "valid.example.com"}}
	m.Sync("corp", status, "10.200.0.2", "vh001", "10.200.0.1", "ns-corp")

	if got := fwd.lastRoutes["valid.example.com"]; got != "10.200.0.2" {
		t.Errorf("valid.example.com route = %q, want %q", got, "10.200.0.2")
	}
	for _, domain := range []string{"-interface=eth0", "*.example.com"} {
		if _, ok := fwd.lastRoutes[domain]; ok {
			t.Errorf("the invalid domain %q reached the routes", domain)
		}
	}

	report := m.SplitDNSReport()
	if len(report) != 1 || report[0].TailnetID != "corp" {
		t.Fatalf("report = %v, want one entry for corp", report)
	}
	if len(report[0].Domains) != 1 || report[0].Domains[0] != "valid.example.com" {
		t.Errorf("report domains = %v, want [valid.example.com]", report[0].Domains)
	}
}

// TestSyncDNS_TSNetIsDropped verifies that a split domain below the reserved ts.net zone
// reaches neither the forwarder routes nor the conflict events, while a valid domain
// survives.
func TestSyncDNS_TSNetIsDropped(t *testing.T) {
	fwd := &mockForwarder{}
	m := NewManager("hosts", t.TempDir()+"/hosts", "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	m.SetForwarder(fwd)
	record, captured := eventRecorder()
	m.SetEventRecorder(record)

	status := &daemon.TailscaleStatus{SplitDNSRoutes: []string{"ts.net", "tail1234.ts.net", "acme.example.com"}}
	m.Sync("corp", status, "10.200.0.2", "vh001", "10.200.0.1", "ns-corp")

	if got := fwd.lastRoutes["acme.example.com"]; got != "10.200.0.2" {
		t.Errorf("acme.example.com route = %q, want %q", got, "10.200.0.2")
	}
	for _, domain := range []string{"ts.net", "tail1234.ts.net"} {
		if _, ok := fwd.lastRoutes[domain]; ok {
			t.Errorf("the reserved domain %q reached the routes", domain)
		}
	}
	if events := captured(); len(events) != 0 {
		t.Errorf("the recorder captured %d events, want 0: %v", len(events), events)
	}

	report := m.SplitDNSReport()
	if len(report) != 1 || report[0].TailnetID != "corp" {
		t.Fatalf("report = %v, want one entry for corp", report)
	}
	if len(report[0].Domains) != 1 || report[0].Domains[0] != "acme.example.com" {
		t.Errorf("report domains = %v, want [acme.example.com]", report[0].Domains)
	}
}

// TestSyncDNS_SteadyStateEmitsOneEventOnly verifies that a conflict that stays across
// every tick reports one event, and that a repeat after the conflict is gone reports
// again.
func TestSyncDNS_SteadyStateEmitsOneEventOnly(t *testing.T) {
	m := NewManager("hosts", t.TempDir()+"/hosts", "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	record, captured := eventRecorder()
	m.SetEventRecorder(record)

	shared := []string{"shared.example.com"}
	for range 3 {
		m.Sync("alpha", &daemon.TailscaleStatus{SplitDNSRoutes: shared}, "10.200.0.2", "vh001", "10.200.0.1", "ns-alpha")
		m.Sync("beta", &daemon.TailscaleStatus{SplitDNSRoutes: shared}, "10.200.0.6", "vh002", "10.200.0.5", "ns-beta")
	}
	if events := captured(); len(events) != 1 {
		t.Fatalf("the recorder captured %d events in the steady state, want 1: %v", len(events), events)
	}

	// beta stops holding the domain; the conflict is gone.
	m.Sync("beta", &daemon.TailscaleStatus{}, "10.200.0.6", "vh002", "10.200.0.5", "ns-beta")
	if events := captured(); len(events) != 1 {
		t.Fatalf("the recorder captured %d events after the conflict left, want 1", len(events))
	}
	// The conflict returns; a new event reports it.
	m.Sync("beta", &daemon.TailscaleStatus{SplitDNSRoutes: shared}, "10.200.0.6", "vh002", "10.200.0.5", "ns-beta")
	if events := captured(); len(events) != 2 {
		t.Fatalf("the recorder captured %d events after the conflict returned, want 2", len(events))
	}
}

// TestTeardown_Idempotent verifies that calling Teardown with nothing set up
// does not panic or error.
func TestTeardown_Idempotent(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")

	m := NewManager("hosts", hostsPath, "10.200.0.0/16", 0)
	m.Runner = quietRunner{}

	// Should not panic
	m.Teardown("nonexistent-tailnet")
	m.Teardown("nonexistent-tailnet")
	m.TeardownAll()
}
