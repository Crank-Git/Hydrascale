package hostaccess

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"hydrascale/internal/execx"
)

// syncFixture returns a Recorder and a Manager that sends every command to it. The
// fixture scripts each command that SyncHostRoutes runs for one peer that holds one IPv4
// address and one IPv6 address.
//
// routeTable is the value of the configuration key `route_table`. The value 0 gives the
// main table and no routing policy rule, which is the behaviour of version 0.9.
func syncFixture(t *testing.T, routeTable int) (*execx.Recorder, *Manager, TailnetPeers, []execx.Call) {
	t.Helper()

	peers := TailnetPeers{
		TailnetID:   "corp",
		VethGateway: "10.200.0.1",
		VethHost:    "vh001",
		NsName:      "ns-corp",
		Peers: []Peer{{
			Hostname: "laptop",
			IPv4:     "100.64.0.1",
			IPv6:     "fd7a:115c:a1e0::1",
			Online:   true,
		}},
	}

	// table holds the `table <n>` arguments that every host route command carries. It is
	// empty when the configuration declares no route table.
	var table []string
	if routeTable != 0 {
		table = []string{"table", strconv.Itoa(routeTable)}
	}
	// withTable returns one argument list with the table arguments appended.
	withTable := func(args ...string) []string { return append(args, table...) }

	rec := execx.NewRecorder(t)
	var want []execx.Call
	if routeTable != 0 {
		lookup := strconv.Itoa(routeTable)
		rec.Script(execx.Result{Output: []byte("0:\tfrom all lookup local\n32766:\tfrom all lookup main\n")},
			"ip", "rule", "show")
		rec.Script(execx.Result{}, "ip", "rule", "add", "priority", "32000", "from", "all", "lookup", lookup)
		rec.Script(execx.Result{Output: []byte("0:\tfrom all lookup local\n32766:\tfrom all lookup main\n")},
			"ip", "-6", "rule", "show")
		rec.Script(execx.Result{}, "ip", "-6", "rule", "add", "priority", "32000", "from", "all", "lookup", lookup)
		want = append(want,
			execx.Call{Name: "ip", Args: []string{"rule", "show"}},
			execx.Call{Name: "ip", Args: []string{"rule", "add", "priority", "32000", "from", "all", "lookup", lookup}},
			execx.Call{Name: "ip", Args: []string{"-6", "rule", "show"}},
			execx.Call{Name: "ip", Args: []string{"-6", "rule", "add", "priority", "32000", "from", "all", "lookup", lookup}},
		)
	}
	rec.Script(execx.Result{}, "ip", "netns", "exec", "ns-corp", "ip", "route", "show", "table", "52")
	rec.Script(execx.Result{}, "ip", "netns", "exec", "ns-corp", "ip", "-6", "route", "show", "table", "52")
	rec.Script(execx.Result{Output: []byte("100.64.0.1 via 10.200.0.1 dev vh001 src 10.200.0.2\n")},
		"ip", "route", "get", "100.64.0.1")
	rec.Script(execx.Result{Output: []byte("fd7a:115c:a1e0::1 via fe80::1 dev vh001 src fd7a::2\n")},
		"ip", "-6", "route", "get", "fd7a:115c:a1e0::1")
	rec.Script(execx.Result{Output: []byte("10.9.9.0/24 dev vh001 scope link\n")},
		"ip", withTable("route", "show")...)
	rec.Script(execx.Result{}, "ip", withTable("-6", "route", "show")...)
	rec.Script(execx.Result{}, "ip",
		withTable("route", "replace", "100.64.0.1", "via", "10.200.0.1", "dev", "vh001")...)
	rec.Script(execx.Result{}, "ip", withTable("route", "del", "10.9.9.0/24")...)
	rec.Script(execx.Result{}, "ip",
		withTable("-6", "route", "replace", "fd7a:115c:a1e0::1", "dev", "vh001")...)

	want = append(want,
		execx.Call{Name: "ip", Args: []string{"netns", "exec", "ns-corp", "ip", "route", "show", "table", "52"}},
		execx.Call{Name: "ip", Args: []string{"netns", "exec", "ns-corp", "ip", "-6", "route", "show", "table", "52"}},
		execx.Call{Name: "ip", Args: []string{"route", "get", "100.64.0.1"}},
		execx.Call{Name: "ip", Args: []string{"-6", "route", "get", "fd7a:115c:a1e0::1"}},
		execx.Call{Name: "ip", Args: withTable("route", "show")},
		execx.Call{Name: "ip", Args: withTable("-6", "route", "show")},
		execx.Call{Name: "ip", Args: withTable("route", "replace", "100.64.0.1", "via", "10.200.0.1", "dev", "vh001")},
		execx.Call{Name: "ip", Args: withTable("route", "del", "10.9.9.0/24")},
		execx.Call{Name: "ip", Args: withTable("-6", "route", "replace", "fd7a:115c:a1e0::1", "dev", "vh001")},
	)

	m := NewManager("hosts", filepath.Join(t.TempDir(), "hosts"), "10.200.0.0/16", routeTable)
	m.Runner = rec
	return rec, m, peers, want
}

