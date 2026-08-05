package reconciler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/hostaccess"
)

// writeHostAccessConfig writes a configuration file that holds one tailnet, with the
// host_access value that the caller gives.
func writeHostAccessConfig(t *testing.T, id string, hostAccess bool) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	cfg := config.DefaultConfig()
	cfg.Tailnets = append(cfg.Tailnets, config.Tailnet{ID: id, HostAccess: &hostAccess})
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	return cfgPath
}

// liveState returns an actual state that holds one healthy tailnet.
func liveState(id string) map[string]*TailnetState {
	return map[string]*TailnetState{
		id: {ID: id, NsName: "ns-" + id, NsExists: true, DaemonHealthy: true},
	}
}

func TestDiffEmitsAHostAccessTeardownWhenHostAccessIsFalse(t *testing.T) {
	cfgPath := writeHostAccessConfig(t, "corp", false)
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())

	desired, _ := r.DesiredState()
	actions := r.Diff(desired, liveState("corp"))

	if countActions(actions)[ActionTeardownHostAccess] != 1 {
		t.Errorf("teardown_host_access = %d, want 1", countActions(actions)[ActionTeardownHostAccess])
	}
}

func TestDiffEmitsNoHostAccessTeardownWhenHostAccessIsTrue(t *testing.T) {
	cfgPath := writeHostAccessConfig(t, "corp", true)
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())

	desired, _ := r.DesiredState()
	actions := r.Diff(desired, liveState("corp"))

	counts := countActions(actions)
	if counts[ActionTeardownHostAccess] != 0 {
		t.Errorf("teardown_host_access = %d, want 0", counts[ActionTeardownHostAccess])
	}
	if counts[ActionSyncHostAccess] != 1 {
		t.Errorf("sync_host_access = %d, want 1", counts[ActionSyncHostAccess])
	}
}

func TestDiffEmitsOneHostAccessTeardownForOneTailnet(t *testing.T) {
	cfgPath := writeHostAccessConfig(t, "corp", false)
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())

	var got []string
	r.teardownHostAccess = func(nsName string, index int, infraSubnet string) error {
		got = append(got, nsName)
		return nil
	}

	desired, _ := r.DesiredState()
	for range 3 {
		r.Apply(r.Diff(desired, liveState("corp")))
	}

	if len(got) != 1 {
		t.Errorf("the reconciler tore down host access %d times, want 1: %v", len(got), got)
	}
}

func TestApplyRecordsTeardownFailedWhenTheHostAccessTeardownFails(t *testing.T) {
	cfgPath := writeHostAccessConfig(t, "corp", false)
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	r.teardownHostAccess = func(nsName string, index int, infraSubnet string) error {
		return errors.New("iptables: Permission denied")
	}

	desired, _ := r.DesiredState()
	r.Apply(r.Diff(desired, liveState("corp")))

	if !hasEvent(r, "teardown.failed", "iptables: Permission denied") {
		t.Errorf("the event list holds no teardown.failed: %v", r.Events())
	}
}

func TestApplyRecordsTeardownFailedWhenTheNamespaceDeleteFails(t *testing.T) {
	cfgPath := writeTestConfig(t)
	ns := newMockNS()
	ns.deleteErr = errors.New("ip netns del: device busy")
	r := newTestReconciler(cfgPath, ns, newMockDaemon(), newMockRouting())

	r.Apply([]Action{{Type: ActionDeleteNS, TailnetID: "corp", NsName: "ns-corp"}})

	if !hasEvent(r, "teardown.failed", "device busy") {
		t.Errorf("the event list holds no teardown.failed: %v", r.Events())
	}
}

func TestDeleteNamespaceRemovesTheNamesOfTheTailnetFromTheHostsFile(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	ha := hostaccess.NewManager("hosts", hostsPath, "10.200.0.0/16")
	ha.Sync("corp", statusWithPeer("corp.ts.net", "laptop", "100.64.0.1"), "10.200.0.2", "vh001", "ns-corp")
	ha.Sync("home", statusWithPeer("home.ts.net", "server", "100.64.1.1"), "10.200.0.6", "vh002", "ns-home")

	cfgPath := writeTestConfig(t, "home")
	r := New(cfgPath, newMockNS(), newMockDaemon(), newMockRouting(), time.Second, ha, "10.200.0.0/16")

	if err := r.executeAction(Action{Type: ActionDeleteNS, TailnetID: "corp", NsName: "ns-corp"}); err != nil {
		t.Fatalf("executeAction: %v", err)
	}

	after, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("read the hosts file: %v", err)
	}
	if strings.Contains(string(after), "corp-laptop") {
		t.Errorf("the hosts file still holds a name of the removed tailnet:\n%s", after)
	}
}

func TestLoopRecordsRulesReapedAtStart(t *testing.T) {
	cfgPath := writeTestConfig(t)
	ns := newMockNS()
	ns.reapCount = 2
	r := newTestReconciler(cfgPath, ns, newMockDaemon(), newMockRouting())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Loop(ctx); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if !hasEvent(r, "rules.reaped", "2") {
		t.Errorf("the event list holds no rules.reaped: %v", r.Events())
	}
}

// hasEvent reports whether the event list holds one event of the type eventType whose
// message contains substring.
func hasEvent(r *Reconciler, eventType, substring string) bool {
	for _, e := range r.Events() {
		if e.Type == eventType && strings.Contains(e.Message, substring) {
			return true
		}
	}
	return false
}

// statusWithPeer returns a status that holds one peer with one IPv4 address.
func statusWithPeer(suffix, hostname, ip string) *daemon.TailscaleStatus {
	return &daemon.TailscaleStatus{
		MagicDNSSuffix: suffix,
		Peer: map[string]daemon.StatusNode{
			"key": {HostName: hostname, TailscaleIPs: []string{ip}, Online: true},
		},
	}
}
