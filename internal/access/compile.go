package access

import (
	"fmt"
	"net"
	"sort"
)

// ChainForward holds the rules for traffic that the host forwards between namespaces and
// out to the internet.
const ChainForward = "HYDRASCALE-FWD"

// ChainOut holds the rules for traffic that a namespace sends to the host itself.
const ChainOut = "HYDRASCALE-OUT"

// devicePrefix is the first part of every host side veth device name that
// internal/namespaces/ns.go:220 builds. The compiler matches "vh+" to name every
// namespace device in one rule, which the internet destination needs.
const devicePrefix = "vh"

// Tail holds the rules that close a chain. Each element is one argument list, without
// the chain name and without a match.
type Tail [][]string

// EnforceTail drops a packet that no rule allows.
// Issue #136 adds the tail that the mode observe needs, which logs and returns.
var EnforceTail = Tail{{"-j", "DROP"}}

// Topology holds the host facts that Compile needs. Compile takes them as an argument,
// so that it stays a pure function.
type Topology struct {
	// Devices maps a tailnet identifier to the host side veth device of its namespace.
	Devices map[string]string
	// DNSAddress is the address that the DNS forwarder listens on, in the form host:port.
	DNSAddress string
}

// Compiled holds the rules of both chains, in the order that the daemon writes them.
type Compiled struct {
	Forward [][]string
	Out     [][]string
}

// establishedMatch allows return traffic for a connection that a rule already allowed.
var establishedMatch = []string{"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}

// Compile returns the iptables arguments that the rule set requires.
// set is the rule set, topo holds the veth device of each tailnet and the DNS forwarder
// bind address, and tail holds the rules that close each chain.
// Compile is pure: the same rule set, the same topology, and the same tail always
// produce the same output.
// Compile returns an error when a rule fails validation, when the topology names no
// device for a tailnet, or when the DNS forwarder bind address is not a host and a port.
// Compile returns an empty result together with an error, because a rule set that fails
// validation is never applied in part.
func Compile(set RuleSet, topo Topology, tail Tail) (Compiled, error) {
	ids := make([]string, 0, len(topo.Devices))
	for id := range topo.Devices {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	if err := set.Validate(ids); err != nil {
		return Compiled{}, err
	}

	dnsHost, dnsPort, err := net.SplitHostPort(topo.DNSAddress)
	if err != nil {
		return Compiled{}, fmt.Errorf("invalid DNS forwarder bind address %q: %w", topo.DNSAddress, err)
	}

	forward := [][]string{appendRule(ChainForward, establishedMatch)}
	out := [][]string{appendRule(ChainOut, establishedMatch)}

	// FR-access-14: a namespace reaches the DNS forwarder without a rule, because DNS is
	// how the product works.
	for _, id := range ids {
		for _, protocol := range []string{"udp", "tcp"} {
			out = append(out, appendRule(ChainOut,
				[]string{"-i", topo.Devices[id], "-d", dnsHost, "-p", protocol, "--dport", dnsPort, "-j", "ACCEPT"}))
		}
	}

	for _, rule := range set.Rules {
		chain, match, err := compileRule(rule, topo)
		if err != nil {
			return Compiled{}, err
		}
		if chain == "" {
			continue
		}
		for _, args := range withPorts(match, rule.Ports) {
			args = append(args, "-j", "ACCEPT")
			if chain == ChainForward {
				forward = append(forward, appendRule(chain, args))
			} else {
				out = append(out, appendRule(chain, args))
			}
		}
	}

	for _, closing := range tail {
		forward = append(forward, appendRule(ChainForward, closing))
	}
	// The INPUT chain carries the traffic of every host interface, therefore the tail of
	// the out chain names one namespace device rather than every packet.
	for _, id := range ids {
		for _, closing := range tail {
			out = append(out, appendRule(ChainOut, append([]string{"-i", topo.Devices[id]}, closing...)))
		}
	}

	return Compiled{Forward: forward, Out: out}, nil
}

// appendRule returns one complete argument list for iptables-restore.
func appendRule(chain string, args []string) []string {
	rule := make([]string, 0, len(args)+2)
	rule = append(rule, "-A", chain)
	return append(rule, args...)
}

// compileRule returns the chain and the match of one rule.
// compileRule returns an empty chain for a rule whose source is the host, because the
// host originates that traffic on the OUTPUT chain, which version 1.0 does not filter.
func compileRule(rule Rule, topo Topology) (string, []string, error) {
	if rule.From == Host {
		return "", nil, nil
	}
	source, ok := topo.Devices[rule.From]
	if !ok {
		return "", nil, fmt.Errorf("rule %s: the topology names no device for the tailnet %q", rule, rule.From)
	}

	switch rule.To {
	case Host:
		return ChainOut, []string{"-i", source}, nil
	case Internet:
		// The internet destination is every address that no namespace device serves.
		return ChainForward, []string{"-i", source, "!", "-o", devicePrefix + "+"}, nil
	default:
		destination, ok := topo.Devices[rule.To]
		if !ok {
			return "", nil, fmt.Errorf("rule %s: the topology names no device for the tailnet %q", rule, rule.To)
		}
		return ChainForward, []string{"-i", source, "-o", destination}, nil
	}
}

// withPorts returns one match per port entry, or the match alone when the port list is
// empty. FR-access-9: an empty port list allows every port and every protocol.
func withPorts(match []string, ports []string) [][]string {
	if len(ports) == 0 {
		return [][]string{match}
	}
	out := make([][]string, 0, len(ports))
	for _, entry := range ports {
		// Validate already accepted every entry, so the parse cannot fail here.
		p, _ := parsePort(entry)
		args := make([]string, 0, len(match)+4)
		args = append(args, match...)
		out = append(out, append(args, p.match()...))
	}
	return out
}
