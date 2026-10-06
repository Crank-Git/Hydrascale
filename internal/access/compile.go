package access

import (
	"fmt"
	"net"
	"sort"
	"strconv"
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
var EnforceTail = Tail{{"-j", "DROP"}}

// LogPrefix marks each line that the mode observe writes to the kernel log. The operator
// reads the lines with journalctl -u hydrascale | grep hydrascale-would-deny.
const LogPrefix = "hydrascale-would-deny: "

// logLimit holds 60 packets each minute, because the LOG target on a busy host writes one
// line for every packet that no rule allows.
const logLimit = "60/minute"

// ObserveTail logs a packet that no rule allows and then accepts it.
// The tail accepts rather than returns, because a RETURN rule gives the packet back to
// FORWARD or to INPUT, whose policy is DROP on a host that runs Docker. Issue #238
// measured that loss: the mode observe wrote the would-deny line and the policy of
// FORWARD dropped the same packet one chain later.
// The chain opens with ! -i vh+ ! -o vh+ -j RETURN and the out tail matches one namespace
// device, therefore the ACCEPT applies to the traffic of the daemon alone.
var ObserveTail = Tail{
	{"-m", "limit", "--limit", logLimit, "-j", "LOG", "--log-prefix", LogPrefix},
	{"-j", "ACCEPT"},
}

// TailForMode returns the rules that close each chain in the named mode.
// mode is the value of access.mode, and an empty value means ModeEnforce.
// TailForMode returns an error when the mode is not ModeEnforce and not ModeObserve,
// because a caller that guesses the tail chooses between a host that filters nothing and
// a host that drops everything.
func TailForMode(mode string) (Tail, error) {
	switch mode {
	case "", ModeEnforce:
		return EnforceTail, nil
	case ModeObserve:
		return ObserveTail, nil
	default:
		return nil, fmt.Errorf("invalid mode %q: the daemon runs the modes %s and %s", mode, ModeEnforce, ModeObserve)
	}
}

// Topology holds the host facts that Compile needs. Compile takes them as an argument,
// so that it stays a pure function.
type Topology struct {
	// Devices maps a tailnet identifier to the host side veth device of its namespace.
	Devices map[string]string
	// DNSAddress is the address that the DNS forwarder listens on, in the form host:port.
	DNSAddress string
	// Ports maps a tailnet identifier to the UDP port of its tailscaled, which the host
	// forwards to the namespace. A tailnet without an entry gets no forward rule.
	Ports map[string]int
}

// TopologyIPv6 holds the host facts that CompileIPv6 needs. CompileIPv6 takes them as an
// argument, so that it stays a pure function.
type TopologyIPv6 struct {
	// Devices maps a tailnet identifier to the host side veth device of its namespace.
	Devices map[string]string
	// Ports maps a tailnet identifier to the UDP port of its tailscaled, as in Topology.
	Ports map[string]int
	// HostPrefixes holds each global IPv6 prefix of the host local network, in CIDR form.
	// The internet destination excludes them, as it excludes the RFC 1918 ranges for IPv4.
	HostPrefixes []string
	// GuardedUpstreams holds each upstream device that forwards only because the daemon set
	// force_forwarding on it. The forward chain drops a packet from such a device to a
	// device that is not a namespace device, so the host forwards no new path.
	GuardedUpstreams []string
}

// Compiled holds the rules of both chains, in the order that the daemon writes them.
// Guard holds the forward rules that the Writer puts before the rule that returns a packet
// that touches no namespace device.
type Compiled struct {
	Guard   [][]string
	Forward [][]string
	Out     [][]string
}

// privateRanges holds the address ranges that the internet destination excludes.
// The list holds the three RFC 1918 ranges, the link-local range, and the loopback range.
// The operator decided on 2026-08-05: "Exclude all RFC1918 + the host's subnets".
// The host local network is inside one of these ranges, therefore the compiler needs no
// host command to find it and it stays a pure function.
var privateRanges = []string{
	"10.0.0.0-10.255.255.255",
	"172.16.0.0-172.31.255.255",
	"192.168.0.0-192.168.255.255",
	"169.254.0.0-169.254.255.255",
	"127.0.0.0-127.255.255.255",
}

// privateRangesIPv6 holds the IPv6 address ranges that the internet destination excludes:
// the unique local range, the link-local range, and the loopback address. The unique local
// range holds the tailnet addresses and the veth addresses of every namespace.
var privateRangesIPv6 = []string{
	"fc00::-fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
	"fe80::-febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
	"::1-::1",
}

// excludeRanges returns the matches that keep a rule off every range.
// One iptables rule holds one -d option, therefore the compiler uses the iprange match,
// which a rule holds more than once. Every match must fail for the rule to accept the
// packet, so the exclusions combine as the operator expects.
func excludeRanges(ranges []string) []string {
	args := make([]string, 0, len(ranges)*5)
	for _, r := range ranges {
		args = append(args, "-m", "iprange", "!", "--dst-range", r)
	}
	return args
}

// prefixRange returns the CIDR prefix as the first and the last address, in the form that
// the iprange match reads.
func prefixRange(prefix string) (string, error) {
	_, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", fmt.Errorf("invalid host prefix %q: %w", prefix, err)
	}
	last := make(net.IP, len(ipnet.IP))
	for i := range ipnet.IP {
		last[i] = ipnet.IP[i] | ^ipnet.Mask[i]
	}
	return ipnet.IP.String() + "-" + last.String(), nil
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
	ids := sortedIDs(topo.Devices)
	if err := set.Validate(ids); err != nil {
		return Compiled{}, err
	}

	dnsHost, dnsPort, err := net.SplitHostPort(topo.DNSAddress)
	if err != nil {
		return Compiled{}, fmt.Errorf("invalid DNS forwarder bind address %q: %w", topo.DNSAddress, err)
	}

	// FR-access-14: a namespace reaches the DNS forwarder without a rule, because DNS is
	// how the product works.
	var dns [][]string
	for _, id := range ids {
		for _, protocol := range []string{"udp", "tcp"} {
			dns = append(dns, appendRule(ChainOut,
				[]string{"-i", topo.Devices[id], "-d", dnsHost, "-p", protocol, "--dport", dnsPort, "-j", "ACCEPT"}))
		}
	}

	return compile(set, ids, topo.Devices, listenAccepts(ids, topo.Devices, topo.Ports), dns, privateRanges, tail)
}

