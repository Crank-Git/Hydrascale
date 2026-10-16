package hostaccess

import (
	"encoding/json"
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

	result := ParsePeers("mynet", status, "10.0.0.1", "10.0.0.2", "10.0.0.3", "ns-mynet")

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
	result := ParsePeers("mynet", status, "", "", "", "ns-mynet")
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
	result := ParsePeers("net", status, "", "", "", "ns-net")
	if len(result.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(result.Peers))
	}
	if result.Peers[0].Online {
		t.Error("peer should be Online=false")
	}
}

func TestParsePeers_NilStatus(t *testing.T) {
	result := ParsePeers("mynet", nil, "gw", "host", "hostip", "ns-mynet")
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
	result := ParsePeers("net", status, "", "", "", "ns-net")
	if result.MagicDNSSuffix != "" {
		t.Errorf("expected empty MagicDNSSuffix, got %q", result.MagicDNSSuffix)
	}
}

func TestParsePeers_PeerWithNoIPs_Excluded(t *testing.T) {
	status := makeStatus("", map[string]daemon.StatusNode{
		"noip":  {HostName: "NoIP", TailscaleIPs: []string{}},
		"hasip": {HostName: "HasIP", TailscaleIPs: []string{"100.64.0.9"}},
	})
	result := ParsePeers("net", status, "", "", "", "ns-net")
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
	result := ParsePeers("net", status, "", "", "", "ns-net")
	if len(result.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(result.Peers))
	}
	if result.Peers[0].IPv4 != "100.64.0.3" {
		t.Errorf("IPv4 = %q, want %q", result.Peers[0].IPv4, "100.64.0.3")
	}
}

