package hostaccess

import (
	"errors"
	"fmt"
	"log"
	"maps"
	"net"
	"slices"
	"strings"
	"time"
)

// hostCommandTimeout is the time that one command to the host gets. A command that does
// not return blocks the reconcile cycle, so every command carries this deadline.
// See SA-21.
const hostCommandTimeout = 5 * time.Second

var (
	cgnatNet *net.IPNet
	tsV6Net  *net.IPNet
	magicDNS = "100.100.100.100"

	// Tailscale exit-node split defaults. tailscaled installs these into
	// the namespace's table 52 when an exit node is selected; they should
	// never be replicated to the host's main routing table because doing
	// so funnels every host packet (including DNS to public resolvers)
	// into the namespace. See issue #21.
	exitNodeSplitDefaults = map[string]struct{}{
		"0.0.0.0/1":   {},
		"128.0.0.0/1": {},
		"::/1":        {},
		"8000::/1":    {},
	}
)

func init() {
	_, cgnatNet, _ = net.ParseCIDR("100.64.0.0/10")
	_, tsV6Net, _ = net.ParseCIDR("fd7a:115c:a1e0::/48")
}

// validRouteDest reports whether dest is an address or a CIDR block.
//
// The control server advertises the destination, and tailscaled writes it into table 52.
// The destination becomes one argument of `ip route replace`, and ip reads an argument
// that starts with a hyphen as an option. See SA-18.
func validRouteDest(dest string) bool {
	if net.ParseIP(dest) != nil {
		return true
	}
	_, _, err := net.ParseCIDR(dest)
	return err == nil
}

// isExitNodeSplitDefault reports whether dest is one of the Tailscale
// exit-node split-default routes (0.0.0.0/1, 128.0.0.0/1, ::/1, 8000::/1).
func isExitNodeSplitDefault(dest string) bool {
	_, ok := exitNodeSplitDefaults[dest]
	return ok
}

// isCGNAT reports whether dest is in the Tailscale CGNAT range 100.64.0.0/10.
func isCGNAT(dest string) bool {
	if ip := net.ParseIP(dest); ip != nil {
		return cgnatNet.Contains(ip)
	}
	if ip, _, err := net.ParseCIDR(dest); err == nil {
		return cgnatNet.Contains(ip)
	}
	return false
}

// isTailscaleV6 reports whether dest is in fd7a:115c:a1e0::/48.
func isTailscaleV6(dest string) bool {
	if ip := net.ParseIP(dest); ip != nil {
		return tsV6Net.Contains(ip)
	}
	if ip, _, err := net.ParseCIDR(dest); err == nil {
		return tsV6Net.Contains(ip)
	}
	return false
}

// parseHostRoutes parses `ip route show` output and returns route destinations
// on vethDev, excluding MagicDNS and infra routes.
func parseHostRoutes(output string, vethDev string, infraSubnet string) []string {
	var routes []string
	_, infraNet, _ := net.ParseCIDR(infraSubnet)

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		dest := fields[0]
		if dest == "default" {
			continue
		}
		if dest == magicDNS {
			continue
		}
		if infraNet != nil {
			if destIP, _, err := net.ParseCIDR(dest); err == nil {
				if infraNet.Contains(destIP) {
					continue
				}
			} else if destIP := net.ParseIP(dest); destIP != nil {
				if infraNet.Contains(destIP) {
					continue
				}
			}
		}
		if vethDev != "" && !strings.Contains(line, "dev "+vethDev) {
			continue
		}
		if !validRouteDest(dest) {
			log.Printf("hostaccess: the destination %q is not an address and not a CIDR block", dest)
			continue
		}
		routes = append(routes, dest)
	}
	return routes
}

// parseHostRoutesV6 parses `ip -6 route show` output and returns route destinations
// on vethDev, excluding default routes.
func parseHostRoutesV6(output string, vethDev string) []string {
	var routes []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		dest := fields[0]
		if dest == "default" || dest == "::/0" {
			continue
		}
		if vethDev != "" && !strings.Contains(line, "dev "+vethDev) {
			continue
		}
		if !validRouteDest(dest) {
			log.Printf("hostaccess: the destination %q is not an address and not a CIDR block", dest)
			continue
		}
		routes = append(routes, dest)
	}
	return routes
}

