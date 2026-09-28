package hostaccess

import (
	"context"
	"sync"
	"testing"

	"hydrascale/internal/daemon"
	"hydrascale/internal/dns"
)

// aliasStatus returns a status that holds one peer whose MagicDNS name is host.suffix.
func aliasStatus(suffix, host, v4 string) *daemon.TailscaleStatus {
	return &daemon.TailscaleStatus{
		MagicDNSSuffix: suffix,
		Peer: map[string]daemon.StatusNode{
			"k": {
				HostName:     host,
				DNSName:      host + "." + suffix + ".",
				TailscaleIPs: []string{v4},
				Online:       true,
			},
		},
	}
}

// aliasRunner records every command and answers each one with success.
// The Manager runs route commands as well as resolvectl commands during a sync, and this
// test reads the resolvectl arguments alone, therefore the runner scripts nothing.
type aliasRunner struct {
	mu    sync.Mutex
	calls [][]string
}

func (r *aliasRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]string{name}, args...))
	return nil, nil
}

// resolvectl returns the arguments of the last resolvectl command whose first argument is
// verb. The second result is false when the runner recorded no such command.
func (r *aliasRunner) resolvectl(verb string) ([]string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var found []string
	for _, c := range r.calls {
		if len(c) > 1 && c[0] == "resolvectl" && c[1] == verb {
			found = c[1:]
		}
	}
	return found, found != nil
}

// aliasManager returns a Manager in the resolved mode and the runner that records it.
func aliasManager(t *testing.T) (*Manager, *aliasRunner) {
	t.Helper()
	run := &aliasRunner{}
	m := NewManager("resolved", t.TempDir()+"/hosts", "10.200.0.0/16", 0)
	m.Runner = run
	m.resolved = &ResolvedManager{Runner: run}
	return m, run
}

// wantArgs fails when got does not equal want.
func wantArgs(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %q, want %q", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %q, want %q", label, got, want)
		}
	}
}

// wantResolvectl reads the last resolvectl command of verb and compares it with want.
func wantResolvectl(t *testing.T, rec *aliasRunner, verb string, want []string) {
	t.Helper()
	got, held := rec.resolvectl(verb)
	if !held {
		t.Fatalf("the manager ran no resolvectl %s command", verb)
	}
	wantArgs(t, "resolvectl "+verb, got, want)
}

func TestSyncDNSWritesOneDomainWhenAliasResolutionIsOff(t *testing.T) {
	m, rec := aliasManager(t)
	m.SetAliasResolution(false, map[string]string{"Ta1a1a1a1a1CNTRL": "mmo"})
	m.Sync("Ta1a1a1a1a1CNTRL", aliasStatus("tail1.ts.net", "laptop", "100.64.0.1"),
		"10.200.0.2", "vh001", "10.200.0.1", "ns-t")

	wantResolvectl(t, rec, "domain", []string{"domain", "vh001", "~tail1.ts.net"})
}

func TestSyncDNSNamesTheNamespaceAddressWhenAliasResolutionIsOff(t *testing.T) {
	m, rec := aliasManager(t)
	m.SetAliasResolution(false, map[string]string{"Ta1a1a1a1a1CNTRL": "mmo"})
	m.Sync("Ta1a1a1a1a1CNTRL", aliasStatus("tail1.ts.net", "laptop", "100.64.0.1"),
		"10.200.0.2", "vh001", "10.200.0.1", "ns-t")

	wantResolvectl(t, rec, "dns", []string{"dns", "vh001", "10.200.0.2"})
}

func TestSyncDNSWritesBothDomainsWhenAliasResolutionIsOn(t *testing.T) {
	m, rec := aliasManager(t)
	m.SetForwarder(&mockForwarder{})
	m.SetAliasResolution(true, map[string]string{"Ta1a1a1a1a1CNTRL": "mmo"})
	m.Sync("Ta1a1a1a1a1CNTRL", aliasStatus("tail1.ts.net", "laptop", "100.64.0.1"),
		"10.200.0.2", "vh001", "10.200.0.1", "ns-t")

	wantResolvectl(t, rec, "domain",
		[]string{"domain", "vh001", "~tail1.ts.net", "~mmo.ts.internal"})

	// The link names the host side address, because a link carries one server for every
	// domain that it holds and the forwarder answers the alias zone.
	wantResolvectl(t, rec, "dns", []string{"dns", "vh001", "10.200.0.1"})
}

