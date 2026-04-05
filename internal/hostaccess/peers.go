package hostaccess

import (
	"net"
	"strings"

	"hydrascale/internal/daemon"
)

type Peer struct {
	Hostname string
	IPv4     string
	IPv6     string
	Online   bool
}

type TailnetPeers struct {
	TailnetID      string
	MagicDNSSuffix string
	Peers          []Peer
	VethGateway    string
	VethHost       string
}

func ParsePeers(tailnetID string, status *daemon.TailscaleStatus, vethGW, vethHost string) TailnetPeers {
	result := TailnetPeers{
		TailnetID:   tailnetID,
		VethGateway: vethGW,
		VethHost:    vethHost,
	}
	if status == nil {
		return result
	}
	result.MagicDNSSuffix = status.MagicDNSSuffix
	for _, node := range status.Peer {
		peer := Peer{
			Hostname: strings.ToLower(node.HostName),
			Online:   node.Online,
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
