package namespaces

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"hydrascale/internal/execx"
)

// failed is the result of a command that exits non-zero.
var failed = execx.Result{Err: errors.New("exit status 1")}

func TestVethIPv6PutsEachNamespaceInItsOwnPrefix(t *testing.T) {
	hostIP, nsIP, hostGW, prefix := VethIPv6(171)
	if hostIP != "fd5c:9a3e:7b10:ab::1/64" || nsIP != "fd5c:9a3e:7b10:ab::2/64" {
		t.Errorf("VethIPv6(171) = %s, %s", hostIP, nsIP)
	}
	if hostGW != "fd5c:9a3e:7b10:ab::1" || prefix != "fd5c:9a3e:7b10:ab::/64" {
		t.Errorf("VethIPv6(171) gateway %s, prefix %s", hostGW, prefix)
	}
}

func TestParseRouteDevicesReadsASingleRouteAndAMultipathRoute(t *testing.T) {
	output := "default via fe80::1 dev enp1s0f0 proto ra metric 100 expires 1795sec pref medium\n" +
		"default proto ra metric 1024 pref medium\n" +
		"\tnexthop via fe80::1 dev enp1s0f0 weight 1\n" +
		"\tnexthop via fe80::2 dev eth0.2 weight 1\n"
	got := parseRouteDevices(output)
	if want := []string{"enp1s0f0", "eth0.2"}; !slices.Equal(got, want) {
		t.Errorf("parseRouteDevices = %v, want %v", got, want)
	}
}

func TestParseHostPrefixesSkipsTheNamespaceDevices(t *testing.T) {
	output := "2: enp1s0f0    inet6 2001:db8:1:2:aaaa:bbbb:cccc:dddd/64 scope global dynamic noprefixroute \\       valid_lft 86000sec preferred_lft 14000sec\n" +
		"2: enp1s0f0    inet6 2001:db8:1:2::77/64 scope global dynamic \\       valid_lft 86000sec preferred_lft 14000sec\n" +
		"4: tailscale0    inet6 fd7a:115c:a1e0::b936:fe73/128 scope global \\       valid_lft forever preferred_lft forever\n" +
		"9: vh0123456789ab    inet6 fd5c:9a3e:7b10:ab::1/64 scope global \\       valid_lft forever preferred_lft forever\n"
	got := parseHostPrefixes(output)
	want := []string{"2001:db8:1:2::/64", "fd7a:115c:a1e0::b936:fe73/128"}
	if !slices.Equal(got, want) {
		t.Errorf("parseHostPrefixes = %v, want %v", got, want)
	}
}

func TestReadIPv6HostReadsAKernelWithoutForceForwarding(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("default via fe80::1 dev enp1s0f0 proto ra\n")}, "ip", "-6", "route", "show", "default")
	rec.Script(execx.Result{}, "ip", "-6", "-o", "addr", "show", "scope", "global")
	rec.Script(execx.Result{Output: []byte("sysctl: cannot stat /proc/sys/net/ipv6/conf/all/force_forwarding: No such file or directory\n"), Err: errors.New("exit status 255")},
		"sysctl", "-n", "net.ipv6.conf.all.force_forwarding")
	rec.Script(execx.Result{Output: []byte("1\n")}, "sysctl", "-n", "net.ipv6.conf.all.forwarding")

	h, err := (&RealManager{Runner: rec}).ReadIPv6Host()
	if err != nil {
		t.Fatalf("ReadIPv6Host: %v", err)
	}
	if h.ForceForwarding || !h.Forwarding || !slices.Equal(h.Uplinks, []string{"enp1s0f0"}) {
		t.Errorf("ReadIPv6Host = %+v", h)
	}
}

func TestEnableForceForwardingWritesOnlyADeviceThatDoesNotForward(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("1\n")}, "sysctl", "-n", "net.ipv6.conf.enp1s0f0.force_forwarding")
	rec.Script(execx.Result{Output: []byte("0\n")}, "sysctl", "-n", "net.ipv6.conf.eth0/2.force_forwarding")
	rec.Script(execx.Result{}, "sysctl", "-w", "net.ipv6.conf.eth0/2.force_forwarding=1")

	changed, err := (&RealManager{Runner: rec}).EnableForceForwarding([]string{"enp1s0f0", "eth0.2"})
	if err != nil {
		t.Fatalf("EnableForceForwarding: %v", err)
	}
	if !slices.Equal(changed, []string{"eth0.2"}) {
		t.Errorf("changed = %v, want [eth0.2]", changed)
	}
}

func TestEnableAllForwardingChangesAcceptRaBeforeItStartsForwarding(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("1: lo: <LOOPBACK,UP> mtu 65536\n2: enp1s0f0: <BROADCAST,UP> mtu 1500\n3: wlan0: <BROADCAST> mtu 1500\n9: vh0123456789ab@if8: <BROADCAST,UP> mtu 1500\n")},
		"ip", "-o", "link", "show")
	rec.Script(execx.Result{Output: []byte("1\n")}, "sysctl", "-n", "net.ipv6.conf.lo.accept_ra")
	rec.Script(execx.Result{}, "sysctl", "-w", "net.ipv6.conf.lo.accept_ra=2")
	rec.Script(execx.Result{Output: []byte("1\n")}, "sysctl", "-n", "net.ipv6.conf.enp1s0f0.accept_ra")
	rec.Script(execx.Result{}, "sysctl", "-w", "net.ipv6.conf.enp1s0f0.accept_ra=2")
	rec.Script(execx.Result{Output: []byte("0\n")}, "sysctl", "-n", "net.ipv6.conf.wlan0.accept_ra")
	rec.Script(failed, "sysctl", "-n", "net.ipv6.conf.vh0123456789ab.accept_ra")
	rec.Script(execx.Result{}, "sysctl", "-w", "net.ipv6.conf.all.forwarding=1")

	changed, err := (&RealManager{Runner: rec}).EnableAllForwarding()
	if err != nil {
		t.Fatalf("EnableAllForwarding: %v", err)
	}
	if !slices.Equal(changed, []string{"lo", "enp1s0f0"}) {
		t.Errorf("changed = %v, want [lo enp1s0f0]", changed)
	}
	calls := rec.Calls()
	if last := calls[len(calls)-1].String(); !strings.Contains(last, "all.forwarding=1") {
		t.Errorf("the last command = %q, want the write of all.forwarding", last)
	}
}

