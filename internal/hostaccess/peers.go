package hostaccess

import (
	"net"
	"strings"

	"hydrascale/internal/daemon"
)

type Peer struct {
	Hostname string
	// AliasName holds every label of the MagicDNS name of the peer below the MagicDNS
	// suffix of the tailnet. For the MagicDNS name `a.b.tail1234.ts.net` of a tailnet
	// whose suffix is `tail1234.ts.net`, AliasName holds `a.b`. The alias zone answers
	// `a.b.<alias>.ts.internal` from it, therefore a nested MagicDNS name keeps its shape.
	// AliasName is empty when the status holds no MagicDNS name for the peer.
	AliasName string
	IPv4      string
	IPv6      string
	Online    bool
}

// TailnetPeers holds the peers of one tailnet and the veth pair that reaches it.
// VethGateway is the namespace side address, VethHost is the host side device name, and
// VethHostIP is the host side address. The DNS forwarder answers on VethHostIP when
// resolver.resolve_aliases is set.
type TailnetPeers struct {
	TailnetID      string
	MagicDNSSuffix string
	Peers          []Peer
	VethGateway    string
	VethHost       string
	VethHostIP     string
	NsName         string
}

func ParsePeers(tailnetID string, status *daemon.TailscaleStatus, vethGW, vethHost, vethHostIP, nsName string) TailnetPeers {
	result := TailnetPeers{
		TailnetID:   tailnetID,
		VethGateway: vethGW,
		VethHost:    vethHost,
		VethHostIP:  vethHostIP,
		NsName:      nsName,
	}
	if status == nil {
		return result
	}
	result.MagicDNSSuffix = status.MagicDNSSuffix

	for _, node := range status.Peer {
		// Sanitize hostname: lowercase, replace spaces with dashes
		hostname := strings.ToLower(node.HostName)
		hostname = strings.ReplaceAll(hostname, " ", "-")
		peer := Peer{
			Hostname:  hostname,
			AliasName: aliasNameOf(node.DNSName, status.MagicDNSSuffix),
			Online:    node.Online,
		}
		for _, ip := range node.TailscaleIPs {
			parsed := net.ParseIP(ip)
			if parsed == nil {
				continue
			}
			if parsed.To4() != nil {
				peer.IPv4 = ip
			} else {
				peer.IPv6 = ip
			}
		}
		if peer.IPv4 != "" || peer.IPv6 != "" {
			result.Peers = append(result.Peers, peer)
		}
	}
	return result
}

// aliasNameOf returns every label of the MagicDNS name dnsName below the MagicDNS suffix
// of the tailnet. It returns an empty string when dnsName is empty, when it does not end
// in the suffix, or when it holds no label below the suffix.
// The name that `tailscale status --json` reports carries a trailing dot, and the suffix
// carries none, therefore aliasNameOf removes a trailing dot from each one first.
func aliasNameOf(dnsName, suffix string) string {
	name := strings.ToLower(strings.TrimSuffix(dnsName, "."))
	suffix = strings.ToLower(strings.TrimSuffix(suffix, "."))
	if name == "" || suffix == "" {
		return ""
	}
	if !strings.HasSuffix(name, "."+suffix) {
		return ""
	}
	return name[:len(name)-len(suffix)-1]
}

// BuildAliasRecords returns the address records of an alias zone, keyed by the labels of
// the MagicDNS name of a peer below the MagicDNS suffix. The daemon answers
// <name>.<alias>.ts.internal from these records, therefore a MagicDNS name that holds
// several labels keeps every one of them.
// A peer that holds no MagicDNS name is absent from both maps, and a peer that holds no
// address of a family is absent from the map of that family.
// Two peers never hold the same MagicDNS name, because the control server assigns each
// one. BuildAliasRecords therefore writes no key twice.
func BuildAliasRecords(peers []Peer) (v4, v6 map[string]string) {
	v4 = make(map[string]string, len(peers))
	v6 = make(map[string]string, len(peers))
	for _, p := range peers {
		if p.AliasName == "" {
			continue
		}
		if p.IPv4 != "" {
			v4[p.AliasName] = p.IPv4
		}
		if p.IPv6 != "" {
			v6[p.AliasName] = p.IPv6
		}
	}
	return v4, v6
}

func BuildDNSNames(tailnetID string, peers []Peer) map[string]string {
	records := make(map[string]string, len(peers))
	for _, p := range peers {
		if p.IPv4 != "" {
			records[tailnetID+"-"+p.Hostname] = p.IPv4
		}
	}
	return records
}

func BuildDNSRecords(tailnetID string, peers []Peer) (v4 map[string]string, v6 map[string]string) {
	v4 = make(map[string]string, len(peers))
	v6 = make(map[string]string, len(peers))
	for _, p := range peers {
		name := tailnetID + "-" + p.Hostname
		if p.IPv4 != "" {
			v4[name] = p.IPv4
		}
		if p.IPv6 != "" {
			v6[name] = p.IPv6
		}
	}
	return v4, v6
}
