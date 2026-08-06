package namespaces

import (
	"errors"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"hydrascale/internal/execx"
)

// absent is the result that iptables returns for a delete of a rule that is not present.
var absent = execx.Result{
	Output: []byte("iptables: Bad rule (does a matching rule exist in that chain?)."),
	Err:    errors.New("exit status 1"),
}

// broken is the result that iptables returns for a delete that fails for another reason.
var broken = execx.Result{
	Output: []byte("iptables: Permission denied (you must be root)."),
	Err:    errors.New("exit status 4"),
}

func TestTeardownVethReturnsAnErrorWhenARuleDeleteFails(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"

	hostVeth, _ := VethNames(nsName)
	_, nsIP, _, _, err := VethIPs(infraSubnet, VethIndex(nsName))
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}

	rec := execx.NewRecorder(t)
	rec.Script(broken, "iptables", "-D", "FORWARD", "-i", hostVeth, "-j", "ACCEPT")
	rec.Script(execx.Result{}, "iptables", "-D", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	rec.Script(execx.Result{}, "iptables", "-t", "nat", "-D", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE")
	rec.Script(execx.Result{}, "ip", "link", "del", hostVeth)

	m := &RealManager{Runner: rec}
	if err := m.TeardownVeth(nsName, infraSubnet); err == nil {
		t.Fatal("TeardownVeth returned no error for a failed rule delete")
	}
	if len(rec.Calls()) != 4 {
		t.Errorf("TeardownVeth ran %d commands after a failed delete, want 4", len(rec.Calls()))
	}
}

func TestTeardownVethReturnsEveryFailedStepTogether(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"

	hostVeth, _ := VethNames(nsName)
	_, nsIP, _, _, err := VethIPs(infraSubnet, VethIndex(nsName))
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}

	rec := execx.NewRecorder(t)
	rec.Script(broken, "iptables", "-D", "FORWARD", "-i", hostVeth, "-j", "ACCEPT")
	rec.Script(broken, "iptables", "-D", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	rec.Script(broken, "iptables", "-t", "nat", "-D", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE")
	rec.Script(execx.Result{Err: errors.New("exit status 1")}, "ip", "link", "del", hostVeth)

	m := &RealManager{Runner: rec}
	err = m.TeardownVeth(nsName, infraSubnet)
	if err == nil {
		t.Fatal("TeardownVeth returned no error")
	}
	if got := strings.Count(err.Error(), "\n") + 1; got != 4 {
		t.Errorf("TeardownVeth returned %d errors, want 4: %v", got, err)
	}
}

func TestTeardownVethTreatsAnAbsentRuleAsSuccess(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"

	hostVeth, _ := VethNames(nsName)
	_, nsIP, _, _, err := VethIPs(infraSubnet, VethIndex(nsName))
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}

	rec := execx.NewRecorder(t)
	rec.Script(absent, "iptables", "-D", "FORWARD", "-i", hostVeth, "-j", "ACCEPT")
	rec.Script(absent, "iptables", "-D", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	rec.Script(absent, "iptables", "-t", "nat", "-D", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE")
	rec.Script(execx.Result{}, "ip", "link", "del", hostVeth)

	m := &RealManager{Runner: rec}
	if err := m.TeardownVeth(nsName, infraSubnet); err != nil {
		t.Fatalf("TeardownVeth: %v", err)
	}
}

func TestTeardownHostAccessReturnsEveryFailedStepTogether(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"
	const index = 1

	rec, m, want := hostAccessFixture(t, nsName, infraSubnet, index)
	for _, c := range want {
		rec.Script(broken, c.Name, c.Args...)
	}

	err := m.TeardownHostAccess(nsName, index, infraSubnet)
	if err == nil {
		t.Fatal("TeardownHostAccess returned no error for three failed rule deletes")
	}
	if got := strings.Count(err.Error(), "\n") + 1; got != 3 {
		t.Errorf("TeardownHostAccess returned %d errors, want 3: %v", got, err)
	}
	if len(rec.Calls()) != 3 {
		t.Errorf("TeardownHostAccess ran %d commands, want 3", len(rec.Calls()))
	}
}

func TestTeardownHostAccessTreatsAnAbsentRuleAsSuccess(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"
	const index = 1

	rec, m, want := hostAccessFixture(t, nsName, infraSubnet, index)
	for _, c := range want {
		rec.Script(absent, c.Name, c.Args...)
	}

	if err := m.TeardownHostAccess(nsName, index, infraSubnet); err != nil {
		t.Fatalf("TeardownHostAccess: %v", err)
	}
}

// hostAccessFixture returns a Recorder, a Manager, and the three rule deletes that
// TeardownHostAccess runs. The caller scripts each command.
func hostAccessFixture(t *testing.T, nsName, infraSubnet string, index int) (*execx.Recorder, *RealManager, []execx.Call) {
	t.Helper()

	_, nsVeth := VethNames(nsName)
	_, nsIPRange, _, _, err := VethIPs(infraSubnet, index)
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}

	want := []execx.Call{
		{Name: "ip", Args: []string{"netns", "exec", nsName, "iptables", "-t", "nat", "-D", "POSTROUTING", "-s", nsIPRange, "-o", "tailscale0", "-j", "MASQUERADE"}},
		{Name: "ip", Args: []string{"netns", "exec", nsName, "iptables", "-t", "nat", "-D", "PREROUTING", "-i", nsVeth, "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53"}},
		{Name: "ip", Args: []string{"netns", "exec", nsName, "iptables", "-t", "nat", "-D", "PREROUTING", "-i", nsVeth, "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53"}},
	}

	rec := execx.NewRecorder(t)
	return rec, &RealManager{Runner: rec}, want
}

// TestTeardownRemovesEveryRuleThatSetupAdded holds that the teardown list covers the setup
// list. The teardown list is longer, because it also removes the two FORWARD rules of
// version 0.9 that this version does not write.
func TestTeardownRemovesEveryRuleThatSetupAdded(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"
	// TeardownVeth derives the index from the namespace name, so the balance holds only
	// for the index that Create passes to SetupVeth.
	index := VethIndex(nsName)

	rec, m, _ := setupVethFixture(t, nsName, infraSubnet, index)
	scriptHostAccessSetup(t, rec, nsName, infraSubnet, index)
	if err := m.SetupVeth(nsName, index, infraSubnet); err != nil {
		t.Fatalf("SetupVeth: %v", err)
	}
	if err := m.SetupHostAccess(nsName, index, infraSubnet); err != nil {
		t.Fatalf("SetupHostAccess: %v", err)
	}
	added := ruleSpecs(t, rec.Calls(), "add")

	rec2, m2, deletes := hostAccessFixture(t, nsName, infraSubnet, index)
	for _, c := range deletes {
		rec2.Script(execx.Result{}, c.Name, c.Args...)
	}
	scriptVethTeardown(t, rec2, nsName, infraSubnet)
	if err := m2.TeardownHostAccess(nsName, index, infraSubnet); err != nil {
		t.Fatalf("TeardownHostAccess: %v", err)
	}
	if err := m2.TeardownVeth(nsName, infraSubnet); err != nil {
		t.Fatalf("TeardownVeth: %v", err)
	}
	removed := ruleSpecs(t, rec2.Calls(), "del")

	if len(added) == 0 {
		t.Fatal("setup added no rule")
	}
	for _, rule := range added {
		if !slices.Contains(removed, rule) {
			t.Errorf("setup added %q and teardown removed no such rule:\nremoved:\n%s",
				rule, strings.Join(removed, "\n"))
		}
	}
}

// scriptHostAccessSetup scripts each command that SetupHostAccess runs. It makes every
// check command fail, because a failed check is the condition under which SetupHostAccess
// writes the rule.
func scriptHostAccessSetup(t *testing.T, rec *execx.Recorder, nsName, infraSubnet string, index int) {
	t.Helper()

	_, nsVeth := VethNames(nsName)
	_, nsIPRange, _, _, err := VethIPs(infraSubnet, index)
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}

	for _, op := range []string{"-C", "-A"} {
		res := execx.Result{}
		if op == "-C" {
			res = absent
		}
		rec.Script(res, "ip", "netns", "exec", nsName, "iptables", "-t", "nat", op, "POSTROUTING", "-s", nsIPRange, "-o", "tailscale0", "-j", "MASQUERADE")
		for _, proto := range []string{"udp", "tcp"} {
			rec.Script(res, "ip", "netns", "exec", nsName, "iptables", "-t", "nat", op, "PREROUTING", "-i", nsVeth, "-p", proto, "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53")
		}
	}
}

// scriptVethTeardown scripts each command that TeardownVeth runs.
func scriptVethTeardown(t *testing.T, rec *execx.Recorder, nsName, infraSubnet string) {
	t.Helper()

	hostVeth, _ := VethNames(nsName)
	_, nsIP, _, _, err := VethIPs(infraSubnet, VethIndex(nsName))
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}
	rec.Script(execx.Result{}, "iptables", "-D", "FORWARD", "-i", hostVeth, "-j", "ACCEPT")
	rec.Script(execx.Result{}, "iptables", "-D", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	rec.Script(execx.Result{}, "iptables", "-t", "nat", "-D", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE")
	rec.Script(execx.Result{}, "ip", "link", "del", hostVeth)
}

// ruleSpecs returns one sorted key for each recorded iptables command whose operation is
// op. The key holds the namespace, the table, the chain, and the match arguments, and it
// holds no operation letter and no rule position. An add and the matching delete therefore
// produce the same key.
func ruleSpecs(t *testing.T, calls []execx.Call, op string) []string {
	t.Helper()

	var out []string
	for _, c := range calls {
		spec, got, ok := parseRule(c)
		if !ok || got != op {
			continue
		}
		out = append(out, spec)
	}
	sort.Strings(out)
	return out
}

// parseRule returns the key of one iptables command, and the operation: "add", "del", or
// "check". parseRule reports false for a command that is not an iptables rule command.
func parseRule(c execx.Call) (spec string, op string, ok bool) {
	fields := append([]string{c.Name}, c.Args...)

	ns := "host"
	if len(fields) >= 4 && fields[0] == "ip" && fields[1] == "netns" && fields[2] == "exec" {
		ns = fields[3]
		fields = fields[4:]
	}
	if len(fields) == 0 || fields[0] != "iptables" {
		return "", "", false
	}
	fields = fields[1:]

	table := "filter"
	if len(fields) >= 2 && fields[0] == "-t" {
		table = fields[1]
		fields = fields[2:]
	}
	if len(fields) < 2 {
		return "", "", false
	}

	switch fields[0] {
	case "-A", "-I":
		op = "add"
	case "-D":
		op = "del"
	case "-C":
		op = "check"
	default:
		return "", "", false
	}
	chain := fields[1]
	rest := fields[2:]
	// iptables -I takes an optional rule position after the chain. The position is not
	// part of the rule, so drop it before the key.
	if len(rest) > 0 {
		if _, err := strconv.Atoi(rest[0]); err == nil {
			rest = rest[1:]
		}
	}
	return ns + " " + table + " " + chain + " " + strings.Join(rest, " "), op, true
}

func TestReapStaleRulesRemovesARuleThatNamesAMissingVethDevice(t *testing.T) {
	const gone = "vhaaaaaaaaaaaa"

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("1: lo: <LOOPBACK> mtu 65536\n2: eth0: <BROADCAST> mtu 1500\n")},
		"ip", "-o", "link", "show")
	rec.Script(execx.Result{Output: []byte(
		"-P FORWARD ACCEPT\n" +
			"-A FORWARD -i " + gone + " -j ACCEPT\n")},
		"iptables", "-S", "FORWARD")
	rec.Script(execx.Result{}, "iptables", "-D", "FORWARD", "-i", gone, "-j", "ACCEPT")

	m := &RealManager{Runner: rec}
	count, err := m.ReapStaleRules()
	if err != nil {
		t.Fatalf("ReapStaleRules: %v", err)
	}
	if count != 1 {
		t.Errorf("ReapStaleRules removed %d rules, want 1", count)
	}
}