func TestSyncHostRoutesRunsTheFullCommandListInOrder(t *testing.T) {
	rec, m, peers, want := syncFixture(t, 0)

	if err := m.SyncHostRoutes(peers); err != nil {
		t.Fatalf("SyncHostRoutes: %v", err)
	}

	got := rec.Calls()
	if len(got) != len(want) {
		t.Fatalf("SyncHostRoutes ran %d commands, want %d:\n%s", len(got), len(want), callList(got))
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Errorf("command %d = %q, want %q", i, got[i].String(), want[i].String())
		}
	}
}

func TestEveryCommandOfARouteSynchronisationCarriesADeadline(t *testing.T) {
	rec, m, peers, want := syncFixture(t, 0)

	if err := m.SyncHostRoutes(peers); err != nil {
		t.Fatalf("SyncHostRoutes: %v", err)
	}

	deadlines := rec.Deadlines()
	if len(deadlines) != len(want) {
		t.Fatalf("the recorder holds %d contexts, want %d", len(deadlines), len(want))
	}
	for i, ok := range deadlines {
		if !ok {
			t.Errorf("command %d carries no deadline: %s", i, rec.Calls()[i])
		}
	}
}

func TestARouteDestinationThatIsNotAnAddressAndNotACIDRReachesNoCommand(t *testing.T) {
	const hostile = "-lo"

	peers := TailnetPeers{
		TailnetID:   "corp",
		VethGateway: "10.200.0.1",
		VethHost:    "vh001",
		NsName:      "ns-corp",
	}

	rec := execx.NewRecorder(t)
	// The control server advertises the hostile destination, so it arrives in table 52.
	rec.Script(execx.Result{Output: []byte(hostile + " via 100.64.0.1 dev tailscale0\n")},
		"ip", "netns", "exec", "ns-corp", "ip", "route", "show", "table", "52")
	rec.Script(execx.Result{Output: []byte(hostile + " dev tailscale0\n")},
		"ip", "netns", "exec", "ns-corp", "ip", "-6", "route", "show", "table", "52")
	// The host route table holds the same text, so the removal path sees it too.
	rec.Script(execx.Result{Output: []byte(hostile + " dev vh001 scope link\n")}, "ip", "route", "show")
	rec.Script(execx.Result{Output: []byte(hostile + " dev vh001\n")}, "ip", "-6", "route", "show")

	m := NewManager("hosts", filepath.Join(t.TempDir(), "hosts"), "10.200.0.0/16", 0)
	m.Runner = rec

	if err := m.SyncHostRoutes(peers); err != nil {
		t.Fatalf("SyncHostRoutes: %v", err)
	}

	for _, c := range rec.Calls() {
		for _, a := range c.Args {
			if a == hostile {
				t.Fatalf("the hostile destination reached a command: %s", c)
			}
		}
	}
}

