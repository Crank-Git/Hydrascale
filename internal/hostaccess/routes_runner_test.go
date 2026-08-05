package hostaccess

import (
	"context"
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