func TestReapStaleRulesKeepsARuleThatNamesALiveVethDevice(t *testing.T) {
	const live = "vhbbbbbbbbbbbb"

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("1: lo: <LOOPBACK> mtu 65536\n7: " + live + "@if6: <BROADCAST> mtu 1500\n")},
		"ip", "-o", "link", "show")
	rec.Script(execx.Result{Output: []byte(
		"-P FORWARD ACCEPT\n" +
			"-A FORWARD -i " + live + " -j ACCEPT\n" +
			"-A FORWARD -o " + live + " -m state --state RELATED,ESTABLISHED -j ACCEPT\n")},
		"iptables", "-S", "FORWARD")

	m := &RealManager{Runner: rec}
	count, err := m.ReapStaleRules()
	if err != nil {
		t.Fatalf("ReapStaleRules: %v", err)
	}
	if count != 0 {
		t.Errorf("ReapStaleRules removed %d rules, want 0", count)
	}
}

func TestRemoveLegacyForwardRulesRemovesBothRulesOfVersionZeroNine(t *testing.T) {
	const live = "vhbbbbbbbbbbbb"

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(
		"-P FORWARD DROP\n" +
			"-A FORWARD -i " + live + " -j ACCEPT\n" +
			"-A FORWARD -o " + live + " -m state --state RELATED,ESTABLISHED -j ACCEPT\n" +
			"-A FORWARD -j HYDRASCALE-FWD\n")},
		"iptables", "-S", "FORWARD")
	rec.Script(execx.Result{}, "iptables", "-D", "FORWARD", "-i", live, "-j", "ACCEPT")
	rec.Script(execx.Result{}, "iptables", "-D", "FORWARD", "-o", live, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")

	m := &RealManager{Runner: rec}
	count, err := m.RemoveLegacyForwardRules()
	if err != nil {
		t.Fatalf("RemoveLegacyForwardRules: %v", err)
	}
	if count != 2 {
		t.Errorf("RemoveLegacyForwardRules removed %d rules, want 2", count)
	}
}

