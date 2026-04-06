package hostaccess

import (
	"sort"
	"testing"

	"hydrascale/internal/daemon"
)

func makeStatus(suffix string, peers map[string]daemon.StatusNode) *daemon.TailscaleStatus {
	return &daemon.TailscaleStatus{
		MagicDNSSuffix: suffix,
		Peer:           peers,
	}
}

func TestParsePeers_MultiplePeers(t *testing.T) {
	status := makeStatus("example.ts.net", map[string]daemon.StatusNode{
		"abc123": {
			HostName:     "Alpha",
			TailscaleIPs: []string{"100.64.0.1", "fd7a::1"},
			Online:       true,
		},
		"def456": {
			HostName:     "Beta",
			TailscaleIPs: []string{"100.64.0.2", "fd7a::2"},
			Online:       false,
		},
	})

	result := ParsePeers("mynet", status, "10.0.0.1", "10.0.0.2")

	if result.TailnetID != "mynet" {
		t.Errorf("TailnetID = %q, want %q", result.TailnetID, "mynet")
	}
	if result.MagicDNSSuffix != "example.ts.net" {
		t.Errorf("MagicDNSSuffix = %q, want %q", result.MagicDNSSuffix, "example.ts.net")
	}
	if result.VethGateway != "10.0.0.1" {
		t.Errorf("VethGateway = %q, want %q", result.VethGateway, "10.0.0.1")
	}
	if result.VethHost != "10.0.0.2" {
		t.Errorf("VethHost = %q, want %q", result.VethHost, "10.0.0.2")
	}
	if len(result.Peers) != 2 {
		t.Fatalf("len(Peers) = %d, want 2", len(result.Peers))
	}

	// Sort for deterministic comparison.
	sort.Slice(result.Peers, func(i, j int) bool {
		return result.Peers[i].Hostname < result.Peers[j].Hostname
	})

	alpha := result.Peers[0]
	if alpha.Hostname != "alpha" {
		t.Errorf("Hostname = %q, want %q", alpha.Hostname, "alpha")
	}
	if alpha.IPv4 != "100.64.0.1" {
		t.Errorf("IPv4 = %q, want %q", alpha.IPv4, "100.64.0.1")
	}
	if alpha.IPv6 != "fd7a::1" {
		t.Errorf("IPv6 = %q, want %q", alpha.IPv6, "fd7a::1")
	}
	if !alpha.Online {
		t.Error("alpha should be Online=true")
	}

	beta := result.Peers[1]
	if beta.Hostname != "beta" {
		t.Errorf("Hostname = %q, want %q", beta.Hostname, "beta")
	}
	if beta.Online {
		t.Error("beta should be Online=false")
	}
}

func TestParsePeers_EmptyPeerList(t *testing.T) {
	status := makeStatus("example.ts.net", map[string]daemon.StatusNode{})
	result := ParsePeers("mynet", status, "", "")
	if len(result.Peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(result.Peers))
	}
	if result.MagicDNSSuffix != "example.ts.net" {
		t.Errorf("MagicDNSSuffix = %q, want %q", result.MagicDNSSuffix, "example.ts.net")
	}
}

func TestParsePeers_OfflinePeersIncluded(t *testing.T) {
	status := makeStatus("", map[string]daemon.StatusNode{
		"x": {
			HostName:     "Offline",
			TailscaleIPs: []string{"100.64.1.1"},
			Online:       false,
		},
	})
	result := ParsePeers("net", status, "", "")
	if len(result.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(result.Peers))
	}
	if result.Peers[0].Online {
		t.Error("peer should be Online=false")
	}
}

func TestParsePeers_NilStatus(t *testing.T) {
	result := ParsePeers("mynet", nil, "gw", "host")
	if result.TailnetID != "mynet" {
		t.Errorf("TailnetID = %q, want %q", result.TailnetID, "mynet")
	}
	if result.VethGateway != "gw" {
		t.Errorf("VethGateway = %q, want %q", result.VethGateway, "gw")
	}
	if len(result.Peers) != 0 {
		t.Errorf("expected 0 peers on nil status, got %d", len(result.Peers))
	}
	if result.MagicDNSSuffix != "" {
		t.Errorf("expected empty MagicDNSSuffix, got %q", result.MagicDNSSuffix)
	}
}