func TestParsePeers_CopiesSplitDNSRoutes(t *testing.T) {
	status := makeStatus("example.ts.net", nil)
	status.SplitDNSRoutes = []string{"acme.example.com", "zeta.example.com"}

	result := ParsePeers("mynet", status, "10.0.0.1", "vh001", "10.0.0.2", "ns-mynet")

	if len(result.SplitDNSDomains) != 2 || result.SplitDNSDomains[0] != "acme.example.com" || result.SplitDNSDomains[1] != "zeta.example.com" {
		t.Fatalf("SplitDNSDomains = %v, want [acme.example.com zeta.example.com]", result.SplitDNSDomains)
	}
	// The field is a copy, therefore a later edit of the status keeps the peers unchanged.
	status.SplitDNSRoutes[0] = "changed.example.com"
	if result.SplitDNSDomains[0] != "acme.example.com" {
		t.Errorf("SplitDNSDomains[0] = %q, want %q", result.SplitDNSDomains[0], "acme.example.com")
	}

	empty := ParsePeers("mynet", nil, "10.0.0.1", "vh001", "10.0.0.2", "ns-mynet")
	if len(empty.SplitDNSDomains) != 0 {
		t.Errorf("SplitDNSDomains on a nil status = %v, want empty", empty.SplitDNSDomains)
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

func TestParsePeersPicksUpTheServicesOfTheTailnet(t *testing.T) {
	status := makeStatus("tail1234.ts.net", map[string]daemon.StatusNode{
		"abc123": {HostName: "Alpha", TailscaleIPs: []string{"100.64.0.1"}, Online: true},
	})
	status.Self = daemon.StatusNode{
		HostName: "host",
		CapMap: map[string][]json.RawMessage{
			"services/litellm": {json.RawMessage(`{"Name":"svc:litellm","Addrs":["100.64.0.138","fd7a:115c:a1e0::4242"],"Ports":["tcp:443"]}`)},
		},
	}

	result := ParsePeers("corp", status, "10.200.0.1", "vh001", "10.200.0.2", "ns-corp")

	if len(result.Services) != 1 {
		t.Fatalf("len(Services) = %d, want 1: %+v", len(result.Services), result.Services)
	}
	service := result.Services[0]
	if service.Name != "litellm" {
		t.Errorf("Name = %q, want %q", service.Name, "litellm")
	}
	if service.Raw != "svc:litellm" {
		t.Errorf("Raw = %q, want %q", service.Raw, "svc:litellm")
	}
	if len(service.IPv4) != 1 || service.IPv4[0] != "100.64.0.138" {
		t.Errorf("IPv4 = %v, want [100.64.0.138]", service.IPv4)
	}
	if len(service.IPv6) != 1 || service.IPv6[0] != "fd7a:115c:a1e0::4242" {
		t.Errorf("IPv6 = %v, want [fd7a:115c:a1e0::4242]", service.IPv6)
	}
	if len(service.Ports) != 1 || service.Ports[0] != "tcp:443" {
		t.Errorf("Ports = %v, want [tcp:443]", service.Ports)
	}
}

func TestServiceLabelMapsEveryCharacterThatIsNotADNSLabel(t *testing.T) {
	// The name of a service reaches the hosts file and the resolver, so every character that
	// is not a letter, a digit or a hyphen becomes a hyphen, and a name that leaves nothing
	// behind holds no record.
	cases := []struct {
		in   string
		want string
	}{
		{"svc:litellm", "litellm"},
		{"svc:Lite Llm", "lite-llm"},
		{"svc:litellm--staging", "litellm-staging"},
		{"svc:lit:ellm\n100.64.0.9  evil", "lit-ellm-100-64-0-9-evil"},
		{"svc:_evil", "evil"},
		{"svc:1", "1"},
		{"svc:", ""},
		{"svc:---", ""},
		{"svc:.", ""},
	}
	for _, tc := range cases {
		got := serviceLabel(tc.in)
		if got != tc.want {
			t.Errorf("serviceLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseServicesDropsANameWithoutALabelAndAnEntryThatIsNotAnAddress(t *testing.T) {
	got := parseServices([]daemon.ServiceEndpoint{
		{Name: "svc::", Addrs: []string{"100.64.0.138"}},
		{Name: "svc:ok", Addrs: []string{"not-an-address", "100.64.0.139"}},
		{Name: "svc:empty", Addrs: nil},
		{Name: "svc:zeta", Addrs: []string{"100.64.0.140"}},
	})

	if len(got) != 2 {
		t.Fatalf("parseServices returned %d services, want 2: %+v", len(got), got)
	}
	// parseServices sorts by name, so the order is fixed.
	if got[0].Name != "ok" || got[1].Name != "zeta" {
		t.Errorf("names = %q, %q, want ok then zeta", got[0].Name, got[1].Name)
	}
	if len(got[0].IPv4) != 1 || got[0].IPv4[0] != "100.64.0.139" {
		t.Errorf("IPv4 of ok = %v, want [100.64.0.139]", got[0].IPv4)
	}
}

func TestBuildServiceRecordsNamesAServiceAfterTheTailnet(t *testing.T) {
	services := []Service{{
		Name: "litellm",
		IPv4: []string{"100.64.0.138"},
		IPv6: []string{"fd7a:115c:a1e0::4242"},
	}}

	v4, v6 := BuildServiceRecords("corp", services)
	if v4["corp-litellm"] != "100.64.0.138" {
		t.Errorf("v4 record = %q, want 100.64.0.138", v4["corp-litellm"])
	}
	if v6["corp-litellm"] != "fd7a:115c:a1e0::4242" {
		t.Errorf("v6 record = %q, want fd7a:115c:a1e0::4242", v6["corp-litellm"])
	}
}

func TestServiceHostNameNamesAServiceAsTheHostDoes(t *testing.T) {
	// The control API and the console name a service with the word that the hosts file holds,
	// so an operator reads one name in the console, in the hosts file, and in a query.
	cases := []struct {
		tailnetID string
		raw       string
		want      string
	}{
		{"corp", "svc:litellm", "corp-litellm"},
		{"corp", "svc:Lite Llm", "corp-lite-llm"},
		{"havoc", "svc:redis--staging", "havoc-redis-staging"},
		{"corp", "svc:100.64.0.9 evil", "corp-100-64-0-9-evil"},
		{"corp", "svc:::", ""},
	}
	for _, tc := range cases {
		if got := ServiceHostName(tc.tailnetID, tc.raw); got != tc.want {
			t.Errorf("ServiceHostName(%q, %q) = %q, want %q", tc.tailnetID, tc.raw, got, tc.want)
		}
	}

	// The name is the key that BuildServiceRecords writes into the hosts file, so the two
	// cannot drift apart.
	v4, _ := BuildServiceRecords("corp", parseServices([]daemon.ServiceEndpoint{
		{Name: "svc:litellm", Addrs: []string{"100.64.0.138"}},
	}))
	if v4[ServiceHostName("corp", "svc:litellm")] != "100.64.0.138" {
		t.Errorf("the hosts record of the service holds no name that ServiceHostName states: %v", v4)
	}
}

func TestAddServiceAliasRecordsKeepsANameThatAPeerHolds(t *testing.T) {
	zone4 := map[string]string{"litellm": "100.64.0.9"}
	zone6 := map[string]string{}

	addServiceAliasRecords(zone4, zone6, []Service{
		{Name: "litellm", IPv4: []string{"100.64.0.138"}, IPv6: []string{"fd7a:115c:a1e0::4242"}},
		{Name: "redis", IPv4: []string{"100.64.0.139"}},
	})

	if zone4["litellm"] != "100.64.0.9" {
		t.Errorf("the service took the name of a peer: zone4[litellm] = %q", zone4["litellm"])
	}
	if zone6["litellm"] != "fd7a:115c:a1e0::4242" {
		t.Errorf("zone6[litellm] = %q, want fd7a:115c:a1e0::4242", zone6["litellm"])
	}
	if zone4["redis"] != "100.64.0.139" {
		t.Errorf("zone4[redis] = %q, want 100.64.0.139", zone4["redis"])
	}
}