// parseTableRoutes parses `ip route show table N` output and returns only
// subnet route destinations. It excludes default routes, infra subnet,
// MagicDNS, and Tailscale CGNAT/v6 ranges (peer IPs and catch-alls that
// are already handled by the peer route sync or veth setup).
func parseTableRoutes(output string, infraSubnet string) []string {
	var routes []string
	_, infraNet, _ := net.ParseCIDR(infraSubnet)

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		dest := fields[0]
		if dest == "default" || dest == "0.0.0.0/0" || dest == "::/0" {
			continue
		}
		if isExitNodeSplitDefault(dest) {
			continue
		}
		if dest == magicDNS {
			continue
		}
		if isCGNAT(dest) {
			continue
		}
		if isTailscaleV6(dest) {
			continue
		}
		if infraNet != nil {
			if destIP, _, err := net.ParseCIDR(dest); err == nil {
				if infraNet.Contains(destIP) {
					continue
				}
			} else if destIP := net.ParseIP(dest); destIP != nil {
				if infraNet.Contains(destIP) {
					continue
				}
			}
		}
		if !validRouteDest(dest) {
			log.Printf("hostaccess: the destination %q is not an address and not a CIDR block", dest)
			continue
		}
		routes = append(routes, dest)
	}
	return routes
}

// desiredPeerRoutes extracts the v4 and v6 peer IPs from a TailnetPeers set.
func desiredPeerRoutes(peers TailnetPeers) (v4, v6 []string) {
	for _, p := range peers.Peers {
		if p.IPv4 != "" {
			v4 = append(v4, p.IPv4)
		}
		if p.IPv6 != "" {
			v6 = append(v6, p.IPv6)
		}
	}
	return v4, v6
}

// diffRoutes returns the destinations to add (in desired but not actual) and to remove
// (in actual but not desired).
func diffRoutes(desired, actual []string) (toAdd, toRemove []string) {
	desiredSet := make(map[string]bool, len(desired))
	for _, ip := range desired {
		desiredSet[ip] = true
	}
	actualSet := make(map[string]bool, len(actual))
	for _, ip := range actual {
		actualSet[ip] = true
	}

	for _, ip := range desired {
		if !actualSet[ip] {
			toAdd = append(toAdd, ip)
		}
	}
	for _, ip := range actual {
		if !desiredSet[ip] {
			toRemove = append(toRemove, ip)
		}
	}
	return toAdd, toRemove
}

// listNsTableRoutes reads routing table 52 from inside a namespace and returns
// the route destinations (subnet routes accepted by tailscaled via --accept-routes).
func (m *Manager) listNsTableRoutes(nsName string, infraSubnet string) ([]string, error) {
	out, err := m.run("ip", "netns", "exec", nsName, "ip", "route", "show", "table", "52")
	if err != nil {
		return nil, fmt.Errorf("ip netns exec %s ip route show table 52: %w", nsName, err)
	}
	return parseTableRoutes(string(out), infraSubnet), nil
}

// listNsTableRoutesV6 reads IPv6 routing table 52 from inside a namespace,
// returning only non-Tailscale subnet routes.
func (m *Manager) listNsTableRoutesV6(nsName string) ([]string, error) {
	out, err := m.run("ip", "netns", "exec", nsName, "ip", "-6", "route", "show", "table", "52")
	if err != nil {
		return nil, fmt.Errorf("ip netns exec %s ip -6 route show table 52: %w", nsName, err)
	}
	var routes []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		dest := fields[0]
		if dest == "default" || dest == "::/0" {
			continue
		}
		if isExitNodeSplitDefault(dest) {
			continue
		}
		if isTailscaleV6(dest) {
			continue
		}
		if !validRouteDest(dest) {
			log.Printf("hostaccess: the destination %q is not an address and not a CIDR block", dest)
			continue
		}
		routes = append(routes, dest)
	}
	return routes, nil
}

// parseRouteGetOutput extracts the `dev` interface name, whether a `via` gateway is
// present, and the routing table that answered, from the first line of `ip [-6] route get`
// output. The table is empty when the kernel names none, which means the main table.
// Examples:
//
//	"192.168.1.0 dev eth0 src 192.168.1.50"          -> dev=eth0, hasVia=false, table=""
//	"10.42.0.0 via 192.168.1.1 dev eth0 src ..."     -> dev=eth0, hasVia=true, table=""
//	"100.64.0.5 via 10.200.0.1 dev vh<hash> src ..." -> dev=vh<hash>, hasVia=true, table=""
//	"fd7a::1 dev tailscale0 table 52 src ..."        -> dev=tailscale0, hasVia=false, table=52
func parseRouteGetOutput(output string) (dev string, hasVia bool, table string) {
	line := strings.SplitN(strings.TrimSpace(output), "\n", 2)[0]
	fields := strings.Fields(line)
	for i, f := range fields {
		switch f {
		case "dev":
			if i+1 < len(fields) {
				dev = fields[i+1]
			}
		case "via":
			hasVia = true
		case "table":
			if i+1 < len(fields) {
				table = fields[i+1]
			}
		}
	}
	return dev, hasVia, table
}

