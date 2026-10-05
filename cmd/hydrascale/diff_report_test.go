package main

import (
	"strings"
	"testing"

	"hydrascale/internal/reconciler"
)

// report returns the text that writeActionReport writes for one action list.
func report(t *testing.T, actions []reconciler.Action) string {
	t.Helper()
	return reportWithChains(t, actions, nil)
}

// reportWithChains returns the text that writeActionReport writes for one action list and
// the differences of the local rule chains.
func reportWithChains(t *testing.T, actions []reconciler.Action, chains []string) string {
	t.Helper()

	var out strings.Builder
	writeActionReport(&out, actions, chains, "%d action(s) needed:", "No changes needed. Desired state matches actual state.")
	return out.String()
}

// periodicActions returns the four actions that a healthy host of two tailnets produces on
// every cycle. Issue #274 measured exactly this list on the test host.
func periodicActions() []reconciler.Action {
	return []reconciler.Action{
		{Type: reconciler.ActionSyncRoutes, TailnetID: "jbones", NsName: "ns-jbones"},
		{Type: reconciler.ActionSyncHostAccess, TailnetID: "jbones", NsName: "ns-jbones"},
		{Type: reconciler.ActionSyncRoutes, TailnetID: "havoc", NsName: "ns-havoc"},
		{Type: reconciler.ActionSyncHostAccess, TailnetID: "havoc", NsName: "ns-havoc"},
	}
}

func TestTheReportStatesNoChangeForAHostThatOnlyHoldsThePeriodicActions(t *testing.T) {
	// Issue #274. The command states what would change. A converged host of two tailnets
	// reported "4 action(s) needed", therefore the operator read no difference between a
	// pending change and no pending change.
	got := report(t, periodicActions())

	if !strings.Contains(got, "No changes needed") {
		t.Errorf("the report states no converged result:\n%s", got)
	}
	if strings.Contains(got, "4 action(s) needed") {
		t.Errorf("the report counts a periodic action as a change:\n%s", got)
	}
}

func TestTheReportNamesThePeriodicActionsApart(t *testing.T) {
	// The operator still reads what the daemon runs on each cycle, and the report states
	// that each one writes only a difference.
	got := report(t, periodicActions())

	if !strings.Contains(got, "4 periodic sync action(s)") {
		t.Errorf("the report names no periodic action:\n%s", got)
	}
	if !strings.Contains(got, "sync_routes jbones (ns-jbones)") {
		t.Errorf("the report lists no periodic action:\n%s", got)
	}
}

func TestTheReportCountsOnlyTheActionsThatChangeTheHost(t *testing.T) {
	actions := append(periodicActions(),
		reconciler.Action{Type: reconciler.ActionCreateNS, TailnetID: "new", NsName: "ns-new"})

	got := report(t, actions)

	if !strings.Contains(got, "1 action(s) needed:") {
		t.Errorf("the report counts a number other than the one change:\n%s", got)
	}
	if !strings.Contains(got, "create_namespace new (ns-new)") {
		t.Errorf("the report names no change:\n%s", got)
	}
	if !strings.Contains(got, "4 periodic sync action(s)") {
		t.Errorf("the report drops the periodic actions when a change is present:\n%s", got)
	}
}

func TestTheReportStatesNoPeriodicSectionForAnEmptyList(t *testing.T) {
	got := report(t, nil)

	if !strings.Contains(got, "No changes needed") {
		t.Errorf("the report states no converged result:\n%s", got)
	}
	if strings.Contains(got, "periodic") {
		t.Errorf("the report names a periodic action for an empty list:\n%s", got)
	}
}

func TestTheReportCountsADifferenceOfTheLocalRuleChainsAsAChange(t *testing.T) {
	// Issue #407. The daemon failed to write HYDRASCALE-FWD on each tick, and the report
	// stated "No changes needed".
	chains := []string{
		"write chain HYDRASCALE-FWD: the host holds no chain or no marker rule",
		"insert jump rule: FORWARD holds no rule -j HYDRASCALE-FWD",
	}
	got := reportWithChains(t, periodicActions(), chains)

	if strings.Contains(got, "No changes needed") {
		t.Errorf("the report states no change for a host that holds no chain:\n%s", got)
	}
	if !strings.Contains(got, "2 action(s) needed:") {
		t.Errorf("the report counts a number other than the two chain differences:\n%s", got)
	}
	for _, c := range chains {
		if !strings.Contains(got, "  "+c+"\n") {
			t.Errorf("the report names no difference %q:\n%s", c, got)
		}
	}
}