func TestValidRouteDestAcceptsAnAddressAndACIDRAndRejectsOtherText(t *testing.T) {
	valid := []string{"100.64.0.1", "192.168.1.0/24", "fd7a:115c:a1e0::1", "fd7a:115c:a1e0::/48"}
	for _, d := range valid {
		if !validRouteDest(d) {
			t.Errorf("validRouteDest(%q) = false, want true", d)
		}
	}
	invalid := []string{"", "-lo", "--dev", "default", "100.64.0.1 extra", "no-such-thing"}
	for _, d := range invalid {
		if validRouteDest(d) {
			t.Errorf("validRouteDest(%q) = true, want false", d)
		}
	}
}

// callList returns the commands as one block of text, for a failure message.
func callList(calls []execx.Call) string {
	lines := make([]string, len(calls))
	for i, c := range calls {
		lines[i] = c.String()
	}
	return strings.Join(lines, "\n")
}

// quietRunner answers every command with empty output and no error. A test that asserts a
// file uses it, so that the test changes no state on the host that runs the test suite.
type quietRunner struct{}

func (quietRunner) Run(context.Context, string, ...string) ([]byte, error) { return nil, nil }

// skipFixture returns a Manager whose runner answers `ip -6 route get` with one line.
func skipFixture(t *testing.T, dest, answer string) *Manager {
	t.Helper()

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(answer)}, "ip", "-6", "route", "get", dest)
	return &Manager{Runner: rec}
}

func TestTheDaemonWritesAnIPv6RouteThatAPolicyTableAnswersFirstForNoTailnet(t *testing.T) {
	// Issue #273. The host runs its own tailscaled beside the daemon. That daemon writes
	// `fd7a:115c:a1e0::/48 dev tailscale0` into the routing table 52, and `ip rule` places
	// the lookup of table 52 before the lookup of the main table. A route in the main table
	// therefore reaches no packet, and the daemon states that rather than writing one.
	const dest = "fd7a:115c:a1e0::2735:6b25"
	m := skipFixture(t, dest,
		"fd7a:115c:a1e0::2735:6b25 from :: dev tailscale0 table 52 src fd7a:115c:a1e0::b936:fe73 metric 1024 pref medium\n")

	reason := m.skipReasonForRoute(dest, true)
	if reason == "" {
		t.Fatal("skipReasonForRoute returned no reason for a destination that the table 52 answers")
	}
	if !strings.Contains(reason, "52") {
		t.Errorf("the reason %q names no routing table", reason)
	}
	// The address is not on a directly connected network, and the reason must not say so.
	if strings.Contains(reason, "directly connected") {
		t.Errorf("the reason %q states a directly connected network for an address that a policy table answers", reason)
	}
}

func TestTheDaemonWritesNoRouteForADirectlyConnectedNetwork(t *testing.T) {
	// The guard that issue #21 added. The host reaches the address over an attached
	// network, therefore a route of the daemon would take that traffic.
	const dest = "fd00:abcd::5"
	m := skipFixture(t, dest, "fd00:abcd::5 dev eth0 src fd00:abcd::2 metric 256\n")

	reason := m.skipReasonForRoute(dest, true)
	if !strings.Contains(reason, "directly connected") {
		t.Errorf("the reason %q states no directly connected network", reason)
	}
	if !strings.Contains(reason, "eth0") {
		t.Errorf("the reason %q names no device", reason)
	}
}

func TestTheDaemonWritesARouteThatItsOwnDeviceAnswers(t *testing.T) {
	// The daemon already holds the route, therefore a replace is safe.
	const dest = "fd7a:115c:a1e0::1"
	m := skipFixture(t, dest, "fd7a:115c:a1e0::1 via fe80::1 dev vh001 src fd7a::2\n")

	if reason := m.skipReasonForRoute(dest, true); reason != "" {
		t.Errorf("skipReasonForRoute returned %q for a device of the daemon", reason)
	}
}

