package namespaces

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"hydrascale/internal/execx"
)

// The tests of this file use the veth index 1, so the host side addresses are fixed:
// 10.200.0.1 for IPv4 and fd5c:9a3e:7b10:1::1 for IPv6. A literal address in an argument
// list makes the test fail when the address form changes.
const (
	publishNS     = "ns-team-prod"
	publishSubnet = "10.200.0.0/16"
	publishIndex  = 1
)

// emptyPrerouting is the output of `iptables -t nat -S PREROUTING` for a chain that holds
// no rule.
var emptyPrerouting = execx.Result{Output: []byte("-P PREROUTING ACCEPT\n")}

// natCall returns the arguments of one `ip netns exec` command on the nat table of the
// test namespace. command is iptables or ip6tables.
func natCall(command string, args ...string) []string {
	return append([]string{"netns", "exec", publishNS, command, "-t", "nat"}, args...)
}

// tcp22v4 and tcp22v6 are the rule arguments of the published port tcp/22, as
// `iptables -S` prints them, after the operation letter.
var (
	tcp22v4 = []string{"PREROUTING", "-i", "tailscale0", "-p", "tcp", "-m", "tcp", "--dport", "22", "-j", "DNAT", "--to-destination", "10.200.0.1:22"}
	tcp22v6 = []string{"PREROUTING", "-i", "tailscale0", "-p", "tcp", "-m", "tcp", "--dport", "22", "-j", "DNAT", "--to-destination", "[fd5c:9a3e:7b10:1::1]:22"}
)

// withOp returns the arguments of one rule command with the operation letter op.
func withOp(op string, rule []string) []string {
	return append([]string{op}, rule...)
}

// publishFixture returns a Recorder that answers every command of SetupHostAccess except
// the published port rules, and a Manager that uses it.
func publishFixture(t *testing.T) (*execx.Recorder, *RealManager) {
	t.Helper()
	rec := execx.NewRecorder(t)
	scriptHostAccessSetup(t, rec, publishNS, publishSubnet, publishIndex)
	rec.Script(emptyPrerouting, "ip", natCall("iptables", "-S", "PREROUTING")...)
	rec.Script(emptyPrerouting, "ip", natCall("ip6tables", "-S", "PREROUTING")...)
	return rec, &RealManager{Runner: rec}
}

// opCalls returns each recorded command whose nat operation is op, for example "-A".
func opCalls(calls []execx.Call, op string) []execx.Call {
	var out []execx.Call
	for _, c := range calls {
		if len(c.Args) > 6 && c.Args[4] == "-t" && c.Args[5] == "nat" && c.Args[6] == op {
			out = append(out, c)
		}
	}
	return out
}

func TestASyncWritesTheExactIPv4AndIPv6RuleOfOnePublishedPort(t *testing.T) {
	rec, m := publishFixture(t)
	for _, family := range []struct {
		command string
		rule    []string
	}{{"iptables", tcp22v4}, {"ip6tables", tcp22v6}} {
		rec.Script(absent, "ip", natCall(family.command, withOp("-C", family.rule)...)...)
		rec.Script(execx.Result{}, "ip", natCall(family.command, withOp("-A", family.rule)...)...)
	}

	if err := m.SetupHostAccess(publishNS, publishIndex, publishSubnet, []string{"tcp/22"}, true); err != nil {
		t.Fatalf("SetupHostAccess: %v", err)
	}

	calls := rec.Calls()
	want := [][]string{
		{"netns", "exec", publishNS, "iptables", "-t", "nat", "-A", "PREROUTING", "-i", "tailscale0", "-p", "tcp", "-m", "tcp", "--dport", "22", "-j", "DNAT", "--to-destination", "10.200.0.1:22"},
		{"netns", "exec", publishNS, "ip6tables", "-t", "nat", "-A", "PREROUTING", "-i", "tailscale0", "-p", "tcp", "-m", "tcp", "--dport", "22", "-j", "DNAT", "--to-destination", "[fd5c:9a3e:7b10:1::1]:22"},
	}
	for _, args := range want {
		if !hasCall(calls, "ip", args...) {
			t.Errorf("SetupHostAccess did not run ip %s\ncalls: %v", strings.Join(args, " "), calls)
		}
	}
	for _, command := range []string{"iptables", "ip6tables"} {
		if !hasCall(calls, "ip", natCall(command, "-S", "PREROUTING")...) {
			t.Errorf("SetupHostAccess did not read the PREROUTING chain with %s", command)
		}
	}
}

