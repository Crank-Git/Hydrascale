package namespaces

import (
	"errors"
	"testing"

	"hydrascale/internal/execx"
)

// setupVethFixture returns a Recorder and a Manager that runs every command through it.
// The fixture scripts each command that SetupVeth runs. It makes every iptables check
// command fail, because a failed check is the condition under which SetupVeth writes the
// rule.
func setupVethFixture(t *testing.T, nsName, infraSubnet string, index int) (*execx.Recorder, *RealManager, []execx.Call) {
	t.Helper()

	hostVeth, nsVeth := VethNames(nsName)
	hostIP, nsIP, hostGW, _, err := VethIPs(infraSubnet, index)
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}

	want := []execx.Call{
		{Name: "ip", Args: []string{"link", "add", hostVeth, "type", "veth", "peer", "name", nsVeth}},
		{Name: "ip", Args: []string{"link", "set", nsVeth, "netns", nsName}},
		{Name: "ip", Args: []string{"addr", "add", hostIP, "dev", hostVeth}},
		{Name: "ip", Args: []string{"link", "set", hostVeth, "up"}},
		{Name: "ip", Args: []string{"netns", "exec", nsName, "ip", "addr", "add", nsIP, "dev", nsVeth}},
		{Name: "ip", Args: []string{"netns", "exec", nsName, "ip", "link", "set", nsVeth, "up"}},
		{Name: "ip", Args: []string{"netns", "exec", nsName, "ip", "route", "add", "default", "via", hostGW, "dev", nsVeth}},
		{Name: "sysctl", Args: []string{"-w", "net.ipv4.conf." + hostVeth + ".forwarding=1"}},
		{Name: "iptables", Args: []string{"-C", "FORWARD", "-i", hostVeth, "-j", "ACCEPT"}},
		{Name: "iptables", Args: []string{"-I", "FORWARD", "1", "-i", hostVeth, "-j", "ACCEPT"}},
		{Name: "iptables", Args: []string{"-C", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"}},
		{Name: "iptables", Args: []string{"-I", "FORWARD", "1", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"}},
		{Name: "iptables", Args: []string{"-t", "nat", "-C", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE"}},
		{Name: "iptables", Args: []string{"-t", "nat", "-A", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE"}},
	}

	rec := execx.NewRecorder(t)
	for _, c := range want {
		res := execx.Result{}
		if len(c.Args) > 0 && (c.Args[0] == "-C" || (len(c.Args) > 2 && c.Args[2] == "-C")) {
			res.Err = errors.New("iptables: the rule does not exist")
		}
		rec.Script(res, c.Name, c.Args...)
	}

	return rec, &RealManager{Runner: rec}, want
}

func TestSetupVethRunsIPLinkAddWithTheExpectedArguments(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"
	const index = 1

	rec, m, _ := setupVethFixture(t, nsName, infraSubnet, index)
	if err := m.SetupVeth(nsName, index, infraSubnet); err != nil {
		t.Fatalf("SetupVeth: %v", err)
	}

	hostVeth, nsVeth := VethNames(nsName)
	want := execx.Call{
		Name: "ip",
		Args: []string{"link", "add", hostVeth, "type", "veth", "peer", "name", nsVeth},
	}

	calls := rec.Calls()
	if len(calls) == 0 {
		t.Fatal("SetupVeth ran no command")
	}
	if got := calls[0].String(); got != want.String() {
		t.Errorf("first command = %q, want %q", got, want.String())
	}
}

func TestSetupVethRunsTheFullCommandListInOrder(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"
	const index = 1

	rec, m, want := setupVethFixture(t, nsName, infraSubnet, index)
	if err := m.SetupVeth(nsName, index, infraSubnet); err != nil {
		t.Fatalf("SetupVeth: %v", err)
	}

	got := rec.Calls()
	if len(got) != len(want) {
		t.Fatalf("SetupVeth ran %d commands, want %d:\ngot:\n%s\nwant:\n%s",
			len(got), len(want), format(got), format(want))
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Errorf("command %d = %q, want %q", i, got[i].String(), want[i].String())
		}
	}
}

func TestSetupVethKeepsAnIptablesRuleThatAlreadyExists(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"
	const index = 1

	rec, m, want := setupVethFixture(t, nsName, infraSubnet, index)
	// A successful check means the rule is present, so SetupVeth writes no rule.
	for _, c := range want {
		if len(c.Args) > 0 && (c.Args[0] == "-C" || (len(c.Args) > 2 && c.Args[2] == "-C")) {
			rec.Script(execx.Result{}, c.Name, c.Args...)
		}
	}

	if err := m.SetupVeth(nsName, index, infraSubnet); err != nil {
		t.Fatalf("SetupVeth: %v", err)
	}

	for _, c := range rec.Calls() {
		if c.Name != "iptables" {
			continue
		}
		if c.Args[0] == "-I" || (len(c.Args) > 2 && c.Args[2] == "-A") {
			t.Errorf("SetupVeth wrote a rule that is already present: %s", c)
		}
	}
}

func TestSetupVethReturnsTheErrorOfAFailedCommand(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"
	const index = 1

	hostVeth, nsVeth := VethNames(nsName)
	rec := execx.NewRecorder(t)
	rec.Script(
		execx.Result{Output: []byte("RTNETLINK answers: File exists"), Err: errors.New("exit status 2")},
		"ip", "link", "add", hostVeth, "type", "veth", "peer", "name", nsVeth,
	)

	m := &RealManager{Runner: rec}
	err := m.SetupVeth(nsName, index, infraSubnet)
	if err == nil {
		t.Fatal("SetupVeth returned no error for a failed command")
	}
	if len(rec.Calls()) != 1 {
		t.Errorf("SetupVeth ran %d commands after a failure, want 1", len(rec.Calls()))
	}
}

func TestTeardownVethRunsTheFullCommandListInOrder(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"

	hostVeth, _ := VethNames(nsName)
	_, nsIP, _, _, err := VethIPs(infraSubnet, VethIndex(nsName))
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}

	want := []execx.Call{
		{Name: "iptables", Args: []string{"-D", "FORWARD", "-i", hostVeth, "-j", "ACCEPT"}},
		{Name: "iptables", Args: []string{"-D", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"}},
		{Name: "iptables", Args: []string{"-t", "nat", "-D", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE"}},
		{Name: "ip", Args: []string{"link", "del", hostVeth}},
	}

	rec := execx.NewRecorder(t)
	for _, c := range want {
		rec.Script(execx.Result{}, c.Name, c.Args...)
	}

	m := &RealManager{Runner: rec}
	if err := m.TeardownVeth(nsName, infraSubnet); err != nil {
		t.Fatalf("TeardownVeth: %v", err)
	}

	got := rec.Calls()
	if len(got) != len(want) {
		t.Fatalf("TeardownVeth ran %d commands, want %d:\ngot:\n%s\nwant:\n%s",
			len(got), len(want), format(got), format(want))
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Errorf("command %d = %q, want %q", i, got[i].String(), want[i].String())
		}
	}
}

func TestTeardownVethRemovesEveryRuleWhenADeleteFails(t *testing.T) {
	const nsName, infraSubnet = "ns-team-prod", "10.200.0.0/16"

	hostVeth, _ := VethNames(nsName)
	_, nsIP, _, _, err := VethIPs(infraSubnet, VethIndex(nsName))
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}

	fail := execx.Result{Err: errors.New("exit status 1")}
	rec := execx.NewRecorder(t)
	rec.Script(fail, "iptables", "-D", "FORWARD", "-i", hostVeth, "-j", "ACCEPT")
	rec.Script(fail, "iptables", "-D", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	rec.Script(fail, "iptables", "-t", "nat", "-D", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE")
	rec.Script(execx.Result{}, "ip", "link", "del", hostVeth)

	m := &RealManager{Runner: rec}
	if err := m.TeardownVeth(nsName, infraSubnet); err != nil {
		t.Fatalf("TeardownVeth: %v", err)
	}
	if len(rec.Calls()) != 4 {
		t.Errorf("TeardownVeth ran %d commands, want 4", len(rec.Calls()))
	}
}

func TestARealManagerWithNoRunnerUsesTheOSRunner(t *testing.T) {
	var m RealManager
	if _, ok := m.runner().(execx.OSRunner); !ok {
		t.Errorf("runner() = %T, want execx.OSRunner", m.runner())
	}
}

// format returns one line per call, for a failure message.
func format(calls []execx.Call) string {
	out := ""
	for _, c := range calls {
		out += "  " + c.String() + "\n"
	}
	return out
}