func TestEnsureIPv6PathWritesTheAddressesTheRouteAndTheNAT66Rule(t *testing.T) {
	const nsName = "ns-team-prod"
	hostVeth, nsVeth := VethNames(nsName)
	hostIP, nsIP, hostGW, prefix := VethIPv6(7)

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, "ip", "-6", "addr", "replace", hostIP, "dev", hostVeth, "nodad")
	rec.Script(execx.Result{}, "ip", "netns", "exec", nsName, "ip", "-6", "addr", "replace", nsIP, "dev", nsVeth, "nodad")
	rec.Script(execx.Result{}, "ip", "netns", "exec", nsName, "ip", "-6", "route", "replace", "default", "via", hostGW, "dev", nsVeth)
	rec.Script(execx.Result{}, "sysctl", "-w", "net.ipv6.conf."+hostVeth+".force_forwarding=1")
	rec.Script(failed, "ip6tables", "-t", "nat", "-C", "POSTROUTING", "-s", prefix, "-j", "MASQUERADE")
	rec.Script(execx.Result{}, "ip6tables", "-t", "nat", "-A", "POSTROUTING", "-s", prefix, "-j", "MASQUERADE")

	written, err := (&RealManager{Runner: rec}).EnsureIPv6Path(nsName, 7, true)
	if err != nil {
		t.Fatalf("EnsureIPv6Path: %v", err)
	}
	if len(written) != 1 {
		t.Errorf("written = %v, want the NAT66 rule", written)
	}
	if len(rec.Calls()) != 6 {
		t.Errorf("EnsureIPv6Path ran %d commands, want 6", len(rec.Calls()))
	}
}

func TestEnsureIPv6PathSetsNoForceForwardingWhenTheHostForwardsOnEveryDevice(t *testing.T) {
	const nsName = "ns-team-prod"
	hostVeth, nsVeth := VethNames(nsName)
	hostIP, nsIP, hostGW, prefix := VethIPv6(7)

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, "ip", "-6", "addr", "replace", hostIP, "dev", hostVeth, "nodad")
	rec.Script(execx.Result{}, "ip", "netns", "exec", nsName, "ip", "-6", "addr", "replace", nsIP, "dev", nsVeth, "nodad")
	rec.Script(execx.Result{}, "ip", "netns", "exec", nsName, "ip", "-6", "route", "replace", "default", "via", hostGW, "dev", nsVeth)
	rec.Script(execx.Result{}, "ip6tables", "-t", "nat", "-C", "POSTROUTING", "-s", prefix, "-j", "MASQUERADE")

	written, err := (&RealManager{Runner: rec}).EnsureIPv6Path(nsName, 7, false)
	if err != nil {
		t.Fatalf("EnsureIPv6Path: %v", err)
	}
	if len(written) != 0 {
		t.Errorf("written = %v, want none for a rule that the host holds", written)
	}
}

func TestTeardownDeletesTheNAT66RuleThatEnsureIPv6PathAdds(t *testing.T) {
	const nsName = "ns-team-prod"
	index := VethIndex(nsName)

	rec := execx.NewRecorder(t)
	hostVeth, nsVeth := VethNames(nsName)
	hostIP, nsIP, hostGW, prefix := VethIPv6(index)
	rec.Script(execx.Result{}, "ip", "-6", "addr", "replace", hostIP, "dev", hostVeth, "nodad")
	rec.Script(execx.Result{}, "ip", "netns", "exec", nsName, "ip", "-6", "addr", "replace", nsIP, "dev", nsVeth, "nodad")
	rec.Script(execx.Result{}, "ip", "netns", "exec", nsName, "ip", "-6", "route", "replace", "default", "via", hostGW, "dev", nsVeth)
	rec.Script(failed, "ip6tables", "-t", "nat", "-C", "POSTROUTING", "-s", prefix, "-j", "MASQUERADE")
	rec.Script(execx.Result{}, "ip6tables", "-t", "nat", "-A", "POSTROUTING", "-s", prefix, "-j", "MASQUERADE")
	if _, err := (&RealManager{Runner: rec}).EnsureIPv6Path(nsName, index, false); err != nil {
		t.Fatalf("EnsureIPv6Path: %v", err)
	}

	calls := rec.Calls()
	added := slices.Clone(calls[len(calls)-1].Args)
	added[slices.Index(added, "-A")] = "-D"
	if deleted := vethTeardownRulesIPv6(nsName)[0]; !slices.Equal(added, deleted) {
		t.Errorf("teardown deletes %v, want %v", deleted, added)
	}
}
