package hostaccess

import (
	"sort"
	"testing"
)

func TestParseHostRoutes(t *testing.T) {
	const vethDev = "vh123456"
	const infraSubnet = "10.200.0.0/16"
	input := `100.64.0.1 dev vh123456 scope link
100.64.0.2 dev vh123456 scope link
100.100.100.100 via 10.200.1.2 dev vh123456
10.0.0.1 dev eth0 scope link
100.64.0.3 dev eth0 scope link
192.168.1.0/24 via 10.200.1.2 dev vh123456
`
	got := parseHostRoutes(input, vethDev, infraSubnet)
	want := []string{"100.64.0.1", "100.64.0.2", "192.168.1.0/24"}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("parseHostRoutes: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseHostRoutes[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseHostRoutes_ExcludesMagicDNS(t *testing.T) {
	const vethDev = "vh123456"
	const infraSubnet = "10.200.0.0/16"
	input := `100.100.100.100 via 10.200.1.2 dev vh123456
100.64.1.1 dev vh123456 scope link
`
	got := parseHostRoutes(input, vethDev, infraSubnet)
	for _, ip := range got {
		if ip == "100.100.100.100" {
			t.Errorf("parseHostRoutes should exclude 100.100.100.100, but found it in %v", got)
		}
	}
	if len(got) != 1 || got[0] != "100.64.1.1" {
		t.Errorf("parseHostRoutes: got %v, want [100.64.1.1]", got)
	}
}

func TestParseTableRoutes(t *testing.T) {
	const infraSubnet = "10.200.0.0/16"
	input := `10.0.0.0/8 via 100.64.0.1 dev tailscale0
172.16.0.0/12 via 100.64.0.1 dev tailscale0
192.168.1.0/24 via 100.64.0.1 dev tailscale0
default via 100.64.0.1 dev tailscale0
10.200.3.0/30 dev vnabc proto kernel scope link
100.64.0.5 dev tailscale0
100.100.100.100 dev tailscale0
fd7a:115c:a1e0::1 dev tailscale0
fd7a:115c:a1e0::/48 dev tailscale0
fd7a:115c:a1e0::53 dev tailscale0
`
	got := parseTableRoutes(input, infraSubnet)
	want := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.1.0/24"}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("parseTableRoutes: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseTableRoutes[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDesiredRoutes(t *testing.T) {
	peers := TailnetPeers{
		Peers: []Peer{
			{Hostname: "alpha", IPv4: "100.64.0.1", IPv6: "fd7a:115c:a1e0::1"},
			{Hostname: "beta", IPv4: "100.64.0.2"},
			{Hostname: "gamma", IPv6: "fd7a:115c:a1e0::3"},
			{Hostname: "delta"},
		},
	}
	v4, v6 := desiredPeerRoutes(peers)

	wantV4 := []string{"100.64.0.1", "100.64.0.2"}
	wantV6 := []string{"fd7a:115c:a1e0::1", "fd7a:115c:a1e0::3"}

	sort.Strings(v4)
	sort.Strings(v6)
	sort.Strings(wantV4)
	sort.Strings(wantV6)

	if len(v4) != len(wantV4) {
		t.Fatalf("v4: got %v, want %v", v4, wantV4)
	}
	for i := range wantV4 {
		if v4[i] != wantV4[i] {
			t.Errorf("v4[%d]: got %q, want %q", i, v4[i], wantV4[i])
		}
	}

	if len(v6) != len(wantV6) {
		t.Fatalf("v6: got %v, want %v", v6, wantV6)
	}
	for i := range wantV6 {
		if v6[i] != wantV6[i] {
			t.Errorf("v6[%d]: got %q, want %q", i, v6[i], wantV6[i])
		}
	}
}

func TestRouteSync_DiffLogic(t *testing.T) {
	desired := []string{"100.64.0.1", "100.64.0.2", "100.64.0.3"}
	actual := []string{"100.64.0.2", "100.64.0.4"}

	toAdd, toRemove := diffRoutes(desired, actual)

	sort.Strings(toAdd)
	sort.Strings(toRemove)

	wantAdd := []string{"100.64.0.1", "100.64.0.3"}
	wantRemove := []string{"100.64.0.4"}

	if len(toAdd) != len(wantAdd) {
		t.Fatalf("toAdd: got %v, want %v", toAdd, wantAdd)
	}
	for i := range wantAdd {
		if toAdd[i] != wantAdd[i] {
			t.Errorf("toAdd[%d]: got %q, want %q", i, toAdd[i], wantAdd[i])
		}
	}

	if len(toRemove) != len(wantRemove) {
		t.Fatalf("toRemove: got %v, want %v", toRemove, wantRemove)
	}
	for i := range wantRemove {
		if toRemove[i] != wantRemove[i] {
			t.Errorf("toRemove[%d]: got %q, want %q", i, toRemove[i], wantRemove[i])
		}
	}
}

func TestRouteSync_NoOp(t *testing.T) {
	routes := []string{"100.64.0.1", "100.64.0.2"}

	toAdd, toRemove := diffRoutes(routes, routes)

	if len(toAdd) != 0 {
		t.Errorf("NoOp: expected no routes to add, got %v", toAdd)
	}
	if len(toRemove) != 0 {
		t.Errorf("NoOp: expected no routes to remove, got %v", toRemove)
	}
}
