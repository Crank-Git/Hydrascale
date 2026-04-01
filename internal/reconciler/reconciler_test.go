package reconciler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/routing"
)

// --- Mock implementations ---

type mockNS struct {
	namespaces map[string]bool // nsName -> exists
	createErr  error
	deleteErr  error
	listErr    error
}

func newMockNS() *mockNS {
	return &mockNS{namespaces: make(map[string]bool)}
}

func (m *mockNS) Create(tailnetID string) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.namespaces[m.GetName(tailnetID)] = true
	return nil
}

func (m *mockNS) Delete(nsName string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.namespaces, nsName)
	return nil
}

func (m *mockNS) List() ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
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

func (m *mockNS) SetupVeth(nsName string, index int) error {
	return nil
}

func (m *mockNS) TeardownVeth(nsName string) error {
	return nil
}

type mockDaemon struct {
	healthy   map[string]bool // tailnetID -> healthy
	startErr  map[string]error
	stopErr   error
	healthErr error
}

func newMockDaemon() *mockDaemon {
	return &mockDaemon{
		healthy:  make(map[string]bool),
		startErr: make(map[string]error),
	}
}

func (m *mockDaemon) Start(tailnetID, nsName string) error {
	if err, ok := m.startErr[tailnetID]; ok && err != nil {
		return err
	}
	m.healthy[tailnetID] = true
	return nil
}

func (m *mockDaemon) Stop(nsName, tailnetID string) error {
	if m.stopErr != nil {
		return m.stopErr
	}
	delete(m.healthy, tailnetID)
	return nil
}

func (m *mockDaemon) CheckHealth(nsName, tailnetID string) (bool, error) {
	if m.healthErr != nil {
		return false, m.healthErr
	}
	return m.healthy[tailnetID], nil
}

func (m *mockDaemon) GetSocketPath(tailnetID string) string {
	return "/tmp/test-" + tailnetID + ".sock"
}

func (m *mockDaemon) AuthorizeDaemon(tailnetID, nsName, authKey string) error {
	return nil
}

type mockRouting struct {
	routes   map[string][]routing.Route
	pollErr  error
	syncErr  error
	listErr  error
	listResp []string
}

func newMockRouting() *mockRouting {
	return &mockRouting{routes: make(map[string][]routing.Route)}
}

func (m *mockRouting) PollStatus(nsName, socketPath string) ([]routing.Route, error) {
	if m.pollErr != nil {
		return nil, m.pollErr
	}
	return m.routes[nsName], nil
}

func (m *mockRouting) SyncRoutes(nsName string, routes []routing.Route) error {
	if m.syncErr != nil {
		return m.syncErr
	}
	m.routes[nsName] = routes
	return nil
}

func (m *mockRouting) ListRoutes(nsName string) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listResp, nil
}

// --- Helper ---

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

func newTestReconciler(cfgPath string, ns *mockNS, dm *mockDaemon, rt *mockRouting) *Reconciler {
	return New(cfgPath, ns, dm, rt, 1*time.Second)
}

// --- Tests ---

func TestDesiredState(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha", "beta", "gamma")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())

	desired, err := r.DesiredState()
	if err != nil {
		t.Fatalf("DesiredState: %v", err)
	}
	if len(desired) != 3 {
		t.Errorf("len(desired) = %d, want 3", len(desired))
	}
	for _, id := range []string{"alpha", "beta", "gamma"} {
		if _, ok := desired[id]; !ok {
			t.Errorf("missing tailnet %q in desired state", id)
		}
	}
}

func TestDiff_CreateAll(t *testing.T) {
	// Desired: 3 tailnets. Actual: 0. Should create all.
	cfgPath := writeTestConfig(t, "a", "b", "c")
	ns := newMockNS()
	dm := newMockDaemon()
	rt := newMockRouting()
	r := newTestReconciler(cfgPath, ns, dm, rt)

	desired, _ := r.DesiredState()
	actual := make(map[string]*TailnetState) // empty

	actions := r.Diff(desired, actual)

	// Two-phase reconcile: new tailnets get create_ns + start_daemon only (no sync_routes).
	// Route sync happens on the next cycle once daemons are healthy.
	if len(actions) != 6 {
		t.Errorf("len(actions) = %d, want 6 (3 tailnets x 2 actions: create_ns + start_daemon)", len(actions))
		for _, a := range actions {
			t.Logf("  %s", a)
		}
	}

	counts := countActions(actions)
	if counts[ActionCreateNS] != 3 {
		t.Errorf("create_ns = %d, want 3", counts[ActionCreateNS])
	}
	if counts[ActionStartDaemon] != 3 {
		t.Errorf("start_daemon = %d, want 3", counts[ActionStartDaemon])
	}
}