// listenAccepts returns one forward rule for each tailnet with a port. The rule accepts
// inbound UDP that the host sent to the namespace for the port of its tailscaled. A peer
// then reaches tailscaled without a hole that its own packets opened, as it reaches the
// host tailscaled. The conntrack match limits the rule to a packet that the listen
// forward rule of the host changed. See issue #404.
func listenAccepts(ids []string, devices map[string]string, ports map[string]int) [][]string {
	var rules [][]string
	for _, id := range ids {
		port, ok := ports[id]
		if !ok {
			continue
		}
		rules = append(rules, appendRule(ChainForward, []string{"-o", devices[id], "-p", "udp",
			"--dport", strconv.Itoa(port), "-m", "conntrack", "--ctstate", "DNAT", "-j", "ACCEPT"}))
	}
	return rules
}

// CompileIPv6 returns the ip6tables arguments that the rule set requires.
// set is the rule set, topo holds the veth device of each tailnet and the IPv6 facts of
// the host, and tail holds the rules that close each chain.
// CompileIPv6 is pure, as Compile is. The IPv6 chains hold no DNS rule, because a
// namespace reaches the DNS forwarder over IPv4. The out chain opens neighbor discovery
// from each namespace device, because neighbor discovery is ICMPv6 and the closing drop
// would stop the solicitation for the gateway. ARP never enters the IPv4 chain.
// CompileIPv6 returns an error when a rule fails validation, when the topology names no
// device for a tailnet, or when a host prefix is not a CIDR prefix.
func CompileIPv6(set RuleSet, topo TopologyIPv6, tail Tail) (Compiled, error) {
	ids := sortedIDs(topo.Devices)
	if err := set.Validate(ids); err != nil {
		return Compiled{}, err
	}

	ranges := append([]string{}, privateRangesIPv6...)
	for _, prefix := range topo.HostPrefixes {
		r, err := prefixRange(prefix)
		if err != nil {
			return Compiled{}, err
		}
		ranges = append(ranges, r)
	}

	var ndp [][]string
	for _, id := range ids {
		for _, kind := range []string{"neighbour-solicitation", "neighbour-advertisement"} {
			ndp = append(ndp, appendRule(ChainOut,
				[]string{"-i", topo.Devices[id], "-p", "ipv6-icmp", "--icmpv6-type", kind, "-j", "ACCEPT"}))
		}
	}

	c, err := compile(set, ids, topo.Devices, listenAccepts(ids, topo.Devices, topo.Ports), ndp, ranges, tail)
	if err != nil {
		return Compiled{}, err
	}
	for _, upstream := range topo.GuardedUpstreams {
		c.Guard = append(c.Guard, appendRule(ChainForward, []string{"-i", upstream, "!", "-o", devicePrefix + "+", "-j", "DROP"}))
	}
	return c, nil
}

// sortedIDs returns the tailnet identifiers of the device map in a fixed order.
func sortedIDs(devices map[string]string) []string {
	ids := make([]string, 0, len(devices))
	for id := range devices {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// compile returns the rules of both chains for one address family.
// listen holds the forward rules and open holds the out rules that every namespace needs
// without a rule of the operator. private holds the ranges that the internet destination
// excludes.
func compile(set RuleSet, ids []string, devices map[string]string, listen, open [][]string, private []string, tail Tail) (Compiled, error) {
	forward := append([][]string{appendRule(ChainForward, establishedMatch)}, listen...)
	out := append([][]string{appendRule(ChainOut, establishedMatch)}, open...)

	for _, rule := range set.Rules {
		chain, match, err := compileRule(rule, devices, private)
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
			out = append(out, appendRule(ChainOut, append([]string{"-i", devices[id]}, closing...)))
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
// devices maps a tailnet identifier to its device, and private holds the ranges that the
// internet destination excludes.
func compileRule(rule Rule, devices map[string]string, private []string) (string, []string, error) {
	if rule.From == Host {
		return "", nil, nil
	}
	source, ok := devices[rule.From]
	if !ok {
		return "", nil, fmt.Errorf("rule %s: the topology names no device for the tailnet %q", rule, rule.From)
	}

	switch rule.To {
	case Host:
		return ChainOut, []string{"-i", source}, nil
	case Internet:
		// The internet destination is a public address only. The match excludes every
		// namespace device, the three RFC 1918 ranges, the link-local range, and the
		// loopback range. Without the range exclusion a namespace reaches the host local
		// network, which is the finding SA-9.
		match := []string{"-i", source, "!", "-o", devicePrefix + "+"}
		return ChainForward, append(match, excludeRanges(private)...), nil
	default:
		destination, ok := devices[rule.To]
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
