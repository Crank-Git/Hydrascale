package hostaccess

import (
	"log"
	"net"
	"regexp"
	"sort"
	"strings"

	"hydrascale/internal/daemon"
)

// serviceLabelPattern is the shape that a name of a service must hold before the daemon
// writes it into the hosts file or into a resolver zone. It is one DNS label: a letter or a
// digit, then up to 61 characters of letters, digits and hyphen, then a letter or a digit.
var serviceLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

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

// Service holds one Tailscale service of a tailnet.
//
// A service is not a machine. The control server advertises it as a capability of the node
// that collects its endpoints, so it reaches the daemon through the capability map of that
// node and not through the peer list. Name holds the label that the host names the service
// with. Raw holds the name that the control server reported, which carries the prefix `svc:`
// and is not a DNS label. IPv4 and IPv6 hold the addresses that the service answers on, and
// Ports holds entries of the form `tcp:443`.
type Service struct {
	Name  string
	Raw   string
	IPv4  []string
	IPv6  []string
	Ports []string
}

// TailnetPeers holds the peers of one tailnet and the veth pair that reaches it.
// VethGateway is the namespace side address, VethHost is the host side device name, and
// VethHostIP is the host side address. The DNS forwarder answers on VethHostIP when
// resolver.resolve_aliases is set.
type TailnetPeers struct {
	TailnetID      string
	MagicDNSSuffix string
	// SplitDNSDomains holds the split DNS domains of the tailnet, as the control server
	// reported them. The daemon registers each one on the veth device of the tailnet, so
	// that the host resolver sends a query of that domain to the tailnet. See FR-split-2.
	SplitDNSDomains []string
	Peers           []Peer
	// Services holds the Tailscale services that the tailnet advertises. A service holds no
	// peer entry, so the daemon reads it off the capability maps of the nodes instead.
	Services    []Service
	VethGateway string
	VethHost    string
	VethHostIP  string
	NsName      string
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
	if len(status.SplitDNSRoutes) > 0 {
		result.SplitDNSDomains = append([]string(nil), status.SplitDNSRoutes...)
	}

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
	result.Services = parseServices(daemon.CollectServices(status))
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

// parseServices turns the service records that the control server reported into the services
// that the host can name and route.
//
// A record whose name holds no usable label is dropped and recorded in the log, because its
// name reaches the hosts file and the resolver. An entry of Addrs that is not an address is
// dropped, because an address reaches one argument of `ip route replace`. A record that
// holds no address of either family is dropped, because no route and no name reaches it.
// parseServices sorts by name, because the capability maps of the status have no order.
func parseServices(endpoints []daemon.ServiceEndpoint) []Service {
	if len(endpoints) == 0 {
		return nil
	}
	var out []Service
	for _, endpoint := range endpoints {
		label := serviceLabel(endpoint.Name)
		if label == "" {
			log.Printf("hostaccess: the daemon writes no record for the service %q, because its name holds no DNS label", endpoint.Name)
			continue
		}
		service := Service{Name: label, Raw: endpoint.Name, Ports: endpoint.Ports}
		for _, addr := range endpoint.Addrs {
			parsed := net.ParseIP(addr)
			if parsed == nil {
				continue
			}
			if parsed.To4() != nil {
				service.IPv4 = append(service.IPv4, addr)
			} else {
				service.IPv6 = append(service.IPv6, addr)
			}
		}
		if len(service.IPv4) == 0 && len(service.IPv6) == 0 {
			continue
		}
		out = append(out, service)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// serviceLabel returns the DNS label that the host names a service with.
//
// The control server names a service `svc:litellm`. A colon, a space, and every other
// character that is not a letter, a digit or a hyphen would reach the hosts file and the
// resolver, so serviceLabel drops the `svc:` prefix, maps every other character to a hyphen,
// collapses the hyphens that touch, and trims both ends. It returns an empty string when
// nothing survives or when the result is not one DNS label, and the caller drops the service.
func serviceLabel(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimPrefix(name, daemon.ServiceNamePrefix)

	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	label := strings.Trim(b.String(), "-")
	for strings.Contains(label, "--") {
		label = strings.ReplaceAll(label, "--", "-")
	}
	if len(label) > 63 {
		label = strings.TrimRight(label[:63], "-")
	}
	if !serviceLabelPattern.MatchString(label) {
		return ""
	}
	return label
}

// ServiceHostName returns the name that this host gives a service: `<tailnetID>-<label>`, for
// example `havoc-litellm` for the service that the control server names `svc:litellm`. It is
// the name that BuildServiceRecords writes into the hosts file and the name that the alias
// zone answers for, so a control API response and the console state the word that the operator
// types on the host. It returns an empty string when the name of the service holds no DNS
// label, because the daemon writes no record for such a service.
func ServiceHostName(tailnetID, rawName string) string {
	label := serviceLabel(rawName)
	if label == "" {
		return ""
	}
	return serviceRecordName(tailnetID, label)
}

// serviceRecordName joins the identifier of a tailnet and the label of one of its services
// into the name that the hosts file holds.
func serviceRecordName(tailnetID, label string) string {
	return tailnetID + "-" + label
}

// BuildServiceRecords returns the address records of the services of one tailnet, keyed by
// `<tailnetID>-<service name>`. The key follows the convention of BuildDNSRecords, so a
// service and a peer of one tailnet hold names of one shape, and a service of another tailnet
// never takes the name of a peer of this one.
// A service that holds no address of a family is absent from the map of that family. A
// service that holds several addresses of one family answers under the first of them, because
// the hosts file holds one address per name and per family. Every address of the service
// still reaches a host route, which SyncHostRoutes writes from Service.IPv4 and Service.IPv6.
func BuildServiceRecords(tailnetID string, services []Service) (v4, v6 map[string]string) {
	v4 = make(map[string]string, len(services))
	v6 = make(map[string]string, len(services))
	for _, s := range services {
		name := serviceRecordName(tailnetID, s.Name)
		if len(s.IPv4) > 0 {
			v4[name] = s.IPv4[0]
		}
		if len(s.IPv6) > 0 {
			v6[name] = s.IPv6[0]
		}
	}
	return v4, v6
}

// addServiceAliasRecords adds the address record of each service into the maps of one alias
// zone, keyed by the name of the service, for every name that the zone does not hold. The
// forwarder answers `<service>.<alias>.ts.internal` from them. A peer that holds the name
// keeps it, because the peer is the record that the daemon held first.
func addServiceAliasRecords(zone4, zone6 map[string]string, services []Service) {
	for _, s := range services {
		if len(s.IPv4) > 0 {
			if _, taken := zone4[s.Name]; !taken {
				zone4[s.Name] = s.IPv4[0]
			}
		}
		if len(s.IPv6) > 0 {
			if _, taken := zone6[s.Name]; !taken {
				zone6[s.Name] = s.IPv6[0]
			}
		}
	}
}