func TestTheDaemonWritesARouteThatTheHostCannotResolve(t *testing.T) {
	// The kernel resolves the destination over no route, therefore the daemon replaces
	// nothing and it writes its own route.
	const dest = "fd7a:115c:a1e0::9"
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Err: errors.New("exit status 2")}, "ip", "-6", "route", "get", dest)
	m := &Manager{Runner: rec}

	if reason := m.skipReasonForRoute(dest, true); reason != "" {
		t.Errorf("skipReasonForRoute returned %q for a destination that the host resolves over no route", reason)
	}
}

func TestSyncHostRoutesWritesEveryCommandIntoTheDeclaredRouteTable(t *testing.T) {
	rec, m, peers, want := syncFixture(t, 53)

	if err := m.SyncHostRoutes(peers); err != nil {
		t.Fatalf("SyncHostRoutes: %v", err)
	}

	got := rec.Calls()
	if len(got) != len(want) {
		t.Fatalf("SyncHostRoutes ran %d commands, want %d:\n%s", len(got), len(want), callList(got))
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Errorf("command %d = %q, want %q", i, got[i].String(), want[i].String())
		}
	}
}

func TestSyncHostRoutesRunsNoRuleCommandWhenNoRouteTableIsDeclared(t *testing.T) {
	rec, m, peers, _ := syncFixture(t, 0)

	if err := m.SyncHostRoutes(peers); err != nil {
		t.Fatalf("SyncHostRoutes: %v", err)
	}

	for _, c := range rec.Calls() {
		for _, a := range c.Args {
			if a == "rule" {
				t.Fatalf("a rule command ran for a manager that declares no route table: %s", c)
			}
			if a == "table" && c.Args[0] != "netns" {
				t.Fatalf("a host route command names a table: %s", c)
			}
		}
	}
}

// ruleList is the `ip rule show` output of a host that already holds the rule of the daemon.
const ruleList = "0:\tfrom all lookup local\n" +
	"32000:\tfrom all lookup 53\n" +
	"32766:\tfrom all lookup main\n" +
	"32767:\tfrom all lookup default\n"

func TestTheDaemonAddsNoSecondRuleWhenTheRuleIsAlreadyPresent(t *testing.T) {
	// `ip rule add` writes a second copy of a rule that is already present, and the
	// reconcile cycle runs every ten seconds. The Recorder fails the test for a command
	// that the test did not script, so an `ip rule add` here fails the test.
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(ruleList)}, "ip", "rule", "show")
	m := &Manager{Runner: rec, routeTable: 53}

	if err := m.ensureHostRouteRule(false); err != nil {
		t.Fatalf("ensureHostRouteRule: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("ensureHostRouteRule ran %d commands, want 1:\n%s", len(calls), callList(calls))
	}
	if calls[0].String() != "ip rule show" {
		t.Errorf("command 0 = %q, want %q", calls[0].String(), "ip rule show")
	}
}

func TestTheDaemonAddsTheRuleWhenTheHostHoldsNone(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("0:\tfrom all lookup local\n32766:\tfrom all lookup main\n")},
		"ip", "-6", "rule", "show")
	rec.Script(execx.Result{}, "ip", "-6", "rule", "add", "priority", "32000", "from", "all", "lookup", "53")
	m := &Manager{Runner: rec, routeTable: 53}

	if err := m.ensureHostRouteRule(true); err != nil {
		t.Fatalf("ensureHostRouteRule: %v", err)
	}

	want := []string{
		"ip -6 rule show",
		"ip -6 rule add priority 32000 from all lookup 53",
	}
	calls := rec.Calls()
	if len(calls) != len(want) {
		t.Fatalf("ensureHostRouteRule ran %d commands, want %d:\n%s", len(calls), len(want), callList(calls))
	}
	for i := range want {
		if calls[i].String() != want[i] {
			t.Errorf("command %d = %q, want %q", i, calls[i].String(), want[i])
		}
	}
}

