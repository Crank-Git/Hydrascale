package routing

import (
	"testing"

	"hydrascale/internal/execx"
)

// TestSyncRoutes_ReplacesAMissingRoute asserts the argument list of the command that adds
// a route. The Recorder fails the test when SyncRoutes runs an unscripted command.
func TestSyncRoutes_ReplacesAMissingRoute(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("10.0.0.0/8 dev tailscale0\n")},
		"ip", "netns", "exec", "ns0", "ip", "route", "show")
	rec.Script(execx.Result{},
		"ip", "netns", "exec", "ns0", "ip", "route", "replace", "172.16.0.0/12")
	rec.Script(execx.Result{},
		"ip", "netns", "exec", "ns0", "ip", "route", "del", "10.0.0.0/8")

	m := &RealManager{Runner: rec}
	desired := []Route{{Network: "172.16.0.0/12"}}
	if err := m.SyncRoutes("ns0", desired, "10.200.0.0/16"); err != nil {
		t.Fatalf("SyncRoutes returned an error: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 3 {
		t.Fatalf("SyncRoutes ran %d commands, want 3: %v", len(calls), calls)
	}
	want := "ip netns exec ns0 ip route replace 172.16.0.0/12"
	if got := calls[1].String(); got != want {
		t.Errorf("the second command is %q, want %q", got, want)
	}
}