func TestASecondSyncWritesNoPublishedPortRuleAgain(t *testing.T) {
	rec, m := publishFixture(t)
	rec.Script(execx.Result{Output: []byte("-P PREROUTING ACCEPT\n-A " + strings.Join(tcp22v4, " ") + "\n")},
		"ip", natCall("iptables", "-S", "PREROUTING")...)
	rec.Script(execx.Result{Output: []byte("-P PREROUTING ACCEPT\n-A " + strings.Join(tcp22v6, " ") + "\n")},
		"ip", natCall("ip6tables", "-S", "PREROUTING")...)
	rec.Script(execx.Result{}, "ip", natCall("iptables", withOp("-C", tcp22v4)...)...)
	rec.Script(execx.Result{}, "ip", natCall("ip6tables", withOp("-C", tcp22v6)...)...)

	if err := m.SetupHostAccess(publishNS, publishIndex, publishSubnet, []string{"tcp/22"}, true); err != nil {
		t.Fatalf("SetupHostAccess: %v", err)
	}

	for _, c := range opCalls(rec.Calls(), "-A") {
		if slices.Contains(c.Args, "tailscale0") && slices.Contains(c.Args, "DNAT") {
			t.Errorf("a second sync wrote a published port rule again: %s", c)
		}
	}
	if got := opCalls(rec.Calls(), "-D"); len(got) != 0 {
		t.Errorf("a second sync deleted a rule: %v", got)
	}
}

func TestASyncDeletesAPublishedPortRuleThatTheFileNoLongerNames(t *testing.T) {
	rec, m := publishFixture(t)
	_, nsVeth := VethNames(publishNS)
	stale := []string{"PREROUTING", "-i", "tailscale0", "-p", "tcp", "-m", "tcp", "--dport", "8080", "-j", "DNAT", "--to-destination", "10.200.0.1:8080"}
	chain := strings.Join([]string{
		"-P PREROUTING ACCEPT",
		"-A PREROUTING -i " + nsVeth + " -p udp -m udp --dport 53 -j DNAT --to-destination 100.100.100.100:53",
		"-A PREROUTING -i " + nsVeth + " -p tcp -m tcp --dport 53 -j DNAT --to-destination 100.100.100.100:53",
		"-A " + strings.Join(stale, " "),
		"-A " + strings.Join(tcp22v4, " "),
	}, "\n") + "\n"
	rec.Script(execx.Result{Output: []byte(chain)}, "ip", natCall("iptables", "-S", "PREROUTING")...)
	rec.Script(execx.Result{}, "ip", natCall("iptables", withOp("-C", tcp22v4)...)...)
	rec.Script(execx.Result{}, "ip", natCall("iptables", withOp("-D", stale)...)...)

	if err := m.SetupHostAccess(publishNS, publishIndex, publishSubnet, []string{"tcp/22"}, false); err != nil {
		t.Fatalf("SetupHostAccess: %v", err)
	}

	deletes := opCalls(rec.Calls(), "-D")
	if len(deletes) != 1 || !slices.Equal(deletes[0].Args, natCall("iptables", withOp("-D", stale)...)) {
		t.Errorf("the sync ran these deletes, want the tcp/8080 rule alone:\n%v", deletes)
	}
	for _, c := range deletes {
		if slices.Contains(c.Args, nsVeth) {
			t.Errorf("the sync deleted a DNS DNAT rule on the veth: %s", c)
		}
	}
}

func TestASyncWithoutTheIPv6PathWritesTheIPv4RuleAlone(t *testing.T) {
	rec, m := publishFixture(t)
	rec.Script(absent, "ip", natCall("iptables", withOp("-C", tcp22v4)...)...)
	rec.Script(execx.Result{}, "ip", natCall("iptables", withOp("-A", tcp22v4)...)...)

	if err := m.SetupHostAccess(publishNS, publishIndex, publishSubnet, []string{"tcp/22"}, false); err != nil {
		t.Fatalf("SetupHostAccess: %v", err)
	}

	for _, c := range rec.Calls() {
		if slices.Contains(c.Args, "ip6tables") {
			t.Errorf("a sync without the IPv6 path ran ip6tables: %s", c)
		}
	}
	if !hasCall(rec.Calls(), "ip", natCall("iptables", withOp("-A", tcp22v4)...)...) {
		t.Error("the sync wrote no IPv4 rule")
	}
}

func TestAnIP6tablesFailureReturnsAnErrorAndKeepsTheIPv4Rule(t *testing.T) {
	rec, m := publishFixture(t)
	missing := execx.Result{
		Output: []byte(`exec of "ip6tables" failed: No such file or directory`),
		Err:    errors.New("exit status 255"),
	}
	rec.Script(absent, "ip", natCall("iptables", withOp("-C", tcp22v4)...)...)
	rec.Script(execx.Result{}, "ip", natCall("iptables", withOp("-A", tcp22v4)...)...)
	rec.Script(missing, "ip", natCall("ip6tables", "-S", "PREROUTING")...)

	err := m.SetupHostAccess(publishNS, publishIndex, publishSubnet, []string{"tcp/22"}, true)
	if err == nil {
		t.Fatal("SetupHostAccess returned no error for a failed ip6tables read")
	}
	if !errors.Is(err, ErrPublishedPorts) {
		t.Errorf("the error is not ErrPublishedPorts: %v", err)
	}
	if !strings.Contains(err.Error(), "No such file or directory") {
		t.Errorf("the error does not carry the output of ip6tables: %v", err)
	}
	if !hasCall(rec.Calls(), "ip", natCall("iptables", withOp("-A", tcp22v4)...)...) {
		t.Error("the IPv4 rule was not written")
	}
}

