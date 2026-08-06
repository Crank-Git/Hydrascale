package reconciler

import (
	"context"
	"sync"
	"testing"
	"time"

	"hydrascale/internal/reach"
)

// mockProber is a NamespaceProber for a test. It returns a scripted Result for each
// namespace, and it records the namespace and the target of every probe.
type mockProber struct {
	mu      sync.Mutex
	results map[string]reach.Result // nsName -> the Result that Probe returns
	names   []string                // the namespace of each probe, in order
	targets []string                // the target of each probe, in order
	// block makes Probe wait for the end of ctx, which is what a probe of a namespace
	// with no answer does.
	block bool
}

func newMockProber() *mockProber {
	return &mockProber{results: make(map[string]reach.Result)}
}

func (m *mockProber) Probe(ctx context.Context, nsName, target string) reach.Result {
	m.mu.Lock()
	m.names = append(m.names, nsName)
	m.targets = append(m.targets, target)
	res, ok := m.results[nsName]
	block := m.block
	m.mu.Unlock()

	if block {
		<-ctx.Done()
		return reach.Result{
			State:     reach.StateUnreachable,
			Target:    target,
			CheckedAt: time.Now(),
			Detail:    ctx.Err().Error(),
		}
	}
	if !ok {
		return reach.Result{State: reach.StateReachable, Target: target, CheckedAt: time.Now()}
	}
	return res
}

func (m *mockProber) probed() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.names))
	copy(out, m.names)
	return out
}

// reachabilityOf runs one measurement round and returns the field of the status response
// for each tailnet that the configuration file declares.
func reachabilityOf(t *testing.T, r *Reconciler) map[string]reach.Result {
	t.Helper()
	desired, err := r.DesiredState()
	if err != nil {
		t.Fatalf("DesiredState: %v", err)
	}
	actual, err := r.ActualState()
	if err != nil {
		t.Fatalf("ActualState: %v", err)
	}
	r.probeReachability(desired, actual)

	// The status response reads the state through ActualState, therefore the test reads
	// it there as well and not from the map that probeReachability filled.
	after, err := r.ActualState()
	if err != nil {
		t.Fatalf("ActualState: %v", err)
	}
	out := make(map[string]reach.Result, len(after))
	for id, state := range after {
		if state.MeasuredReachability == nil {
			t.Fatalf("tailnet %q carries no measured reachability", id)
		}
		out[id] = *state.MeasuredReachability
	}
	return out
}

func TestAFailedProbeReportsUnreachableWhileHealthyStillReportsThatTheProcessRuns(t *testing.T) {
	cfgPath := writeTestConfig(t, "havoc")
	ns := newMockNS()
	ns.namespaces["ns-havoc"] = true
	dm := newMockDaemon()
	dm.healthy["havoc"] = true

	r := newTestReconciler(cfgPath, ns, dm, newMockRouting())
	prober := newMockProber()
	prober.results["ns-havoc"] = reach.Result{
		State:     reach.StateUnreachable,
		Target:    "192.168.1.1",
		CheckedAt: time.Now(),
		Detail:    "100% packet loss",
	}
	r.SetProber(prober)

	got := reachabilityOf(t, r)
	if got["havoc"].State != reach.StateUnreachable {
		t.Errorf("the measured reachability is %q, want %q", got["havoc"].State, reach.StateUnreachable)
	}

	actual, err := r.ActualState()
	if err != nil {
		t.Fatalf("ActualState: %v", err)
	}
	if !actual["havoc"].DaemonHealthy {
		t.Error("healthy no longer reports that the tailscaled process runs")
	}
}

func TestASuccessfulProbeAndAFailedProbeGiveDifferentValues(t *testing.T) {
	cfgPath := writeTestConfig(t, "havoc", "jbones")
	ns := newMockNS()
	ns.namespaces["ns-havoc"] = true
	ns.namespaces["ns-jbones"] = true
	dm := newMockDaemon()
	dm.healthy["havoc"] = true
	dm.healthy["jbones"] = true

	r := newTestReconciler(cfgPath, ns, dm, newMockRouting())
	prober := newMockProber()
	prober.results["ns-havoc"] = reach.Result{State: reach.StateReachable, Target: "192.168.1.1", CheckedAt: time.Now()}
	prober.results["ns-jbones"] = reach.Result{State: reach.StateUnreachable, Target: "192.168.1.1", CheckedAt: time.Now(), Detail: "100% packet loss"}
	r.SetProber(prober)

	got := reachabilityOf(t, r)
	if got["havoc"].State == got["jbones"].State {
		t.Fatalf("both tailnets report %q, and the two measurements differ", got["havoc"].State)
	}
	if got["havoc"].State != reach.StateReachable {
		t.Errorf("havoc reports %q, want %q", got["havoc"].State, reach.StateReachable)
	}
	if got["jbones"].State != reach.StateUnreachable {
		t.Errorf("jbones reports %q, want %q", got["jbones"].State, reach.StateUnreachable)
	}
}