func TestTheDaemonReportsARuleOfAnotherTableAtItsOwnPriority(t *testing.T) {
	// The operator holds the priority 32000. The daemon adds no second rule at that
	// priority, because two rules at one priority hide the order that the operator reads.
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("0:\tfrom all lookup local\n32000:\tfrom all lookup 99\n")},
		"ip", "rule", "show")
	m := &Manager{Runner: rec, routeTable: 53}

	err := m.ensureHostRouteRule(false)
	if err == nil {
		t.Fatal("ensureHostRouteRule returned no error for a rule of another table")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("the error %q names no table of the operator", err)
	}
	if len(rec.Calls()) != 1 {
		t.Errorf("ensureHostRouteRule ran %d commands, want 1:\n%s", len(rec.Calls()), callList(rec.Calls()))
	}
}

func TestRemoveHostRouteTableRemovesTheRulesAndEmptiesTheTable(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, "ip", "rule", "del", "priority", "32000", "from", "all", "lookup", "53")
	rec.Script(execx.Result{}, "ip", "-6", "rule", "del", "priority", "32000", "from", "all", "lookup", "53")
	rec.Script(execx.Result{}, "ip", "route", "flush", "table", "53")
	rec.Script(execx.Result{}, "ip", "-6", "route", "flush", "table", "53")
	m := &Manager{Runner: rec, routeTable: 53}

	if err := m.RemoveHostRouteTable(); err != nil {
		t.Fatalf("RemoveHostRouteTable: %v", err)
	}

	// The rule goes before the flush, so that no packet reaches a table that holds a part
	// of the routes.
	want := []string{
		"ip rule del priority 32000 from all lookup 53",
		"ip -6 rule del priority 32000 from all lookup 53",
		"ip route flush table 53",
		"ip -6 route flush table 53",
	}
	calls := rec.Calls()
	if len(calls) != len(want) {
		t.Fatalf("RemoveHostRouteTable ran %d commands, want %d:\n%s", len(calls), len(want), callList(calls))
	}
	for i := range want {
		if calls[i].String() != want[i] {
			t.Errorf("command %d = %q, want %q", i, calls[i].String(), want[i])
		}
	}
}

func TestRemoveHostRouteTableAcceptsARuleThatIsNotPresent(t *testing.T) {
	notPresent := execx.Result{
		Output: []byte("RTNETLINK answers: No such file or directory\n"),
		Err:    errors.New("exit status 2"),
	}
	rec := execx.NewRecorder(t)
	rec.Script(notPresent, "ip", "rule", "del", "priority", "32000", "from", "all", "lookup", "53")
	rec.Script(notPresent, "ip", "-6", "rule", "del", "priority", "32000", "from", "all", "lookup", "53")
	rec.Script(execx.Result{}, "ip", "route", "flush", "table", "53")
	rec.Script(execx.Result{}, "ip", "-6", "route", "flush", "table", "53")
	m := &Manager{Runner: rec, routeTable: 53}

	if err := m.RemoveHostRouteTable(); err != nil {
		t.Errorf("RemoveHostRouteTable returned %v for a rule that is not present, want no error", err)
	}
}

