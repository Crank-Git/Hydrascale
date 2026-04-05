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
	cgnatNet  *net.IPNet
	tsV6Net   *net.IPNet
	magicDNS  = "100.100.100.100"
)

func init() {
	_, cgnatNet, _ = net.ParseCIDR("100.64.0.0/10")
	_, tsV6Net, _ = net.ParseCIDR("fd7a:115c:a1e0::/48")
}

// isCGNAT reports whether ip is in the Tailscale CGNAT range 100.64.0.0/10.
func isCGNAT(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return cgnatNet.Contains(parsed)
}

// isTailscaleV6 reports whether ip is in fd7a:115c:a1e0::/48.
func isTailscaleV6(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return tsV6Net.Contains(parsed)
}

// parseHostRoutes parses `ip route show` output and returns host routes
// that are in the CGNAT range on vethDev, excluding the MagicDNS address.
func parseHostRoutes(output string, vethDev string) []string {
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
		if dest == "default" {
			continue
		}
		// Exclude MagicDNS infrastructure address
		if dest == magicDNS {
			continue
		}
		// Must be a host route (bare IP, no prefix) or /32
		ip := strings.TrimSuffix(dest, "/32")
		if !isCGNAT(ip) {
			continue
		}
		// Must be on the expected veth device
		if vethDev != "" && !strings.Contains(line, "dev "+vethDev) {
			continue
		}
		routes = append(routes, ip)
	}
	return routes
}

// parseHostRoutesV6 parses `ip -6 route show` output and returns host routes
// in the Tailscale IPv6 range on vethDev.
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
		ip := strings.TrimSuffix(dest, "/128")
		if !isTailscaleV6(ip) {
			continue
		}
		if vethDev != "" && !strings.Contains(line, "dev "+vethDev) {
			continue
		}
		routes = append(routes, ip)
	}
	return routes
}

// desiredRoutes extracts the v4 and v6 peer IPs from a TailnetPeers set.
func desiredRoutes(peers TailnetPeers) (v4, v6 []string) {
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

// diffRoutes returns the IPs to add (in desired but not actual) and to remove
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

// SyncHostRoutes synchronises host routing table entries for all peers in the
// TailnetPeers set, routing their IPs via the veth gateway.
func SyncHostRoutes(peers TailnetPeers) error {
	vethDev := peers.VethHost
	gw := peers.VethGateway

	// Gather current host routes
	v4Out, err := exec.Command("ip", "route", "show").Output()
	if err != nil {
		return fmt.Errorf("ip route show: %w", err)
	}
	v6Out, err := exec.Command("ip", "-6", "route", "show").Output()
	if err != nil {
		return fmt.Errorf("ip -6 route show: %w", err)
	}

	actualV4 := parseHostRoutes(string(v4Out), vethDev)
	actualV6 := parseHostRoutesV6(string(v6Out), vethDev)

	wantV4, wantV6 := desiredRoutes(peers)

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
		args := []string{"-6", "route", "replace", ip, "via", gw, "dev", vethDev}
		if out, e := exec.Command("ip", args...).CombinedOutput(); e != nil {
			errs = append(errs, fmt.Errorf("ip -6 route replace %s: %w (%s)", ip, e, out))
		} else {
			log.Printf("hostaccess: added v6 route %s via %s dev %s", ip, gw, vethDev)
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

// RemoveAllHostRoutes removes all CGNAT and Tailscale v6 host routes on vethDev.
func RemoveAllHostRoutes(vethDev string) {
	v4Out, err := exec.Command("ip", "route", "show").Output()
	if err == nil {
		for _, ip := range parseHostRoutes(string(v4Out), vethDev) {
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