func TestAFailedAppendReturnsAnErrorThatNamesTheRule(t *testing.T) {
	rec, m := publishFixture(t)
	rec.Script(absent, "ip", natCall("iptables", withOp("-C", tcp22v4)...)...)
	rec.Script(broken, "ip", natCall("iptables", withOp("-A", tcp22v4)...)...)

	err := m.SetupHostAccess(publishNS, publishIndex, publishSubnet, []string{"tcp/22"}, false)
	if !errors.Is(err, ErrPublishedPorts) {
		t.Fatalf("SetupHostAccess error = %v, want ErrPublishedPorts", err)
	}
	if !strings.Contains(err.Error(), "--dport 22") {
		t.Errorf("the error does not name the rule: %v", err)
	}
}

func TestASyncUsesTheParsedPortNumberForAnEntryWithALeadingZero(t *testing.T) {
	rec, m := publishFixture(t)
	rec.Script(absent, "ip", natCall("iptables", withOp("-C", tcp22v4)...)...)
	rec.Script(execx.Result{}, "ip", natCall("iptables", withOp("-A", tcp22v4)...)...)

	if err := m.SetupHostAccess(publishNS, publishIndex, publishSubnet, []string{"tcp/022"}, false); err != nil {
		t.Fatalf("SetupHostAccess: %v", err)
	}
	if !hasCall(rec.Calls(), "ip", natCall("iptables", withOp("-A", tcp22v4)...)...) {
		t.Errorf("the sync did not write --dport 22 for tcp/022; calls: %v", rec.Calls())
	}
}

func TestTheHostAccessTeardownDeletesEachPublishedPortRule(t *testing.T) {
	rec, m, want := hostAccessFixture(t, publishNS, publishSubnet, publishIndex)
	for _, c := range want {
		rec.Script(execx.Result{}, c.Name, c.Args...)
	}
	_, nsVeth := VethNames(publishNS)
	udp53 := []string{"PREROUTING", "-i", "tailscale0", "-p", "udp", "-m", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "10.200.0.1:53"}
	chain4 := strings.Join([]string{
		"-P PREROUTING ACCEPT",
		"-A PREROUTING -i " + nsVeth + " -p udp -m udp --dport 53 -j DNAT --to-destination 100.100.100.100:53",
		"-A " + strings.Join(tcp22v4, " "),
		"-A " + strings.Join(udp53, " "),
	}, "\n") + "\n"
	chain6 := "-P PREROUTING ACCEPT\n-A " + strings.Join(tcp22v6, " ") + "\n"
	rec.Script(execx.Result{Output: []byte(chain4)}, "ip", natCall("iptables", "-S", "PREROUTING")...)
	rec.Script(execx.Result{Output: []byte(chain6)}, "ip", natCall("ip6tables", "-S", "PREROUTING")...)
	published := [][]string{
		natCall("iptables", withOp("-D", tcp22v4)...),
		natCall("iptables", withOp("-D", udp53)...),
		natCall("ip6tables", withOp("-D", tcp22v6)...),
	}
	for _, args := range published {
		rec.Script(execx.Result{}, "ip", args...)
	}

	if err := m.TeardownHostAccess(publishNS, publishIndex, publishSubnet); err != nil {
		t.Fatalf("TeardownHostAccess: %v", err)
	}
	for _, args := range published {
		if !hasCall(rec.Calls(), "ip", args...) {
			t.Errorf("TeardownHostAccess did not run ip %s", strings.Join(args, " "))
		}
	}
}

func TestTheHostAccessTeardownReportsAFailedPublishedPortDeleteWithTheOthers(t *testing.T) {
	rec, m, want := hostAccessFixture(t, publishNS, publishSubnet, publishIndex)
	for _, c := range want {
		rec.Script(broken, c.Name, c.Args...)
	}
	rec.Script(execx.Result{Output: []byte("-A " + strings.Join(tcp22v4, " ") + "\n")}, "ip", natCall("iptables", "-S", "PREROUTING")...)
	rec.Script(broken, "ip", natCall("iptables", withOp("-D", tcp22v4)...)...)

	err := m.TeardownHostAccess(publishNS, publishIndex, publishSubnet)
	if err == nil {
		t.Fatal("TeardownHostAccess returned no error")
	}
	if got := strings.Count(err.Error(), "\n") + 1; got != 4 {
		t.Errorf("TeardownHostAccess returned %d errors, want 4: %v", got, err)
	}
	if !strings.Contains(err.Error(), "--dport 22") {
		t.Errorf("the error does not name the published port rule: %v", err)
	}
}