// skipReasonForRoute returns the reason that the daemon writes no host route to dest, and
// it returns an empty string when the daemon writes the route.
//
// The probe is `ip [-6] route get <address>`. It reports two conditions:
//
//   - Another routing table answers before the main table. The daemon writes its route
//     into the main table, therefore that route reaches no packet. A host that runs its
//     own tailscaled holds `fd7a:115c:a1e0::/48 dev tailscale0` in the table 52, and
//     `ip rule` places the lookup of the table 52 before the lookup of the main table.
//     Every IPv6 address of every tailnet falls inside that range. See issue #273.
//   - The host reaches the address over a directly connected network. A route of the
//     daemon would take that traffic. See issue #21.
//
// A device that the daemon owns is neither condition: a replace of its own route is safe.
func (m *Manager) skipReasonForRoute(dest string, v6 bool) string {
	addr := dest
	if i := strings.Index(addr, "/"); i >= 0 {
		addr = addr[:i]
	}
	args := []string{"route", "get", addr}
	if v6 {
		args = []string{"-6", "route", "get", addr}
	}
	out, err := m.run("ip", args...)
	if err != nil {
		// The kernel resolves the destination over no route, therefore the daemon
		// replaces nothing and it writes its own route.
		return ""
	}
	dev, hasVia, table := parseRouteGetOutput(string(out))
	if strings.HasPrefix(dev, "vh") {
		// One of ours (or a sibling tailnet's). Replace is idempotent / safe.
		return ""
	}
	// A table that the kernel names is not the main table, and `ip rule` reaches it first.
	// This test comes before the test for a directly connected network, because an answer
	// out of a policy routing table names a device and carries no `via` gateway, and the
	// address is on no attached network.
	if table != "" && table != "main" {
		return fmt.Sprintf("the routing table %s answers for that address before the main table, on the device %s", table, dev)
	}
	if !hasVia {
		return fmt.Sprintf("the host reaches that address over a directly connected network on the device %s", dev)
	}
	return ""
}

// filterWritableRoutes returns the destinations that the daemon writes, and it records one
// line for each reason that it left a destination to the host.
//
// The line carries the count and one example rather than one line per destination. A host
// that runs its own tailscaled answers for every IPv6 address of every tailnet out of the
// table 52, and the reconcile cycle runs every ten seconds, therefore one line for each
// destination filled the log of the host. See issue #273.
func (m *Manager) filterWritableRoutes(candidates []string, v6 bool) []string {
	if len(candidates) == 0 {
		return candidates
	}
	out := make([]string, 0, len(candidates))
	skipped := make(map[string][]string)
	for _, dest := range candidates {
		if !validRouteDest(dest) {
			log.Printf("hostaccess: the destination %q is not an address and not a CIDR block", dest)
			continue
		}
		if reason := m.skipReasonForRoute(dest, v6); reason != "" {
			skipped[reason] = append(skipped[reason], dest)
			continue
		}
		out = append(out, dest)
	}
	for _, reason := range slices.Sorted(maps.Keys(skipped)) {
		dests := skipped[reason]
		if len(dests) == 1 {
			log.Printf("hostaccess: the daemon writes no route to %s, because %s", dests[0], reason)
			continue
		}
		log.Printf("hostaccess: the daemon writes no route to %d addresses, %s among them, because %s",
			len(dests), dests[0], reason)
	}
	return out
}