// The link lists the domains in the order: MagicDNS suffix, alias zone, split domains.
func TestSyncDNSOrdersTheDomainsSuffixAliasSplit(t *testing.T) {
	m, rec := aliasManager(t)
	m.SetForwarder(&mockForwarder{})
	m.SetAliasResolution(true, map[string]string{"Ta1a1a1a1a1CNTRL": "mmo"})

	status := aliasStatus("tail1.ts.net", "laptop", "100.64.0.1")
	status.SplitDNSRoutes = []string{"acme.example.com"}
	m.Sync("Ta1a1a1a1a1CNTRL", status, "10.200.0.2", "vh001", "10.200.0.1", "ns-t")

	wantResolvectl(t, rec, "domain",
		[]string{"domain", "vh001", "~tail1.ts.net", "~mmo.ts.internal", "~acme.example.com"})
	wantResolvectl(t, rec, "dns", []string{"dns", "vh001", "10.200.0.1"})
}

// A tailnet whose control server serves no MagicDNS suffix still answers its alias zone.
func TestSyncDNSRegistersTheAliasZoneOfATailnetWithNoMagicDNSSuffix(t *testing.T) {
	m, rec := aliasManager(t)
	m.SetForwarder(&mockForwarder{})
	m.SetAliasResolution(true, map[string]string{"Ta1a1a1a1a1CNTRL": "mmo"})

	status := aliasStatus("tail1.ts.net", "laptop", "100.64.0.1")
	status.MagicDNSSuffix = ""
	m.Sync("Ta1a1a1a1a1CNTRL", status, "10.200.0.2", "vh001", "10.200.0.1", "ns-t")

	wantResolvectl(t, rec, "domain", []string{"domain", "vh001", "~mmo.ts.internal"})
	wantResolvectl(t, rec, "dns", []string{"dns", "vh001", "10.200.0.1"})
}

// A listener that fails to bind must not take the MagicDNS suffix of the tailnet with it.
// The link therefore keeps the namespace side address, which answers without the
// forwarder.
func TestSyncDNSKeepsTheNamespaceAddressWhenTheListenerFailsToBind(t *testing.T) {
	m, rec := aliasManager(t)
	fwd := &mockForwarder{deadListeners: map[string]bool{"10.200.0.1": true}}
	m.SetForwarder(fwd)
	m.SetAliasResolution(true, map[string]string{"Ta1a1a1a1a1CNTRL": "mmo"})

	m.Sync("Ta1a1a1a1a1CNTRL", aliasStatus("tail1.ts.net", "laptop", "100.64.0.1"),
		"10.200.0.2", "vh001", "10.200.0.1", "ns-t")

	wantResolvectl(t, rec, "domain", []string{"domain", "vh001", "~tail1.ts.net"})
	wantResolvectl(t, rec, "dns", []string{"dns", "vh001", "10.200.0.2"})
}

func TestSyncDNSGivesTheForwarderTheAliasZoneAndTheListener(t *testing.T) {
	m, _ := aliasManager(t)
	fwd := &mockForwarder{}
	m.SetForwarder(fwd)
	m.SetAliasResolution(true, map[string]string{"Ta1a1a1a1a1CNTRL": "mmo"})
	m.Sync("Ta1a1a1a1a1CNTRL", aliasStatus("tail1.ts.net", "laptop", "100.64.0.1"),
		"10.200.0.2", "vh001", "10.200.0.1", "ns-t")

	zone, held := fwd.lastZones["mmo.ts.internal"]
	if !held {
		t.Fatalf("the forwarder holds the zones %v, want mmo.ts.internal", fwd.lastZones)
	}
	if zone.V4["laptop"] != "100.64.0.1" {
		t.Errorf("the zone holds %q for laptop, want 100.64.0.1", zone.V4["laptop"])
	}
	if len(fwd.lastListeners) != 1 || fwd.lastListeners[0] != "10.200.0.1" {
		t.Errorf("the listeners are %q, want the host address 10.200.0.1", fwd.lastListeners)
	}
}

func TestSyncDNSBuildsNoAliasZoneForATailnetWithNoAlias(t *testing.T) {
	m, _ := aliasManager(t)
	fwd := &mockForwarder{}
	m.SetForwarder(fwd)
	m.SetAliasResolution(true, map[string]string{})
	m.Sync("Ta1a1a1a1a1CNTRL", aliasStatus("tail1.ts.net", "laptop", "100.64.0.1"),
		"10.200.0.2", "vh001", "10.200.0.1", "ns-t")

	if len(fwd.lastZones) != 0 {
		t.Errorf("the forwarder holds the zones %v, want none", fwd.lastZones)
	}
	if len(fwd.lastListeners) != 0 {
		t.Errorf("the forwarder holds the listeners %q, want none", fwd.lastListeners)
	}
}

