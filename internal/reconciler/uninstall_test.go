package reconciler

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"testing"
)

// uninstallFixture returns a Reconciler whose host holds the namespaces of the tailnets,
// with an IPv4 chain writer, an IPv6 chain writer, and a fake IPv6 host that share one
// call log. cfgPath names the configuration file, which the test writes.
func uninstallFixture(t *testing.T, cfgPath string, tailnets ...string) (*Reconciler, *mockNS, *fakeChainWriter, *fakeChainWriter, *callLog) {
	t.Helper()

	ns := newMockNS()
	for _, id := range tailnets {
		ns.namespaces[ns.GetName(id)] = true
	}
	r := newTestReconciler(cfgPath, ns, newMockDaemon(), newMockRouting())

	log := &callLog{}
	w4 := &fakeChainWriter{log: log}
	w6 := &fakeChainWriter{log: log}
	r.SetChainWriter(w4)
	r.access6 = w6
	r.ipv6 = &fakeIPv6Host{host: upstreamHost(true), log: log}
	return r, ns, w4, w6, log
}

func TestUninstallRemovesEveryNamespaceAndBothChainFamilies(t *testing.T) {
	cfgPath := writeAccessConfig(t, "access:\n  mode: enforce\n  rules:\n    - from: alpha\n      to: internet\n", "alpha", "beta")
	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	r, ns, w4, w6, log := uninstallFixture(t, cfgPath, "alpha", "beta")

	if err := r.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	if len(ns.namespaces) != 0 {
		t.Errorf("the host still holds %v", ns.namespaces)
	}
	if w4.teardown != 1 || w6.teardown != 1 {
		t.Errorf("Uninstall removed the IPv4 chains %d times and the IPv6 chains %d times, want 1 and 1", w4.teardown, w6.teardown)
	}
	// Issue #417. The uninstall ran a tick for an empty rule set, which wrote both chains
	// again after the service shutdown had removed them.
	if len(w4.applied) != 0 || len(w6.applied) != 0 {
		t.Errorf("Uninstall wrote the chains: %d IPv4 writes, %d IPv6 writes", len(w4.applied), len(w6.applied))
	}
	if !slices.Contains(log.recorded(), "reset force_forwarding enp1s0f0") {
		t.Errorf("Uninstall did not reset force_forwarding: %v", log.recorded())
	}
	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("Uninstall changed the configuration file:\n%s", after)
	}
}

func TestUninstallTearsDownWhenTheConfigurationFileDoesNotLoad(t *testing.T) {
	// Issue #417. The uninstall read the tailnets from the configuration file, so a file
	// that did not load left every namespace on the host.
	cfgPath := writeAccessConfig(t, "", "alpha")
	if err := os.WriteFile(cfgPath, []byte("tailnets: [unclosed\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r, ns, w4, w6, _ := uninstallFixture(t, cfgPath, "alpha")

	if err := r.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if len(ns.namespaces) != 0 || w4.teardown != 1 || w6.teardown != 1 {
		t.Errorf("namespaces %v, IPv4 teardowns %d, IPv6 teardowns %d", ns.namespaces, w4.teardown, w6.teardown)
	}
}

func TestUninstallContinuesAfterAFailedStep(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha", "beta")
	r, ns, w4, w6, _ := uninstallFixture(t, cfgPath, "alpha", "beta")
	ns.deleteErr = errors.New("ip netns del: device busy")

	err := r.Uninstall()
	if err == nil {
		t.Fatal("Uninstall returned no error for two failed namespace deletes")
	}
	if w4.teardown != 1 || w6.teardown != 1 {
		t.Errorf("Uninstall stopped after a failed delete: IPv4 teardowns %d, IPv6 teardowns %d", w4.teardown, w6.teardown)
	}
}