func TestTheProbeTimeoutKeepsTheReconcilerTickInsideOneSecond(t *testing.T) {
	cfgPath := writeTestConfig(t, "one", "two", "three")
	ns := newMockNS()
	dm := newMockDaemon()
	for _, id := range []string{"one", "two", "three"} {
		ns.namespaces["ns-"+id] = true
		dm.healthy[id] = true
	}

	r := newTestReconciler(cfgPath, ns, dm, newMockRouting())
	prober := newMockProber()
	prober.block = true // no namespace answers, therefore every probe runs to its deadline
	r.SetProber(prober)

	desired, err := r.DesiredState()
	if err != nil {
		t.Fatalf("DesiredState: %v", err)
	}
	actual, err := r.ActualState()
	if err != nil {
		t.Fatalf("ActualState: %v", err)
	}

	start := time.Now()
	r.probeReachability(desired, actual)
	elapsed := time.Since(start)

	if elapsed >= time.Second {
		t.Fatalf("the measurement took %v, and the specification bounds the tick at 1 second", elapsed)
	}
	if len(prober.probed()) != 3 {
		t.Fatalf("the daemon probed %d namespaces, want 3", len(prober.probed()))
	}
	for id, state := range actual {
		if state.MeasuredReachability == nil {
			t.Fatalf("tailnet %q carries no measured reachability", id)
		}
		if state.MeasuredReachability.State != reach.StateUnreachable {
			t.Errorf("tailnet %q reports %q for a probe that timed out, want %q",
				id, state.MeasuredReachability.State, reach.StateUnreachable)
		}
	}
}

func TestATailnetThatTheOperatorStoppedReportsNotProbed(t *testing.T) {
	cfgPath := writeTestConfig(t, "havoc", "jbones")
	ns := newMockNS()
	ns.namespaces["ns-havoc"] = true
	ns.namespaces["ns-jbones"] = true
	dm := newMockDaemon()
	dm.healthy["havoc"] = true
	dm.healthy["jbones"] = true

	r := newTestReconciler(cfgPath, ns, dm, newMockRouting())
	prober := newMockProber()
	prober.results["ns-jbones"] = reach.Result{State: reach.StateUnreachable, Target: "192.168.1.1", CheckedAt: time.Now()}
	r.SetProber(prober)
	if err := r.StopDaemon("havoc"); err != nil {
		t.Fatalf("StopDaemon: %v", err)
	}

	got := reachabilityOf(t, r)
	if got["havoc"].State != reach.StateNotProbed {
		t.Errorf("the tailnet that the operator stopped reports %q, want %q",
			got["havoc"].State, reach.StateNotProbed)
	}
	if got["havoc"].Detail == "" {
		t.Error("the tailnet that the operator stopped states no reason for the absent measurement")
	}
	if got["jbones"].State != reach.StateUnreachable {
		t.Errorf("the broken path reports %q, want %q", got["jbones"].State, reach.StateUnreachable)
	}
	for _, name := range prober.probed() {
		if name == "ns-havoc" {
			t.Error("the daemon sent a packet from the namespace of a tailnet that the operator stopped")
		}
	}
}

func TestANamespaceWithNoMeasurementYetReportsNotProbed(t *testing.T) {
	cfgPath := writeTestConfig(t, "havoc")
	ns := newMockNS()
	ns.namespaces["ns-havoc"] = true
	dm := newMockDaemon()
	dm.healthy["havoc"] = true

	r := newTestReconciler(cfgPath, ns, dm, newMockRouting())

	actual, err := r.ActualState()
	if err != nil {
		t.Fatalf("ActualState: %v", err)
	}
	state := actual["havoc"]
	if state.MeasuredReachability == nil {
		t.Fatal("a namespace with no measurement carries no field")
	}
	if state.MeasuredReachability.State != reach.StateNotProbed {
		t.Errorf("a namespace with no measurement reports %q, want %q",
			state.MeasuredReachability.State, reach.StateNotProbed)
	}
}