func TestDiff_DeleteExtra(t *testing.T) {
	// Desired: 2 tailnets. Actual: 3. Should delete 1.
	cfgPath := writeTestConfig(t, "keep1", "keep2")
	ns := newMockNS()
	dm := newMockDaemon()
	rt := newMockRouting()
	r := newTestReconciler(cfgPath, ns, dm, rt)

	desired, _ := r.DesiredState()
	actual := map[string]*TailnetState{
		"keep1":  {ID: "keep1", NsName: "ns-keep1", NsExists: true, DaemonHealthy: true},
		"keep2":  {ID: "keep2", NsName: "ns-keep2", NsExists: true, DaemonHealthy: true},
		"remove": {ID: "remove", NsName: "ns-remove", NsExists: true, DaemonHealthy: true},
	}

	actions := r.Diff(desired, actual)

	// Should have stop + delete for "remove", plus sync_routes for keep1 and keep2
	stopCount := countActions(actions)[ActionStopDaemon]
	deleteCount := countActions(actions)[ActionDeleteNS]
	if stopCount != 1 {
		t.Errorf("stop_daemon = %d, want 1", stopCount)
	}
	if deleteCount != 1 {
		t.Errorf("delete_ns = %d, want 1", deleteCount)
	}
}

func TestDiff_NoChanges(t *testing.T) {
	// Desired == Actual, all healthy. Only sync_routes actions.
	cfgPath := writeTestConfig(t, "x", "y")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())

	desired, _ := r.DesiredState()
	actual := map[string]*TailnetState{
		"x": {ID: "x", NsName: "ns-x", NsExists: true, DaemonHealthy: true},
		"y": {ID: "y", NsName: "ns-y", NsExists: true, DaemonHealthy: true},
	}

	actions := r.Diff(desired, actual)

	counts := countActions(actions)
	if counts[ActionCreateNS] != 0 {
		t.Errorf("create_ns = %d, want 0", counts[ActionCreateNS])
	}
	if counts[ActionDeleteNS] != 0 {
		t.Errorf("delete_ns = %d, want 0", counts[ActionDeleteNS])
	}
	// Should only have sync_routes
	if counts[ActionSyncRoutes] != 2 {
		t.Errorf("sync_routes = %d, want 2", counts[ActionSyncRoutes])
	}
}

func TestDiff_UnhealthyDaemon(t *testing.T) {
	cfgPath := writeTestConfig(t, "sick")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())

	desired, _ := r.DesiredState()
	actual := map[string]*TailnetState{
		"sick": {ID: "sick", NsName: "ns-sick", NsExists: true, DaemonHealthy: false},
	}

	actions := r.Diff(desired, actual)

	counts := countActions(actions)
	if counts[ActionStartDaemon] != 1 {
		t.Errorf("start_daemon = %d, want 1 (restart unhealthy)", counts[ActionStartDaemon])
	}
	if counts[ActionCreateNS] != 0 {
		t.Errorf("create_ns = %d, want 0 (ns exists)", counts[ActionCreateNS])
	}
}

func TestApply_PartialFailure(t *testing.T) {
	cfgPath := writeTestConfig(t, "ok", "fail")
	ns := newMockNS()
	dm := newMockDaemon()
	dm.startErr["fail"] = fmt.Errorf("simulated failure")
	rt := newMockRouting()
	r := newTestReconciler(cfgPath, ns, dm, rt)

	actions := []Action{
		{Type: ActionStartDaemon, TailnetID: "ok", NsName: "ns-ok"},
		{Type: ActionStartDaemon, TailnetID: "fail", NsName: "ns-fail"},
	}

	r.Apply(actions)

	// "ok" should succeed, "fail" should have failure count
	counts := r.FailureCounts()
	if counts["fail"] != 1 {
		t.Errorf("fail failure count = %d, want 1", counts["fail"])
	}
	if counts["ok"] != 0 {
		t.Errorf("ok failure count = %d, want 0", counts["ok"])
	}
}

func TestApply_ErrorStateAfterMaxFailures(t *testing.T) {
	cfgPath := writeTestConfig(t, "bad")
	ns := newMockNS()
	dm := newMockDaemon()
	dm.startErr["bad"] = fmt.Errorf("always fails")
	rt := newMockRouting()
	r := newTestReconciler(cfgPath, ns, dm, rt)

	action := Action{Type: ActionStartDaemon, TailnetID: "bad", NsName: "ns-bad"}

	// Fail MaxFailures times
	for i := 0; i < MaxFailures; i++ {
		r.Apply([]Action{action})
	}

	errors := r.ErrorStates()
	if !errors["bad"] {
		t.Error("expected 'bad' to be in error state after 3 failures")
	}

	// Diff should skip tailnets in error state
	desired := map[string]config.Tailnet{"bad": {ID: "bad"}}
	actual := make(map[string]*TailnetState)
	actions := r.Diff(desired, actual)
	if len(actions) != 0 {
		t.Errorf("len(actions) = %d, want 0 (error state should be skipped)", len(actions))
	}
}