func TestRemoveHostRouteTableReturnsEveryFailureOfEveryStep(t *testing.T) {
	// A step that fails does not stop the remaining steps. The Recorder fails the test for
	// an unscripted command, therefore every step reaches the runner.
	fail := execx.Result{Output: []byte("RTNETLINK answers: Operation not permitted\n"), Err: errors.New("exit status 2")}
	rec := execx.NewRecorder(t)
	rec.Script(fail, "ip", "rule", "del", "priority", "32000", "from", "all", "lookup", "53")
	rec.Script(fail, "ip", "-6", "rule", "del", "priority", "32000", "from", "all", "lookup", "53")
	rec.Script(fail, "ip", "route", "flush", "table", "53")
	rec.Script(fail, "ip", "-6", "route", "flush", "table", "53")
	m := &Manager{Runner: rec, routeTable: 53}

	err := m.RemoveHostRouteTable()
	if err == nil {
		t.Fatal("RemoveHostRouteTable returned no error for four failed steps")
	}
	if len(rec.Calls()) != 4 {
		t.Errorf("RemoveHostRouteTable ran %d commands, want 4:\n%s", len(rec.Calls()), callList(rec.Calls()))
	}
	if n := strings.Count(err.Error(), "Operation not permitted"); n != 4 {
		t.Errorf("the error names %d failures, want 4: %v", n, err)
	}
}

func TestRemoveHostRouteTableRunsNoCommandWhenNoRouteTableIsDeclared(t *testing.T) {
	rec := execx.NewRecorder(t)
	m := &Manager{Runner: rec}

	if err := m.RemoveHostRouteTable(); err != nil {
		t.Fatalf("RemoveHostRouteTable: %v", err)
	}
	if len(rec.Calls()) != 0 {
		t.Errorf("RemoveHostRouteTable ran %d commands for a manager that declares no route table:\n%s",
			len(rec.Calls()), callList(rec.Calls()))
	}
}

func TestRemoveAllHostRoutesReadsAndDeletesInTheDeclaredRouteTable(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("100.64.0.1 via 10.200.0.1 dev vh001\n")},
		"ip", "route", "show", "table", "53")
	rec.Script(execx.Result{}, "ip", "route", "del", "100.64.0.1", "table", "53")
	rec.Script(execx.Result{Output: []byte("fd7a:115c:a1e0::1 dev vh001 metric 1024 pref medium\n")},
		"ip", "-6", "route", "show", "table", "53")
	rec.Script(execx.Result{}, "ip", "-6", "route", "del", "fd7a:115c:a1e0::1", "table", "53")
	m := &Manager{Runner: rec, routeTable: 53, infraSubnet: "10.200.0.0/16"}

	if err := m.RemoveAllHostRoutes("vh001"); err != nil {
		t.Fatalf("RemoveAllHostRoutes: %v", err)
	}

	want := []string{
		"ip route show table 53",
		"ip route del 100.64.0.1 table 53",
		"ip -6 route show table 53",
		"ip -6 route del fd7a:115c:a1e0::1 table 53",
	}
	calls := rec.Calls()
	if len(calls) != len(want) {
		t.Fatalf("RemoveAllHostRoutes ran %d commands, want %d:\n%s", len(calls), len(want), callList(calls))
	}
	for i := range want {
		if calls[i].String() != want[i] {
			t.Errorf("command %d = %q, want %q", i, calls[i].String(), want[i])
		}
	}
}

func TestHostRouteRuleStateReadsTheTableOfTheRuleAtThePriorityOfTheDaemon(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantTable   string
		wantPresent bool
	}{
		{
			name:        "the host holds the rule of the daemon",
			input:       ruleList,
			wantTable:   "53",
			wantPresent: true,
		},
		{
			name:        "the host holds no rule at that priority",
			input:       "0:\tfrom all lookup local\n32766:\tfrom all lookup main\n",
			wantPresent: false,
		},
		{
			name:        "another table holds that priority",
			input:       "32000:\tfrom all lookup 99\n",
			wantTable:   "99",
			wantPresent: true,
		},
		{
			name:        "tailscaled holds its own rules",
			input:       "5210:\tfrom all fwmark 0x80000/0xff0000 lookup main\n5230:\tfrom all fwmark 0x80000/0xff0000 lookup 52\n",
			wantPresent: false,
		},
		{
			name:        "the rule at that priority names no table",
			input:       "32000:\tfrom all suppress_prefixlength 0\n",
			wantTable:   "",
			wantPresent: true,
		},
		{
			name:        "the output is empty",
			input:       "",
			wantPresent: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table, present := hostRouteRuleState(tt.input)
			if present != tt.wantPresent {
				t.Errorf("present = %v, want %v", present, tt.wantPresent)
			}
			if table != tt.wantTable {
				t.Errorf("table = %q, want %q", table, tt.wantTable)
			}
		})
	}
}

