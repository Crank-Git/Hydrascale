package reconciler

import (
	"testing"
	"time"
)

// messagesFor returns the message of each event of the type and the tailnet, in order.
func messagesFor(r *Reconciler, eventType, tailnetID string) []string {
	var messages []string
	for _, event := range r.Events() {
		if event.Type == eventType && event.TailnetID == tailnetID {
			messages = append(messages, event.Message)
		}
	}
	return messages
}

// failures returns the message of each action_failed event, whatever the tailnet.
func failures(r *Reconciler) []string {
	var messages []string
	for _, event := range r.Events() {
		if event.Type == "action_failed" {
			messages = append(messages, event.TailnetID+": "+event.Message)
		}
	}
	return messages
}

func TestARefreshThatWaitsHoldsNoActionOfAnotherTailnet(t *testing.T) {
	// The tailnet alpha does not reach the Running state, and the tailnet beta does. The
	// tick must apply every action of beta. Before issue #223, the refresh of alpha held
	// the tick for 60 seconds and every action of beta waited for that deadline.
	cfgPath := writeTestConfig(t, "alpha", "beta")
	dm := newMockDaemon()
	dm.refreshReady["beta"] = true
	r := newTestReconciler(cfgPath, newMockNS(), dm, newMockRouting())

	start := time.Now()
	r.Apply([]Action{
		{Type: ActionRefreshDNS, TailnetID: "alpha", NsName: "ns-alpha"},
		{Type: ActionRefreshDNS, TailnetID: "beta", NsName: "ns-beta"},
		{Type: ActionSyncRoutes, TailnetID: "beta", NsName: "ns-beta"},
	})
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("the tick took %v, and no action waits for a deadline", elapsed)
	}
	if got := dm.refreshed(); len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Errorf("the refresh calls are %v, want [alpha beta]", got)
	}
	if indexOf(messagesFor(r, "action_ok", "beta"), string(ActionSyncRoutes)) < 0 {
		t.Error("the tick applied no sync_routes of beta, and the refresh of alpha waits")
	}
	if got := failures(r); len(got) != 0 {
		t.Errorf("the tick recorded action_failed: %v", got)
	}
	if count := r.FailureCounts()["alpha"]; count != 0 {
		t.Errorf("the failure count of alpha is %d, want 0; a wait is no failure", count)
	}
	if len(messagesFor(r, "dns.refresh_waits", "alpha")) != 1 {
		t.Error("the tick recorded no dns.refresh_waits for alpha")
	}
}

func TestTheRefreshRunsOnALaterTickWhenTailscaledReachesTheRunningState(t *testing.T) {
	// tailscaled reports the process as healthy before it reaches the Running state, so the
	// reconciler keeps the refresh planned until the refresh runs. Without this the daemon
	// loses the flip-flop of issue #22 and the resolver chain of the namespace stays wedged.
	cfgPath := writeTestConfig(t, "alpha")
	ns := newMockNS()
	ns.namespaces["ns-alpha"] = true
	dm := newMockDaemon()
	r := newTestReconciler(cfgPath, ns, dm, newMockRouting())

	if err := r.Reconcile(); err != nil {
		t.Fatalf("the first Reconcile: %v", err)
	}
	if got := dm.refreshed(); len(got) != 1 {
		t.Fatalf("the first tick made %d refresh calls, want 1: %v", len(got), got)
	}
	if !r.dnsRefreshWaits("alpha") {
		t.Fatal("the refresh of alpha waits for no later tick, and tailscaled is not Running")
	}

	dm.refreshReady["alpha"] = true
	if err := r.Reconcile(); err != nil {
		t.Fatalf("the second Reconcile: %v", err)
	}
	if got := dm.refreshed(); len(got) != 2 {
		t.Fatalf("the second tick made %d refresh calls in total, want 2: %v", len(got), got)
	}
	if r.dnsRefreshWaits("alpha") {
		t.Error("the refresh of alpha waits, and the refresh ran")
	}

	if err := r.Reconcile(); err != nil {
		t.Fatalf("the third Reconcile: %v", err)
	}
	if got := dm.refreshed(); len(got) != 2 {
		t.Errorf("the third tick made %d refresh calls in total, want 2: %v", len(got), got)
	}
}

func TestTheTickWritesTheChainsBeforeTheActionOfATailnet(t *testing.T) {
	// The shutdown removes both chains and the policy of the FORWARD chain is DROP, so a
	// namespace reaches no control server until the tick writes the chains again. A tick
	// that writes them after the actions therefore holds tailscaled out of the Running
	// state for the whole tick. Issue #223 names the defect.
	cfgPath := writeAccessConfig(t, "access:\n  mode: enforce\n", "alpha")
	log := &callLog{}
	ns := newMockNS()
	ns.namespaces["ns-alpha"] = true
	dm := newMockDaemon()
	dm.log = log
	r := newTestReconciler(cfgPath, ns, dm, newMockRouting())
	r.SetChainWriter(&fakeChainWriter{log: log})

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	names := log.recorded()
	write := indexOf(names, "access.write")
	start := indexOf(names, "start_daemon:alpha")
	if write < 0 {
		t.Fatalf("the tick wrote no chain: %v", names)
	}
	if start < 0 {
		t.Fatalf("the tick started no daemon: %v", names)
	}
	if write > start {
		t.Errorf("the tick wrote the chains after the action of alpha: %v", names)
	}
}