// SyncHostRoutes synchronises host routing table entries for all peers in the
// TailnetPeers set and for any accepted subnet routes from table 52 in the namespace.
// Both desired sets are merged before diffing so that peer routes don't remove
// subnet routes (or vice versa).
func (m *Manager) SyncHostRoutes(peers TailnetPeers) error {
	infraSubnet := m.infraSubnet
	vethDev := peers.VethHost
	gw := peers.VethGateway

	// Build the combined desired set: peer IPs + accepted routes from table 52
	wantV4, wantV6 := desiredPeerRoutes(peers)

	if peers.NsName != "" {
		nsV4, err := m.listNsTableRoutes(peers.NsName, infraSubnet)
		if err != nil {
			log.Printf("hostaccess: failed to read table 52 v4 routes from %s: %v", peers.NsName, err)
		}
		nsV6, err := m.listNsTableRoutesV6(peers.NsName)
		if err != nil {
			log.Printf("hostaccess: failed to read table 52 v6 routes from %s: %v", peers.NsName, err)
		}
		wantV4 = mergeRoutes(wantV4, nsV4)
		wantV6 = mergeRoutes(wantV6, nsV6)
	}

	// Drop any candidate that would shadow a directly-connected host LAN
	// route. Peer IPs are CGNAT (or fd7a:115c:a1e0::/48) so they never
	// overlap a real LAN; the filter is meaningful for accepted subnet
	// routes from table 52. See issue #21.
	wantV4 = m.filterWritableRoutes(wantV4, false)
	wantV6 = m.filterWritableRoutes(wantV6, true)

	// Gather current host routes
	v4Out, err := m.run("ip", "route", "show")
	if err != nil {
		return fmt.Errorf("ip route show: %w", err)
	}
	v6Out, err := m.run("ip", "-6", "route", "show")
	if err != nil {
		return fmt.Errorf("ip -6 route show: %w", err)
	}

	actualV4 := parseHostRoutes(string(v4Out), vethDev, infraSubnet)
	actualV6 := parseHostRoutesV6(string(v6Out), vethDev)

	addV4, delV4 := diffRoutes(wantV4, actualV4)
	addV6, delV6 := diffRoutes(wantV6, actualV6)

	var errs []error

	for _, ip := range addV4 {
		args := []string{"route", "replace", ip, "via", gw, "dev", vethDev}
		if out, e := m.run("ip", args...); e != nil {
			errs = append(errs, fmt.Errorf("ip route replace %s: %w (%s)", ip, e, out))
		} else {
			log.Printf("hostaccess: added route %s via %s dev %s", ip, gw, vethDev)
		}
	}
	for _, ip := range delV4 {
		if out, e := m.run("ip", "route", "del", ip); e != nil {
			errs = append(errs, fmt.Errorf("ip route del %s: %w (%s)", ip, e, out))
		} else {
			log.Printf("hostaccess: removed route %s", ip)
		}
	}
	for _, ip := range addV6 {
		args := []string{"-6", "route", "replace", ip, "dev", vethDev}
		if out, e := m.run("ip", args...); e != nil {
			errs = append(errs, fmt.Errorf("ip -6 route replace %s: %w (%s)", ip, e, out))
		} else {
			log.Printf("hostaccess: added v6 route %s dev %s", ip, vethDev)
		}
	}
	for _, ip := range delV6 {
		if out, e := m.run("ip", "-6", "route", "del", ip); e != nil {
			errs = append(errs, fmt.Errorf("ip -6 route del %s: %w (%s)", ip, e, out))
		} else {
			log.Printf("hostaccess: removed v6 route %s", ip)
		}
	}

	return errors.Join(errs...)
}

// mergeRoutes merges two route lists, deduplicating by destination.
func mergeRoutes(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	var result []string
	for _, r := range a {
		if !seen[r] {
			seen[r] = true
			result = append(result, r)
		}
	}
	for _, r := range b {
		if !seen[r] {
			seen[r] = true
			result = append(result, r)
		}
	}
	return result
}

// RemoveAllHostRoutes removes all host routes on vethDev (excluding MagicDNS and infra).
// A step that fails does not stop the remaining steps. RemoveAllHostRoutes collects every
// failure and returns the failures together.
func (m *Manager) RemoveAllHostRoutes(vethDev string) error {
	infraSubnet := m.infraSubnet
	var errs []error

	v4Out, err := m.run("ip", "route", "show")
	if err != nil {
		errs = append(errs, fmt.Errorf("ip route show: %w", err))
	} else {
		for _, ip := range parseHostRoutes(string(v4Out), vethDev, infraSubnet) {
			if out, e := m.run("ip", "route", "del", ip); e != nil {
				errs = append(errs, fmt.Errorf("ip route del %s: %w (%s)", ip, e, out))
			}
		}
	}

	v6Out, err := m.run("ip", "-6", "route", "show")
	if err != nil {
		errs = append(errs, fmt.Errorf("ip -6 route show: %w", err))
	} else {
		for _, ip := range parseHostRoutesV6(string(v6Out), vethDev) {
			if out, e := m.run("ip", "-6", "route", "del", ip); e != nil {
				errs = append(errs, fmt.Errorf("ip -6 route del %s: %w (%s)", ip, e, out))
			}
		}
	}

	return errors.Join(errs...)
}