func TestRemoveLegacyForwardRulesKeepsARuleOfTheOperator(t *testing.T) {
	const live = "vhbbbbbbbbbbbb"

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(
		"-P FORWARD DROP\n" +
			"-A FORWARD -i docker0 -j ACCEPT\n" +
			"-A FORWARD -i " + live + " -p tcp --dport 22 -j ACCEPT\n" +
			"-A FORWARD -i " + live + " -j DROP\n" +
			"-A FORWARD -j HYDRASCALE-FWD\n")},
		"iptables", "-S", "FORWARD")

	m := &RealManager{Runner: rec}
	count, err := m.RemoveLegacyForwardRules()
	if err != nil {
		t.Fatalf("RemoveLegacyForwardRules: %v", err)
	}
	if count != 0 {
		t.Errorf("RemoveLegacyForwardRules removed %d rules, want 0", count)
	}
}

func TestNamespaceForwardingReturnsTheValueInsideTheNamespace(t *testing.T) {
	const nsName = "ns-team-prod"

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("0\n")},
		"ip", "netns", "exec", nsName, "sysctl", "-n", "net.ipv4.ip_forward")

	m := &RealManager{Runner: rec}
	got, err := m.NamespaceForwarding(nsName)
	if err != nil {
		t.Fatalf("NamespaceForwarding: %v", err)
	}
	if got != "0" {
		t.Errorf("NamespaceForwarding = %q, want %q", got, "0")
	}
}