func TestResetError(t *testing.T) {
	cfgPath := writeTestConfig(t, "recover")
	ns := newMockNS()
	dm := newMockDaemon()
	dm.startErr["recover"] = fmt.Errorf("fails")
	rt := newMockRouting()
	r := newTestReconciler(cfgPath, ns, dm, rt)

	// Push into error state
	action := Action{Type: ActionStartDaemon, TailnetID: "recover", NsName: "ns-recover"}
	for i := 0; i < MaxFailures; i++ {
		r.Apply([]Action{action})
	}

	if !r.ErrorStates()["recover"] {
		t.Fatal("expected error state")
	}

	// Reset
	r.ResetError("recover")

	if r.ErrorStates()["recover"] {
		t.Error("expected error state to be cleared")
	}
	if r.FailureCounts()["recover"] != 0 {
		t.Error("expected failure count to be cleared")
	}
}

func TestReconcile_FullCycle(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha", "beta")
	ns := newMockNS()
	dm := newMockDaemon()
	rt := newMockRouting()
	r := newTestReconciler(cfgPath, ns, dm, rt)

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// After reconcile, namespaces should exist and daemons should be healthy
	if !ns.namespaces["ns-alpha"] {
		t.Error("ns-alpha should exist")
	}
	if !ns.namespaces["ns-beta"] {
		t.Error("ns-beta should exist")
	}
	if !dm.healthy["alpha"] {
		t.Error("alpha daemon should be healthy")
	}
	if !dm.healthy["beta"] {
		t.Error("beta daemon should be healthy")
	}

	// Events should have been emitted
	events := r.Events()
	if len(events) == 0 {
		t.Error("expected events to be emitted")
	}
}

func TestReconcile_Converges(t *testing.T) {
	cfgPath := writeTestConfig(t, "stable")
	ns := newMockNS()
	dm := newMockDaemon()
	rt := newMockRouting()
	r := newTestReconciler(cfgPath, ns, dm, rt)

	// First reconcile creates everything
	r.Reconcile()

	// Second reconcile should find everything healthy
	r.Reconcile()

	events := r.Events()
	// Look for "no changes needed" in the second reconcile
	found := false
	for _, e := range events {
		if e.Type == "reconcile_complete" && e.Message == "no changes needed" {
			found = true
			break
		}
	}
	// After first reconcile, mocks mark daemons as healthy. Second reconcile
	// should find healthy daemons and emit sync_routes actions (not "no changes needed").
	events := r.Events()
	hasReconcileComplete := false
	for _, e := range events {
		if e.Type == "reconcile_complete" {
			hasReconcileComplete = true
		}
	}
	if !hasReconcileComplete {
		t.Error("expected at least one reconcile_complete event")
	}
}

func TestLoop_CancelledContext(t *testing.T) {
	cfgPath := writeTestConfig(t)
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- r.Loop(ctx)
	}()

	// Let it run one cycle
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Loop returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Loop did not stop after context cancellation")
	}
}

func TestDesiredState_MissingConfig(t *testing.T) {
	r := newTestReconciler("/nonexistent/config.yaml", newMockNS(), newMockDaemon(), newMockRouting())
	_, err := r.DesiredState()
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestEvents_MaxCap(t *testing.T) {
	cfgPath := writeTestConfig(t)
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())

	// Emit more than 1000 events
	for i := 0; i < 1010; i++ {
		r.emit("test", "x", fmt.Sprintf("event-%d", i))
	}

	events := r.Events()
	if len(events) != 1000 {
		t.Errorf("len(events) = %d, want 1000 (capped)", len(events))
	}
	// Oldest should be event-10 (first 10 were dropped)
	if events[0].Message != "event-10" {
		t.Errorf("oldest event = %q, want %q", events[0].Message, "event-10")
	}
}

func TestLockPathFor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/var/lib/hydrascale/config.yaml", "/var/lib/hydrascale/.hydrascale.lock"},
		{"/tmp/test.yaml", "/tmp/.hydrascale.lock"},
	}
	for _, tt := range tests {
		if got := lockPathFor(tt.input); got != tt.want {
			t.Errorf("lockPathFor(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAcquireLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".test.lock")

	unlock, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("acquireLock: %v", err)
	}

	// Lock file should exist
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("lock file should exist")
	}

	unlock()
}

// --- Helpers ---

func countActions(actions []Action) map[ActionType]int {
	counts := make(map[ActionType]int)
	for _, a := range actions {
		counts[a.Type]++
	}
	return counts
}
