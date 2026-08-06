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
	"hydrascale/internal/execx"
	"hydrascale/internal/namespaces"
)

// fakeChainWriter keeps the compiled rule set that the Reconciler wrote. It runs no command
// on the host.
type fakeChainWriter struct {
	applied  []access.Compiled
	teardown int
	err      error
	// log records the order of the calls of a test that observes two doubles.
	log *callLog
	// jumps holds the placement that Apply reports. A writer with no placement reports
	// the jump rule at the head of each parent chain.
	jumps []access.Placement
}

func (w *fakeChainWriter) Apply(ctx context.Context, c access.Compiled) (access.Result, error) {
	if w.log != nil {
		w.log.add("access.write")
	}
	w.applied = append(w.applied, c)
	if w.err != nil {
		return access.Result{}, w.err
	}
	jumps := w.jumps
	if jumps == nil {
		jumps = []access.Placement{
			{Parent: access.ParentForward, Position: 1},
			{Parent: access.ParentInput, Position: 1},
		}
	}
	return access.Result{Wrote: true, Jumps: jumps}, nil
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

func (w *deadlineWriter) Apply(ctx context.Context, c access.Compiled) (access.Result, error) {
	w.t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		w.t.Error("Apply received a context with no deadline")
		return access.Result{}, nil
	}
	if left := time.Until(deadline); left > time.Second {
		w.t.Errorf("the deadline is %v away, want one second at most", left)
	}
	return access.Result{}, nil
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

// fillers holds the target of each rule that another service writes into FORWARD. The
// security audit measured these three rules above the rules of the daemon on the test
// host.
var fillers = []string{"ts-forward", "DOCKER-USER", "DOCKER-FORWARD"}

// chainListing returns the output of `iptables -S <parent>` where the jump rule into the
// chain sits at the position. The position counts from 1. A position of 0 returns a
// listing that holds every filler rule and no jump rule.
func chainListing(parent, chain string, position int) string {
	var b strings.Builder
	b.WriteString("-P " + parent + " DROP\n")

	above := len(fillers)
	if position > 0 {
		above = position - 1
	}
	for i := 0; i < above; i++ {
		b.WriteString("-A " + parent + " -j " + fillers[i] + "\n")
	}
	if position > 0 {
		b.WriteString("-A " + parent + " -j " + chain + "\n")
	}
	return b.String()
}

// hostWriter returns a Recorder and the chain Writer of the daemon, which runs every
// command through that Recorder. forward is the position of the jump rule in FORWARD, and
// the jump rule in INPUT always heads its chain.
func hostWriter(t *testing.T, forward int) (*execx.Recorder, *access.Writer) {
	t.Helper()

	rec := execx.NewRecorder(t)
	absent := execx.Result{
		Output: []byte("iptables: No chain/target/match by that name.\n"),
		Err:    errors.New("exit status 1"),
	}
	rec.Script(absent, "iptables", "-S", access.ChainForward)
	rec.Script(absent, "iptables", "-S", access.ChainOut)
	rec.Script(execx.Result{}, "iptables-restore", "--noflush")
	rec.Script(execx.Result{Output: []byte(chainListing(access.ParentForward, access.ChainForward, forward))},
		"iptables", "-S", access.ParentForward)
	rec.Script(execx.Result{Output: []byte(chainListing(access.ParentInput, access.ChainOut, 1))},
		"iptables", "-S", access.ParentInput)
	rec.Script(execx.Result{}, "iptables", "-I", access.ParentForward, "1", "-j", access.ChainForward)

	return rec, &access.Writer{Runner: rec}
}

// jumpEvents returns every access.jump_displaced event that the Reconciler recorded.
func jumpEvents(r *Reconciler) []Event {
	var found []Event
	for _, e := range r.Events() {
		if e.Type == "access.jump_displaced" {
			found = append(found, e)
		}
	}
	return found
}

func TestTheReconcilerRecordsAnEventWhenAnotherRuleDisplacesTheJumpRule(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	_, w := hostWriter(t, 4)
	r.SetChainWriter(w)

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got := jumpEvents(r)
	if len(got) != 1 {
		t.Fatalf("the Reconciler recorded %d access.jump_displaced events, want 1: %+v", len(got), r.Events())
	}
	if !strings.Contains(got[0].Message, "4") {
		t.Errorf("the message %q names no position", got[0].Message)
	}
	for _, target := range fillers {
		if !strings.Contains(got[0].Message, target) {
			t.Errorf("the message %q names no rule %s above the jump rule", got[0].Message, target)
		}
	}
}

func TestTheReconcilerRecordsNoEventWhenTheJumpRuleHeadsTheForwardChain(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	_, w := hostWriter(t, 1)
	r.SetChainWriter(w)

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if got := jumpEvents(r); len(got) != 0 {
		t.Errorf("the Reconciler recorded %d events for a jump rule at position 1: %+v", len(got), got)
	}
}

func TestTheReconcilerAddsTheJumpRuleAndRecordsAnEventWhenForwardHoldsNone(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	rec, w := hostWriter(t, 0)
	r.SetChainWriter(w)

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	want := execx.Call{Name: "iptables", Args: []string{"-I", access.ParentForward, "1", "-j", access.ChainForward}}
	found := 0
	for _, c := range rec.Calls() {
		if c.String() == want.String() {
			found++
		}
	}
	if found != 1 {
		t.Errorf("the Reconciler wrote the jump rule %d times, want 1", found)
	}
	if got := jumpEvents(r); len(got) != 1 {
		t.Fatalf("the Reconciler recorded %d events for an absent jump rule, want 1: %+v", len(got), r.Events())
	}
}

func TestTheReconcilerRecordsTheDisplacementOncePerChangeOfPosition(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	rec, w := hostWriter(t, 4)
	r.SetChainWriter(w)

	for i := 0; i < 3; i++ {
		if err := r.Reconcile(); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
	}
	if got := jumpEvents(r); len(got) != 1 {
		t.Fatalf("the Reconciler recorded %d events for three ticks at one position, want 1", len(got))
	}

	rec.Script(execx.Result{Output: []byte(chainListing(access.ParentForward, access.ChainForward, 2))},
		"iptables", "-S", access.ParentForward)
	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got := jumpEvents(r)
	if len(got) != 2 {
		t.Fatalf("the Reconciler recorded %d events for two positions, want 2", len(got))
	}
	if !strings.Contains(got[1].Message, "2") {
		t.Errorf("the message %q names no position", got[1].Message)
	}
}

func TestTheReconcilerReportsThePositionOfTheJumpRule(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
	_, w := hostWriter(t, 4)
	r.SetChainWriter(w)

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if got := r.AccessJumpPosition(access.ParentForward); got != 4 {
		t.Errorf("AccessJumpPosition(%q) = %d, want 4", access.ParentForward, got)
	}
}

// pathEvents returns the access.path_repaired events that the Reconciler recorded.
func pathEvents(r *Reconciler) []Event {
	var got []Event
	for _, e := range r.Events() {
		if e.Type == "access.path_repaired" {
			got = append(got, e)
		}
	}
	return got
}

func TestTheReconcilerWritesAMissingForwardRuleForANamespaceThatExists(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	ns := newMockNS()
	ns.namespaces[namespaces.GetNamespaceName("alpha")] = true
	ns.pathWritten = []string{"nat POSTROUTING -s 10.200.0.2/30 -j MASQUERADE"}
	r := newTestReconciler(cfgPath, ns, newMockDaemon(), newMockRouting())
	r.SetChainWriter(&fakeChainWriter{})

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	want := namespaces.GetNamespaceName("alpha")
	if len(ns.pathNames) != 1 || ns.pathNames[0] != want {
		t.Fatalf("the Reconciler read the forward path of %v, want [%s]", ns.pathNames, want)
	}
	got := pathEvents(r)
	if len(got) != 1 {
		t.Fatalf("the Reconciler recorded %d access.path_repaired events, want 1: %+v", len(got), r.Events())
	}
	if !strings.Contains(got[0].Message, "MASQUERADE") {
		t.Errorf("the event message is %q, want the rule that the Reconciler wrote", got[0].Message)
	}
}

func TestTheReconcilerRecordsNoEventWhenTheForwardPathIsComplete(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	ns := newMockNS()
	ns.namespaces[namespaces.GetNamespaceName("alpha")] = true
	r := newTestReconciler(cfgPath, ns, newMockDaemon(), newMockRouting())
	r.SetChainWriter(&fakeChainWriter{})

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := pathEvents(r); len(got) != 0 {
		t.Errorf("the Reconciler recorded %d events for a complete forward path: %+v", len(got), got)
	}
}

func TestTheReconcilerReadsNoForwardPathForANamespaceThatIsAbsent(t *testing.T) {
	cfgPath := writeAccessConfig(t, "", "alpha")
	ns := newMockNS()
	r := newTestReconciler(cfgPath, ns, newMockDaemon(), newMockRouting())
	r.SetChainWriter(&fakeChainWriter{})

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(ns.pathNames) != 0 {
		t.Errorf("the Reconciler read the forward path of %v, want no read", ns.pathNames)
	}
}
