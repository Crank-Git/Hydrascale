package namespaces

import (
	"slices"
	"strings"
	"testing"

	"hydrascale/internal/execx"
)

// deletedRules returns the iptables commands that the recorder observed, as one line each.
func deletedRules(rec *execx.Recorder) []string {
	var lines []string
	for _, call := range rec.Calls() {
		if call.Name == "iptables" {
			lines = append(lines, call.String())
		}
	}
	return lines
}

func TestThePlanNamesTheSameIptablesRulesThatTheTeardownDeletes(t *testing.T) {
	// The console dialog states the rule count, therefore the plan and the teardown must
	// read one source. SA-3 and SA-14 of docs/security-audit.md record what a route and a
	// loader did when each held the same rule.
	const id, infraSubnet, stateBase = "team-prod", "10.200.0.0/16", "/var/lib/hydrascale/state"
	nsName := GetNamespaceName(id)

	plan, err := PlanRemoval(id, infraSubnet, stateBase)
	if err != nil {
		t.Fatalf("PlanRemoval: %v", err)
	}

	hostVeth, _ := VethNames(nsName)
	_, nsIP, _, _, err := VethIPs(infraSubnet, VethIndex(nsName))
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, "iptables", "-D", "FORWARD", "-i", hostVeth, "-j", "ACCEPT")
	rec.Script(execx.Result{}, "iptables", "-D", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	rec.Script(execx.Result{}, "iptables", "-t", "nat", "-D", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE")
	rec.Script(execx.Result{}, "ip", "link", "del", hostVeth)

	m := &RealManager{Runner: rec}
	if err := m.TeardownVeth(nsName, infraSubnet); err != nil {
		t.Fatalf("TeardownVeth: %v", err)
	}

	deleted := deletedRules(rec)
	if plan.RuleCount != len(deleted) {
		t.Errorf("the plan states %d rules and the teardown deleted %d", plan.RuleCount, len(deleted))
	}
	for _, rule := range deleted {
		if !slices.Contains(plan.Commands, rule) {
			t.Errorf("the teardown runs %q and the plan names no such command", rule)
		}
	}
}

func TestThePlanNamesTheNamespaceTheVethDeviceAndTheStateDirectory(t *testing.T) {
	const id, infraSubnet, stateBase = "team-prod", "10.200.0.0/16", "/var/lib/hydrascale/state"
	nsName := GetNamespaceName(id)
	hostVeth, _ := VethNames(nsName)

	plan, err := PlanRemoval(id, infraSubnet, stateBase)
	if err != nil {
		t.Fatalf("PlanRemoval: %v", err)
	}

	if plan.Namespace != nsName {
		t.Errorf("the plan names the namespace %q, want %q", plan.Namespace, nsName)
	}
	if plan.HostVeth != hostVeth {
		t.Errorf("the plan names the veth device %q, want %q", plan.HostVeth, hostVeth)
	}
	if want := stateBase + "/" + id; plan.StateDir != want {
		t.Errorf("the plan names the state directory %q, want %q", plan.StateDir, want)
	}

	joined := strings.Join(plan.Commands, "\n")
	for _, want := range []string{nsName, hostVeth, plan.StateDir} {
		if !strings.Contains(joined, want) {
			t.Errorf("the commands of the plan name no %s:\n%s", want, joined)
		}
	}
	if !slices.Contains(plan.Commands, "ip netns del "+nsName) {
		t.Errorf("the plan runs no namespace delete:\n%s", joined)
	}
	if !slices.Contains(plan.Commands, "ip link del "+hostVeth) {
		t.Errorf("the plan runs no veth delete:\n%s", joined)
	}
}

func TestThePlanRefusesAnIdentifierThatTheDaemonRefuses(t *testing.T) {
	// The plan builds a path under the state directory, therefore it applies the same
	// identifier rule that the configuration loader applies.
	for _, id := range []string{"", "My Net", "../../tmp/x", "."} {
		if _, err := PlanRemoval(id, "10.200.0.0/16", "/var/lib/hydrascale/state"); err == nil {
			t.Errorf("PlanRemoval accepted the identifier %q", id)
		}
	}
}
