package hostaccess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/daemon"
	"hydrascale/internal/execx"
)

// statusWithPeer returns a status that holds one peer with one IPv4 address.
func statusWithPeer(suffix, hostname, ip string) *daemon.TailscaleStatus {
	return &daemon.TailscaleStatus{
		MagicDNSSuffix: suffix,
		Peer: map[string]daemon.StatusNode{
			"key": {HostName: hostname, TailscaleIPs: []string{ip}, Online: true},
		},
	}
}

func TestTeardownRemovesTheNamesOfTheTailnetFromTheHostsFile(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")

	m := NewManager("hosts", hostsPath, "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	m.Sync("corp", statusWithPeer("corp.ts.net", "laptop", "100.64.0.1"), "10.200.0.2", "vh001", "10.200.0.1", "ns-corp")
	m.Sync("home", statusWithPeer("home.ts.net", "server", "100.64.1.1"), "10.200.0.6", "vh002", "10.200.0.5", "ns-home")

	before, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("read the hosts file: %v", err)
	}
	if !strings.Contains(string(before), "corp-laptop") {
		t.Fatalf("the hosts file holds no name of corp:\n%s", before)
	}

	_ = m.Teardown("corp")

	after, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("read the hosts file: %v", err)
	}
	if strings.Contains(string(after), "corp-laptop") {
		t.Errorf("the hosts file still holds a name of the removed tailnet:\n%s", after)
	}
	if !strings.Contains(string(after), "home-server") {
		t.Errorf("the hosts file lost a name of the tailnet that stays:\n%s", after)
	}
}

func TestTeardownReturnsTheErrorOfAFailedHostsFileWrite(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")

	m := NewManager("hosts", hostsPath, "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	m.Sync("corp", statusWithPeer("corp.ts.net", "laptop", "100.64.0.1"), "10.200.0.2", "vh001", "10.200.0.1", "ns-corp")
	m.Sync("home", statusWithPeer("home.ts.net", "server", "100.64.1.1"), "10.200.0.6", "vh002", "10.200.0.5", "ns-home")

	// The write of the hosts file needs the directory, so remove it. One tailnet stays
	// after the teardown, so the manager writes a block rather than nothing.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove the directory: %v", err)
	}

	err := m.Teardown("corp")
	if err == nil {
		t.Fatal("Teardown returned no error for a failed hosts file write")
	}
	if !strings.Contains(err.Error(), "hosts file") && !strings.Contains(err.Error(), "temp file") {
		t.Errorf("Teardown error names no hosts file step: %v", err)
	}
}

func TestTeardownAllReturnsTheErrorOfAFailedHostsFileWrite(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")

	m := NewManager("hosts", hostsPath, "10.200.0.0/16", 0)
	m.Runner = quietRunner{}
	m.Sync("corp", statusWithPeer("corp.ts.net", "laptop", "100.64.0.1"), "10.200.0.2", "vh001", "10.200.0.1", "ns-corp")

	// TeardownAll removes the last tailnet, so the manager rewrites the file without the
	// block. Replace the file with a directory, because a rename over a directory fails.
	if err := os.Remove(hostsPath); err != nil {
		t.Fatalf("remove the hosts file: %v", err)
	}
	if err := os.Mkdir(hostsPath, 0755); err != nil {
		t.Fatalf("create the directory: %v", err)
	}

	if err := m.TeardownAll(); err == nil {
		t.Fatal("TeardownAll returned no error for a failed hosts file write")
	}
}

func TestTeardownAllRemovesTheRuleAndEmptiesTheRouteTable(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, "ip", "rule", "del", "priority", "32000", "from", "all", "lookup", "53")
	rec.Script(execx.Result{}, "ip", "-6", "rule", "del", "priority", "32000", "from", "all", "lookup", "53")
	rec.Script(execx.Result{}, "ip", "route", "flush", "table", "53")
	rec.Script(execx.Result{}, "ip", "-6", "route", "flush", "table", "53")
	m := &Manager{Runner: rec, routeTable: 53}

	if err := m.TeardownAll(); err != nil {
		t.Fatalf("TeardownAll: %v", err)
	}

	want := []string{
		"ip rule del priority 32000 from all lookup 53",
		"ip -6 rule del priority 32000 from all lookup 53",
		"ip route flush table 53",
		"ip -6 route flush table 53",
	}
	calls := rec.Calls()
	if len(calls) != len(want) {
		t.Fatalf("TeardownAll ran %d commands, want %d:\n%s", len(calls), len(want), callList(calls))
	}
	for i := range want {
		if calls[i].String() != want[i] {
			t.Errorf("command %d = %q, want %q", i, calls[i].String(), want[i])
		}
	}
}

func TestTeardownAllRunsNoRouteTableCommandWhenNoRouteTableIsDeclared(t *testing.T) {
	rec := execx.NewRecorder(t)
	m := &Manager{Runner: rec}

	if err := m.TeardownAll(); err != nil {
		t.Fatalf("TeardownAll: %v", err)
	}
	if len(rec.Calls()) != 0 {
		t.Errorf("TeardownAll ran %d commands for a manager that declares no route table:\n%s",
			len(rec.Calls()), callList(rec.Calls()))
	}
}