func TestParsePeers_MissingMagicDNSSuffix(t *testing.T) {
	status := makeStatus("", map[string]daemon.StatusNode{
		"a": {HostName: "Node", TailscaleIPs: []string{"100.64.0.5"}},
	})
	result := ParsePeers("net", status, "", "")
	if result.MagicDNSSuffix != "" {
		t.Errorf("expected empty MagicDNSSuffix, got %q", result.MagicDNSSuffix)
	}
}

func TestParsePeers_PeerWithNoIPs_Excluded(t *testing.T) {
	status := makeStatus("", map[string]daemon.StatusNode{
		"noip": {HostName: "NoIP", TailscaleIPs: []string{}},
		"hasip": {HostName: "HasIP", TailscaleIPs: []string{"100.64.0.9"}},
	})
	result := ParsePeers("net", status, "", "")
	if len(result.Peers) != 1 {
		t.Fatalf("expected 1 peer (no-IP excluded), got %d", len(result.Peers))
	}
	if result.Peers[0].Hostname != "hasip" {
		t.Errorf("unexpected peer %q", result.Peers[0].Hostname)
	}
}

func TestParsePeers_InvalidIPSkipped(t *testing.T) {
	status := makeStatus("", map[string]daemon.StatusNode{
		"bad": {HostName: "Bad", TailscaleIPs: []string{"not-an-ip", "100.64.0.3"}},
	})
	result := ParsePeers("net", status, "", "")
	if len(result.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(result.Peers))
	}
	if result.Peers[0].IPv4 != "100.64.0.3" {
		t.Errorf("IPv4 = %q, want %q", result.Peers[0].IPv4, "100.64.0.3")
	}
}

func TestBuildDNSNames(t *testing.T) {
	peers := []Peer{
		{Hostname: "alpha", IPv4: "100.64.0.1"},
		{Hostname: "beta", IPv4: "100.64.0.2", IPv6: "fd7a::2"},
		{Hostname: "noip"},
	}
	records := BuildDNSNames("mynet", peers)

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records["mynet-alpha"] != "100.64.0.1" {
		t.Errorf("mynet-alpha = %q, want %q", records["mynet-alpha"], "100.64.0.1")
	}
	if records["mynet-beta"] != "100.64.0.2" {
		t.Errorf("mynet-beta = %q, want %q", records["mynet-beta"], "100.64.0.2")
	}
	if _, ok := records["mynet-noip"]; ok {
		t.Error("noip should not appear in DNS names (no IPv4)")
	}
}

func TestBuildDNSRecords(t *testing.T) {
	peers := []Peer{
		{Hostname: "alpha", IPv4: "100.64.0.1", IPv6: "fd7a::1"},
		{Hostname: "beta", IPv4: "100.64.0.2"},
		{Hostname: "gamma", IPv6: "fd7a::3"},
	}
	v4, v6 := BuildDNSRecords("net", peers)

	// v4 checks
	if v4["net-alpha"] != "100.64.0.1" {
		t.Errorf("v4[net-alpha] = %q, want %q", v4["net-alpha"], "100.64.0.1")
	}
	if v4["net-beta"] != "100.64.0.2" {
		t.Errorf("v4[net-beta] = %q, want %q", v4["net-beta"], "100.64.0.2")
	}
	if _, ok := v4["net-gamma"]; ok {
		t.Error("gamma has no IPv4, should not appear in v4 map")
	}

	// v6 checks
	if v6["net-alpha"] != "fd7a::1" {
		t.Errorf("v6[net-alpha] = %q, want %q", v6["net-alpha"], "fd7a::1")
	}
	if _, ok := v6["net-beta"]; ok {
		t.Error("beta has no IPv6, should not appear in v6 map")
	}
	if v6["net-gamma"] != "fd7a::3" {
		t.Errorf("v6[net-gamma] = %q, want %q", v6["net-gamma"], "fd7a::3")
	}
}
