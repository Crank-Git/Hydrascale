package reconciler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hydrascale/internal/access"
	"hydrascale/internal/config"
	"hydrascale/internal/namespaces"
)

// fakeChainWriter keeps the compiled rule set that the Reconciler wrote. It runs no command
// on the host.
type fakeChainWriter struct {
	applied  []access.Compiled
	teardown int
	err      error
}

func (w *fakeChainWriter) Apply(ctx context.Context, c access.Compiled) (bool, error) {
	w.applied = append(w.applied, c)
	if w.err != nil {
		return false, w.err
	}
	return true, nil
}

func (w *fakeChainWriter) Teardown(ctx context.Context) error {
	w.teardown++
	return nil
}

// writeAccessConfig writes a configuration file that declares the tailnets and holds the
// access block that body carries. An empty body writes no access block.
func writeAccessConfig(t *testing.T, body string, tailnets ...string) string {
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
	if body == "" {
		return cfgPath
	}

	existing, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(cfgPath, append(existing, []byte(body)...), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return cfgPath
}

// device returns the host side veth device name of the tailnet.
func device(id string) string {
	hostVeth, _ := namespaces.VethNames(namespaces.GetNamespaceName(id))
	return hostVeth
}

// forwardRules returns the forward rules of the last compiled set, as one line each.
func forwardRules(t *testing.T, w *fakeChainWriter) []string {
	t.Helper()
	if len(w.applied) == 0 {
		t.Fatal("the Reconciler wrote no rule set")
	}
	last := w.applied[len(w.applied)-1]
	lines := make([]string, 0, len(last.Forward))
	for _, rule := range last.Forward {
		lines = append(lines, strings.Join(rule, " "))
	}
	return lines
}

func TestReconcileWritesTheCompiledRuleSetOfTheConfigurationFile(t *testing.T) {
	cfgPath := writeAccessConfig(t, "access:\n  mode: enforce\n  rules:\n    - from: alpha\n      to: beta\n", "alpha", "beta")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	w := &fakeChainWriter{}
	r.SetChainWriter(w)

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	want := "-A " + access.ChainForward + " -i " + device("alpha") + " -o " + device("beta") + " -j ACCEPT"
	got := forwardRules(t, w)
	found := false
	for _, line := range got {
		if line == want {
			found = true
		}
	}
	if !found {
		t.Errorf("the compiled forward chain holds no rule %q:\n%s", want, strings.Join(got, "\n"))
	}
}

func TestReconcileClosesTheForwardChainWithADropInTheModeEnforce(t *testing.T) {
	cfgPath := writeAccessConfig(t, "access:\n  mode: enforce\n", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	w := &fakeChainWriter{}
	r.SetChainWriter(w)

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got := forwardRules(t, w)
	last := got[len(got)-1]
	if last != "-A "+access.ChainForward+" -j DROP" {
		t.Errorf("the last forward rule = %q, want a DROP", last)
	}
}

func TestReconcileClosesTheForwardChainWithALogInTheModeObserve(t *testing.T) {
	cfgPath := writeAccessConfig(t, "access:\n  mode: observe\n", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	w := &fakeChainWriter{}
	r.SetChainWriter(w)

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got := forwardRules(t, w)
	for _, line := range got {
		if strings.HasSuffix(line, "-j DROP") {
			t.Errorf("the mode observe wrote a DROP rule: %q", line)
		}
	}
	if !strings.Contains(strings.Join(got, "\n"), access.LogPrefix) {
		t.Errorf("the mode observe wrote no LOG rule:\n%s", strings.Join(got, "\n"))
	}
}

// TestTheReconcilerDropsARuleThatNamesATailnetTheConfigurationFileRemoved holds the edge
// case "A tailnet is removed while a rule names it". The test calls the filter rather than
// Reconcile, because config.LoadConfig rejects such a file before the Reconciler reads it.
// The changelog of docs/specs/spec.md records that gap.
func TestTheReconcilerDropsARuleThatNamesATailnetTheConfigurationFileRemoved(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())

	devices := map[string]string{"alpha": device("alpha")}
	kept := r.declaredRules([]access.Rule{
		{From: "alpha", To: "gone"},
		{From: "alpha", To: access.Internet},
	}, devices)

	if len(kept) != 1 || kept[0].To != access.Internet {
		t.Errorf("the filter kept %v, want the rule to the internet alone", kept)
	}

	dropped := 0
	for _, e := range r.Events() {
		if e.Type == "access.rule_dropped" {
			dropped++
		}
	}
	if dropped != 1 {
		t.Errorf("access.rule_dropped events = %d, want 1", dropped)
	}
}

func TestReconcileRecordsAccessWriteFailedWhenTheWriteFails(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	w := &fakeChainWriter{err: errWrite}
	r.SetChainWriter(w)

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	found := false
	for _, e := range r.Events() {
		if e.Type == "access.write_failed" {
			found = true
		}
	}
	if !found {
		t.Error("the Reconciler recorded no access.write_failed event for a failed write")
	}
}

func TestReconcileBoundsTheChainWriteWithinOneSecond(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	r.SetChainWriter(&deadlineWriter{t: t})

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
}

// deadlineWriter asserts that Apply receives a context with a deadline no further away than
// one second. The specification states that the reconciler tick does not grow longer than
// 1 second because of the rule engine.
type deadlineWriter struct{ t *testing.T }

func (w *deadlineWriter) Apply(ctx context.Context, c access.Compiled) (bool, error) {
	w.t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		w.t.Error("Apply received a context with no deadline")
		return false, nil
	}
	if left := time.Until(deadline); left > time.Second {
		w.t.Errorf("the deadline is %v away, want one second at most", left)
	}
	return false, nil
}

func (w *deadlineWriter) Teardown(ctx context.Context) error { return nil }

func TestShutdownRemovesTheChainsAndTheJumpRules(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	w := &fakeChainWriter{}
	r.SetChainWriter(w)

	if err := r.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if w.teardown != 1 {
		t.Errorf("Shutdown ran the teardown %d times, want 1", w.teardown)
	}
}

func TestTheModeEnforceRemovesTheForwardAcceptRulesOfVersionZeroNine(t *testing.T) {
	cfgPath := writeAccessConfig(t, "access:\n  mode: enforce\n", "alpha")
	ns := newMockNS()
	ns.legacy = 2
	r := newTestReconciler(cfgPath, ns, newMockDaemon(), newMockRouting())
	r.SetChainWriter(&fakeChainWriter{})

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	found := false
	for _, e := range r.Events() {
		if e.Type == "rules.reaped" && strings.Contains(e.Message, "version 0.9") {
			found = true
		}
	}
	if !found {
		t.Error("the Reconciler recorded no event for the removed version 0.9 rules")
	}
}

// TestTheModeObserveKeepsTheForwardAcceptRulesOfVersionZeroNine holds the rule that the
// mode observe denies nothing. The version 0.9 rules are the only path that accepts the
// traffic of a namespace until the chain holds the rules of the operator.
func TestTheModeObserveKeepsTheForwardAcceptRulesOfVersionZeroNine(t *testing.T) {
	cfgPath := writeAccessConfig(t, "access:\n  mode: observe\n", "alpha")
	ns := newMockNS()
	ns.legacy = 2
	r := newTestReconciler(cfgPath, ns, newMockDaemon(), newMockRouting())
	r.SetChainWriter(&fakeChainWriter{})

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	for _, e := range r.Events() {
		if e.Type == "rules.reaped" && strings.Contains(e.Message, "version 0.9") {
			t.Error("the mode observe removed the version 0.9 rules")
		}
	}
}

func TestReconcileRecordsANamespaceThatForwards(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	ns := newMockNS()
	ns.forwarding = "1"
	r := newTestReconciler(cfgPath, ns, newMockDaemon(), newMockRouting())
	r.SetChainWriter(&fakeChainWriter{})

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	found := false
	for _, e := range r.Events() {
		if e.Type == "access.namespace_forwarding" && e.TailnetID == "alpha" {
			found = true
		}
	}
	if !found {
		t.Error("the Reconciler recorded no event for a namespace whose net.ipv4.ip_forward is 1")
	}
}

func TestReconcileRecordsNoEventForANamespaceThatDoesNotForward(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	r.SetChainWriter(&fakeChainWriter{})

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	for _, e := range r.Events() {
		if e.Type == "access.namespace_forwarding" {
			t.Errorf("the Reconciler recorded an event for a namespace that does not forward: %s", e.Message)
		}
	}
}

// errWrite is the failure that a chain write returns in a test.
var errWrite = errors.New("iptables-restore --noflush: exit status 1")
