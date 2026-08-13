package hostaccess

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/execx"
)

// syncFixture returns a Recorder and a Manager that sends every command to it. The
// fixture scripts each command that SyncHostRoutes runs for one peer that holds one IPv4
// address and one IPv6 address.
func syncFixture(t *testing.T) (*execx.Recorder, *Manager, TailnetPeers, []execx.Call) {
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

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, "ip", "netns", "exec", "ns-corp", "ip", "route", "show", "table", "52")
	rec.Script(execx.Result{}, "ip", "netns", "exec", "ns-corp", "ip", "-6", "route", "show", "table", "52")
	rec.Script(execx.Result{Output: []byte("100.64.0.1 via 10.200.0.1 dev vh001 src 10.200.0.2\n")},
		"ip", "route", "get", "100.64.0.1")
	rec.Script(execx.Result{Output: []byte("fd7a:115c:a1e0::1 via fe80::1 dev vh001 src fd7a::2\n")},
		"ip", "-6", "route", "get", "fd7a:115c:a1e0::1")
	rec.Script(execx.Result{Output: []byte("10.9.9.0/24 dev vh001 scope link\n")}, "ip", "route", "show")
	rec.Script(execx.Result{}, "ip", "-6", "route", "show")
	rec.Script(execx.Result{}, "ip", "route", "replace", "100.64.0.1", "via", "10.200.0.1", "dev", "vh001")
	rec.Script(execx.Result{}, "ip", "route", "del", "10.9.9.0/24")
	rec.Script(execx.Result{}, "ip", "-6", "route", "replace", "fd7a:115c:a1e0::1", "dev", "vh001")

	want := []execx.Call{
		{Name: "ip", Args: []string{"netns", "exec", "ns-corp", "ip", "route", "show", "table", "52"}},
		{Name: "ip", Args: []string{"netns", "exec", "ns-corp", "ip", "-6", "route", "show", "table", "52"}},
		{Name: "ip", Args: []string{"route", "get", "100.64.0.1"}},
		{Name: "ip", Args: []string{"-6", "route", "get", "fd7a:115c:a1e0::1"}},
		{Name: "ip", Args: []string{"route", "show"}},
		{Name: "ip", Args: []string{"-6", "route", "show"}},
		{Name: "ip", Args: []string{"route", "replace", "100.64.0.1", "via", "10.200.0.1", "dev", "vh001"}},
		{Name: "ip", Args: []string{"route", "del", "10.9.9.0/24"}},
		{Name: "ip", Args: []string{"-6", "route", "replace", "fd7a:115c:a1e0::1", "dev", "vh001"}},
	}

	m := NewManager("hosts", filepath.Join(t.TempDir(), "hosts"), "10.200.0.0/16")
	m.Runner = rec
	return rec, m, peers, want
}

func TestSyncHostRoutesRunsTheFullCommandListInOrder(t *testing.T) {
	rec, m, peers, want := syncFixture(t)

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
	rec, m, peers, want := syncFixture(t)

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

	m := NewManager("hosts", filepath.Join(t.TempDir(), "hosts"), "10.200.0.0/16")
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
