package hostaccess

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
)

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
func listNsTableRoutes(nsName string, infraSubnet string) ([]string, error) {
	out, err := exec.Command("ip", "netns", "exec", nsName, "ip", "route", "show", "table", "52").Output()
	if err != nil {
		return nil, fmt.Errorf("ip netns exec %s ip route show table 52: %w", nsName, err)
	}
	return parseTableRoutes(string(out), infraSubnet), nil
}

// listNsTableRoutesV6 reads IPv6 routing table 52 from inside a namespace,
// returning only non-Tailscale subnet routes.
func listNsTableRoutesV6(nsName string) ([]string, error) {
	out, err := exec.Command("ip", "netns", "exec", nsName, "ip", "-6", "route", "show", "table", "52").Output()
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
		routes = append(routes, dest)
	}
	return routes, nil
}

// parseRouteGetOutput extracts the `dev` interface name and whether a `via`
// gateway is present from the first line of `ip [-6] route get` output.
// Examples:
//
//	"192.168.1.0 dev eth0 src 192.168.1.50"          -> dev=eth0, hasVia=false
//	"10.42.0.0 via 192.168.1.1 dev eth0 src ..."     -> dev=eth0, hasVia=true
//	"100.64.0.5 via 10.200.0.1 dev vh<hash> src ..." -> dev=vh<hash>, hasVia=true
func parseRouteGetOutput(output string) (dev string, hasVia bool) {
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
		}
	}
	return dev, hasVia
}

// wouldClobberDirectLAN reports whether installing a host route to dest
// would shadow a directly-connected, non-vh* route (e.g. the LAN on eth0).
// We probe with `ip [-6] route get <network-addr>`: if the kernel resolves
// the destination via a non-vh* device with no `via` gateway, the destination
// is on a directly-attached LAN and we must not replace its route. Issue #21.
func wouldClobberDirectLAN(dest string, v6 bool) bool {
	addr := dest
	if i := strings.Index(addr, "/"); i >= 0 {
		addr = addr[:i]
	}
	args := []string{"route", "get", addr}
	if v6 {
		args = []string{"-6", "route", "get", addr}
	}
	out, err := exec.Command("ip", args...).Output()
	if err != nil {
		// If the kernel can't resolve it (no route at all), installing
		// our route is fine — there's nothing to clobber.
		return false
	}
	dev, hasVia := parseRouteGetOutput(string(out))
	if strings.HasPrefix(dev, "vh") {
		// One of ours (or a sibling tailnet's). Replace is idempotent / safe.
		return false
	}
	// Non-vh* dev with no `via` means the destination is reached directly
	// over an attached LAN — installing our route would steal that traffic.
	return !hasVia
}

// filterClobberingRoutes drops candidate destinations that would shadow a
// directly-connected non-vh* route on the host. Skipped entries are logged.
func filterClobberingRoutes(candidates []string, v6 bool) []string {
	if len(candidates) == 0 {
		return candidates
	}
	out := make([]string, 0, len(candidates))
	for _, dest := range candidates {
		if wouldClobberDirectLAN(dest, v6) {
			log.Printf("hostaccess: skipping route %s — would clobber a directly-connected host LAN route", dest)
			continue
		}
		out = append(out, dest)
	}
	return out
}

// SyncHostRoutes synchronises host routing table entries for all peers in the
// TailnetPeers set and for any accepted subnet routes from table 52 in the namespace.
// Both desired sets are merged before diffing so that peer routes don't remove
// subnet routes (or vice versa).
func SyncHostRoutes(peers TailnetPeers, infraSubnet string) error {
	vethDev := peers.VethHost
	gw := peers.VethGateway

	// Build the combined desired set: peer IPs + accepted routes from table 52
	wantV4, wantV6 := desiredPeerRoutes(peers)

	if peers.NsName != "" {
		nsV4, err := listNsTableRoutes(peers.NsName, infraSubnet)
		if err != nil {
			log.Printf("hostaccess: failed to read table 52 v4 routes from %s: %v", peers.NsName, err)
		}
		nsV6, err := listNsTableRoutesV6(peers.NsName)
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
	wantV4 = filterClobberingRoutes(wantV4, false)
	wantV6 = filterClobberingRoutes(wantV6, true)

	// Gather current host routes
	v4Out, err := exec.Command("ip", "route", "show").Output()
	if err != nil {
		return fmt.Errorf("ip route show: %w", err)
	}
	v6Out, err := exec.Command("ip", "-6", "route", "show").Output()
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
		if out, e := exec.Command("ip", args...).CombinedOutput(); e != nil {
			errs = append(errs, fmt.Errorf("ip route replace %s: %w (%s)", ip, e, out))
		} else {
			log.Printf("hostaccess: added route %s via %s dev %s", ip, gw, vethDev)
		}
	}
	for _, ip := range delV4 {
		if out, e := exec.Command("ip", "route", "del", ip).CombinedOutput(); e != nil {
			errs = append(errs, fmt.Errorf("ip route del %s: %w (%s)", ip, e, out))
		} else {
			log.Printf("hostaccess: removed route %s", ip)
		}
	}
	for _, ip := range addV6 {
		args := []string{"-6", "route", "replace", ip, "dev", vethDev}
		if out, e := exec.Command("ip", args...).CombinedOutput(); e != nil {
			errs = append(errs, fmt.Errorf("ip -6 route replace %s: %w (%s)", ip, e, out))
		} else {
			log.Printf("hostaccess: added v6 route %s dev %s", ip, vethDev)
		}
	}
	for _, ip := range delV6 {
		if out, e := exec.Command("ip", "-6", "route", "del", ip).CombinedOutput(); e != nil {
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
func RemoveAllHostRoutes(vethDev string, infraSubnet string) {
	v4Out, err := exec.Command("ip", "route", "show").Output()
	if err == nil {
		for _, ip := range parseHostRoutes(string(v4Out), vethDev, infraSubnet) {
			if out, e := exec.Command("ip", "route", "del", ip).CombinedOutput(); e != nil {
				log.Printf("hostaccess: remove route %s: %v (%s)", ip, e, out)
			}
		}
	}

	v6Out, err := exec.Command("ip", "-6", "route", "show").Output()
	if err == nil {
		for _, ip := range parseHostRoutesV6(string(v6Out), vethDev) {
			if out, e := exec.Command("ip", "-6", "route", "del", ip).CombinedOutput(); e != nil {
				log.Printf("hostaccess: remove v6 route %s: %v (%s)", ip, e, out)
			}
		}
	}
}