// The teardown of a tailnet takes its listener with it.
func TestTeardownStopsTheListenerOfTheTailnet(t *testing.T) {
	m, _ := aliasManager(t)
	fwd := &mockForwarder{}
	m.SetForwarder(fwd)
	m.SetAliasResolution(true, map[string]string{"Ta1a1a1a1a1CNTRL": "mmo"})
	m.Sync("Ta1a1a1a1a1CNTRL", aliasStatus("tail1.ts.net", "laptop", "100.64.0.1"),
		"10.200.0.2", "vh001", "10.200.0.1", "ns-t")
	if len(fwd.lastListeners) != 1 {
		t.Fatalf("the forwarder holds the listeners %q, want one", fwd.lastListeners)
	}

	m.Teardown("Ta1a1a1a1a1CNTRL")

	if len(fwd.lastListeners) != 0 {
		t.Errorf("the forwarder holds the listeners %q after the teardown, want none", fwd.lastListeners)
	}
	if len(fwd.lastZones) != 0 {
		t.Errorf("the forwarder holds the zones %v after the teardown, want none", fwd.lastZones)
	}
}

func TestBuildAliasRecordsKeysThePeersByTheLabelsBelowTheSuffix(t *testing.T) {
	v4, v6 := BuildAliasRecords([]Peer{
		{AliasName: "laptop", IPv4: "100.64.0.1", IPv6: "fd7a::1"},
		{AliasName: "phone", IPv6: "fd7a::2"},
		// A peer that the status reports with no MagicDNS name reaches no zone.
		{Hostname: "ghost", IPv4: "100.64.0.9"},
	})
	if v4["laptop"] != "100.64.0.1" {
		t.Errorf("v4[laptop] = %q, want 100.64.0.1", v4["laptop"])
	}
	if _, held := v4["phone"]; held {
		t.Error("v4 holds phone, which carries no IPv4 address")
	}
	if v6["phone"] != "fd7a::2" {
		t.Errorf("v6[phone] = %q, want fd7a::2", v6["phone"])
	}
	if len(v4) != 1 || len(v6) != 2 {
		t.Errorf("the records hold %d v4 and %d v6 names, want 1 and 2", len(v4), len(v6))
	}
}

// A MagicDNS name that holds several labels below the suffix keeps every one of them, so
// that `a.b.tail1.ts.net` answers as `a.b.mmo.ts.internal`.
func TestParsePeersKeepsEveryLabelBelowTheMagicDNSSuffix(t *testing.T) {
	for _, tc := range []struct {
		dnsName string
		suffix  string
		want    string
	}{
		{"laptop.tail1.ts.net.", "tail1.ts.net", "laptop"},
		{"a.b.tail1.ts.net.", "tail1.ts.net", "a.b"},
		{"LAPTOP.TAIL1.TS.NET.", "tail1.ts.net", "laptop"},
		{"laptop.tail1.ts.net", "tail1.ts.net.", "laptop"},
		// A name outside the suffix names no peer of this tailnet.
		{"laptop.other.ts.net.", "tail1.ts.net", ""},
		{"tail1.ts.net.", "tail1.ts.net", ""},
		{"", "tail1.ts.net", ""},
		{"laptop.tail1.ts.net.", "", ""},
	} {
		status := &daemon.TailscaleStatus{
			MagicDNSSuffix: tc.suffix,
			Peer: map[string]daemon.StatusNode{
				"k": {HostName: "laptop", DNSName: tc.dnsName, TailscaleIPs: []string{"100.64.0.1"}},
			},
		}
		got := ParsePeers("t", status, "10.0.0.2", "vh1", "10.0.0.1", "ns-t")
		if len(got.Peers) != 1 {
			t.Fatalf("ParsePeers(%q) returned %d peers, want 1", tc.dnsName, len(got.Peers))
		}
		if got.Peers[0].AliasName != tc.want {
			t.Errorf("AliasName of %q under %q = %q, want %q",
				tc.dnsName, tc.suffix, got.Peers[0].AliasName, tc.want)
		}
	}
}

// The forwarder and the manager must build the same zone name from one alias.
func TestTheZoneNameOfAnAliasMatchesTheForwarder(t *testing.T) {
	if got, want := dns.AliasZoneName("mmo"), "mmo.ts.internal"; got != want {
		t.Errorf("AliasZoneName = %q, want %q", got, want)
	}
	if got, want := dns.AliasZoneName("MMO"), "mmo.ts.internal"; got != want {
		t.Errorf("AliasZoneName folds no case: %q, want %q", got, want)
	}
}