func TestNamespaceForwardingReportsANamespaceThatForwards(t *testing.T) {
	const nsName = "ns-team-prod"

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("1\n")},
		"ip", "netns", "exec", nsName, "sysctl", "-n", "net.ipv4.ip_forward")

	m := &RealManager{Runner: rec}
	got, err := m.NamespaceForwarding(nsName)
	if err != nil {
		t.Fatalf("NamespaceForwarding: %v", err)
	}
	if got != "1" {
		t.Errorf("NamespaceForwarding = %q, want %q", got, "1")
	}
}

func TestReapStaleRulesKeepsARuleOfTheOperator(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("1: lo: <LOOPBACK> mtu 65536\n")}, "ip", "-o", "link", "show")
	rec.Script(execx.Result{Output: []byte(
		"-P FORWARD DROP\n" +
			"-A FORWARD -i docker0 -j ACCEPT\n" +
			"-A FORWARD -s 10.0.0.0/8 -j ACCEPT\n")},
		"iptables", "-S", "FORWARD")

	m := &RealManager{Runner: rec}
	count, err := m.ReapStaleRules()
	if err != nil {
		t.Fatalf("ReapStaleRules: %v", err)
	}
	if count != 0 {
		t.Errorf("ReapStaleRules removed %d rules, want 0", count)
	}
}
