package reconciler

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"hydrascale/internal/daemon"
	"hydrascale/internal/hostaccess"
)

// silentRunner answers every route command with empty output and no error, so that a
// host access sync in a test changes no route on the host.
type silentRunner struct{}

func (silentRunner) Run(context.Context, string, ...string) ([]byte, error) { return nil, nil }

// newHostAccessManager returns a host access Manager that writes the hosts file under a
// temporary directory and it runs no command on the host.
func newHostAccessManager(t *testing.T) *hostaccess.Manager {
	t.Helper()
	ha := hostaccess.NewManager("hosts", filepath.Join(t.TempDir(), "hosts"), "10.200.0.0/16", 0)
	ha.Runner = silentRunner{}
	return ha
}

func splitStatus(domains ...string) *daemon.TailscaleStatus {
	return &daemon.TailscaleStatus{SplitDNSRoutes: domains}
}

// TestTheSplitDNSConflictEventReachesTheReconciler verifies that the reconciler wires its
// event log as the recorder of the split DNS conflicts of the host access manager.
func TestTheSplitDNSConflictEventReachesTheReconciler(t *testing.T) {
	cfgPath := writeTestConfig(t)
	ha := newHostAccessManager(t)
	r := New(cfgPath, newMockNS(), newMockDaemon(), newMockRouting(), time.Second, ha, "10.200.0.0/16")

	ha.Sync("alpha", splitStatus("shared.example.com"), "10.200.0.2", "vh001", "10.200.0.1", "ns-alpha")
	ha.Sync("beta", splitStatus("shared.example.com"), "10.200.0.6", "vh002", "10.200.0.5", "ns-beta")

	if !hasEvent(r, "dns.split_domain_conflict", "shared.example.com") {
		t.Errorf("the event list holds no split domain conflict: %v", r.Events())
	}
	if !hasEvent(r, "dns.split_domain_conflict", "alpha") || !hasEvent(r, "dns.split_domain_conflict", "beta") {
		t.Errorf("the conflict event names not both tailnets: %v", r.Events())
	}
}

// TestFetchStatusKeepsTheStatusWhenTheSplitDNSReadFails verifies that a client without
// `tailscale dns status` keeps the sync working: the action fails not and the status stays.
func TestFetchStatusKeepsTheStatusWhenTheSplitDNSReadFails(t *testing.T) {
	cfgPath := writeTestConfig(t)
	dm := newMockDaemon()
	dm.statusResult = &daemon.TailscaleStatus{MagicDNSSuffix: "corp.ts.net"}
	dm.splitErr = errors.New("tailscale: unknown command: dns status")
	r := newTestReconciler(cfgPath, newMockNS(), dm, newMockRouting())

	status, err := r.fetchStatus("ns-corp", "corp")
	if err != nil {
		t.Fatalf("fetchStatus returned an error for a failed split DNS read: %v", err)
	}
	if status.MagicDNSSuffix != "corp.ts.net" {
		t.Errorf("MagicDNSSuffix = %q, want %q", status.MagicDNSSuffix, "corp.ts.net")
	}
	if len(status.SplitDNSRoutes) != 0 {
		t.Errorf("SplitDNSRoutes = %v, want empty", status.SplitDNSRoutes)
	}
}

// TestFetchStatusCopiesTheSplitDNSDomains verifies that the split DNS domains of the
// client reach the status that Sync reads.
func TestFetchStatusCopiesTheSplitDNSDomains(t *testing.T) {
	cfgPath := writeTestConfig(t)
	dm := newMockDaemon()
	dm.statusResult = &daemon.TailscaleStatus{MagicDNSSuffix: "corp.ts.net"}
	dm.splitRoutes["corp"] = []string{"acme.example.com", "zeta.example.com"}
	r := newTestReconciler(cfgPath, newMockNS(), dm, newMockRouting())

	status, err := r.fetchStatus("ns-corp", "corp")
	if err != nil {
		t.Fatalf("fetchStatus: %v", err)
	}
	if len(status.SplitDNSRoutes) != 2 || status.SplitDNSRoutes[0] != "acme.example.com" {
		t.Errorf("SplitDNSRoutes = %v, want [acme.example.com zeta.example.com]", status.SplitDNSRoutes)
	}
}

// TestFetchStatusReturnsTheStatusFailure verifies that a failure of the status read fails
// the action, in the manner of the behaviour before the split DNS export.
func TestFetchStatusReturnsTheStatusFailure(t *testing.T) {
	cfgPath := writeTestConfig(t)
	dm := newMockDaemon()
	dm.statusErr = errors.New("tailscale status: connection refused")
	r := newTestReconciler(cfgPath, newMockNS(), dm, newMockRouting())

	if _, err := r.fetchStatus("ns-corp", "corp"); err == nil {
		t.Fatal("fetchStatus returned no error for a failed status read")
	}
}

// TestSplitDNSReportMapsTheHostAccessReport verifies that the reconciler maps the host
// access report to its own type, and that a reconciler without a host access manager
// reports nil.
func TestSplitDNSReportMapsTheHostAccessReport(t *testing.T) {
	cfgPath := writeTestConfig(t)
	plain := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	if got := plain.SplitDNSReport(); got != nil {
		t.Errorf("SplitDNSReport without a host access manager = %v, want nil", got)
	}

	ha := newHostAccessManager(t)
	r := New(cfgPath, newMockNS(), newMockDaemon(), newMockRouting(), time.Second, ha, "10.200.0.0/16")
	ha.Sync("alpha", splitStatus("shared.example.com"), "10.200.0.2", "vh001", "10.200.0.1", "ns-alpha")
	ha.Sync("beta", splitStatus("shared.example.com"), "10.200.0.6", "vh002", "10.200.0.5", "ns-beta")

	report := r.SplitDNSReport()
	if len(report) != 2 {
		t.Fatalf("SplitDNSReport holds %d tailnets, want 2", len(report))
	}
	if report[0].TailnetID != "alpha" || report[1].TailnetID != "beta" {
		t.Fatalf("SplitDNSReport tailnets = %v, want [alpha beta]", report)
	}
	if len(report[0].Domains) != 1 || report[0].Domains[0] != "shared.example.com" {
		t.Errorf("alpha report = %+v, want the domain", report[0])
	}
	if report[1].Conflict == "" {
		t.Errorf("beta report = %+v, want a conflict", report[1])
	}
}