// The kernel creates an IPv4 FIB table with the first route that reaches it, therefore
// `ip route show table 53` fails until the daemon writes its first route. The daemon
// starts from that state, therefore it must read the failure as an empty table and write
// the routes of the tailnet.
func TestSyncHostRoutesReadsAMissingFibTableAsAnEmptyTable(t *testing.T) {
	peers := TailnetPeers{
		TailnetID:   "corp",
		VethGateway: "10.200.0.1",
		VethHost:    "vh001",
		NsName:      "ns-corp",
		Peers:       []Peer{{Hostname: "laptop", IPv4: "100.64.0.1", Online: true}},
	}

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("0:\tfrom all lookup local\n32766:\tfrom all lookup main\n")},
		"ip", "rule", "show")
	rec.Script(execx.Result{}, "ip", "rule", "add", "priority", "32000", "from", "all", "lookup", "53")
	rec.Script(execx.Result{Output: []byte("0:\tfrom all lookup local\n32766:\tfrom all lookup main\n")},
		"ip", "-6", "rule", "show")
	rec.Script(execx.Result{}, "ip", "-6", "rule", "add", "priority", "32000", "from", "all", "lookup", "53")
	rec.Script(execx.Result{}, "ip", "netns", "exec", "ns-corp", "ip", "route", "show", "table", "52")
	rec.Script(execx.Result{}, "ip", "netns", "exec", "ns-corp", "ip", "-6", "route", "show", "table", "52")
	rec.Script(execx.Result{Output: []byte("100.64.0.1 via 10.200.0.1 dev vh001 src 10.200.0.2\n")},
		"ip", "route", "get", "100.64.0.1")
	// This is the answer of the host for a table that holds no route.
	rec.Script(execx.Result{
		Output: []byte("Error: ipv4: FIB table does not exist.\nDump terminated\n"),
		Err:    errors.New("exit status 2"),
	}, "ip", "route", "show", "table", "53")
	rec.Script(execx.Result{}, "ip", "-6", "route", "show", "table", "53")
	rec.Script(execx.Result{}, "ip",
		"route", "replace", "100.64.0.1", "via", "10.200.0.1", "dev", "vh001", "table", "53")

	m := NewManager("hosts", filepath.Join(t.TempDir(), "hosts"), "10.200.0.0/16", 53)
	m.Runner = rec

	if err := m.SyncHostRoutes(peers); err != nil {
		t.Fatalf("SyncHostRoutes returned %v, want no error for a table that holds no route", err)
	}

	var wrote bool
	for _, c := range rec.Calls() {
		if len(c.Args) > 1 && c.Args[0] == "route" && c.Args[1] == "replace" {
			wrote = true
		}
	}
	if !wrote {
		t.Error("the manager wrote no route, and the empty table needs every route of the tailnet")
	}
}

func TestTableIsEmptyReadsTheAnswerOfTheHost(t *testing.T) {
	if !tableIsEmpty("Error: ipv4: FIB table does not exist.\nDump terminated\n") {
		t.Error("tableIsEmpty does not read the answer of the host for a table with no route")
	}
	if tableIsEmpty("") {
		t.Error("tableIsEmpty reads an empty answer as a missing table")
	}
	if tableIsEmpty("Error: argument \"5x\" is wrong: table id value is invalid\n") {
		t.Error("tableIsEmpty reads another failure as a missing table")
	}
}
