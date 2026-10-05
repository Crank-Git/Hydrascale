package namespaces

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
)

// IPv6InfraPrefix is the unique local /48 prefix that holds the IPv6 address of every veth
// pair. Each namespace takes the /64 whose fourth group is its veth index. The host
// translates each /64 to its own global address, therefore the prefix never leaves the
// host. See issue #406.
const IPv6InfraPrefix = "fd5c:9a3e:7b10"

// VethIPv6 returns the IPv6 addresses of the veth pair of the veth index.
// hostIP and nsIP carry the prefix length, hostGW is the bare host address, and prefix is
// the /64 of the pair.
func VethIPv6(index int) (hostIP, nsIP, hostGW, prefix string) {
	base := fmt.Sprintf("%s:%x::", IPv6InfraPrefix, index)
	return base + "1/64", base + "2/64", base + "1", base + "/64"
}

// IPv6Host holds the IPv6 facts of the host that the daemon reads on each tick.
type IPv6Host struct {
	// Uplinks holds the device of each IPv6 default route of the main table.
	Uplinks []string
	// Prefixes holds the prefix of each global IPv6 address of a device that is not a
	// namespace device, in CIDR form.
	Prefixes []string
	// ForceForwarding is true when the kernel holds net.ipv6.conf.*.force_forwarding.
	// Linux 6.17 adds it.
	ForceForwarding bool
	// Forwarding is true when net.ipv6.conf.all.forwarding is 1.
	Forwarding bool
}

// ReadIPv6Host returns the IPv6 facts of the host.
// ReadIPv6Host returns an error when a route read or an address read fails. A failed read
// of force_forwarding states a kernel that does not hold it.
func (m *RealManager) ReadIPv6Host() (IPv6Host, error) {
	var h IPv6Host

	out, err := m.run("ip", "-6", "route", "show", "default")
	if err != nil {
		return h, fmt.Errorf("read the IPv6 default routes: %v (%s)", err, out)
	}
	h.Uplinks = parseRouteDevices(string(out))

	out, err = m.run("ip", "-6", "-o", "addr", "show", "scope", "global")
	if err != nil {
		return h, fmt.Errorf("read the global IPv6 addresses: %v (%s)", err, out)
	}
	h.Prefixes = parseHostPrefixes(string(out))

	_, err = m.run("sysctl", "-n", "net.ipv6.conf.all.force_forwarding")
	h.ForceForwarding = err == nil

	out, err = m.run("sysctl", "-n", "net.ipv6.conf.all.forwarding")
	if err != nil {
		return h, fmt.Errorf("read net.ipv6.conf.all.forwarding: %v (%s)", err, out)
	}
	h.Forwarding = strings.TrimSpace(string(out)) == "1"
	return h, nil
}

// parseRouteDevices returns each device that the output of `ip -6 route show default`
// names, once each and in the order of the output. A multipath route names one device on
// each nexthop line.
func parseRouteDevices(output string) []string {
	var devices []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "dev" && !slices.Contains(devices, fields[i+1]) {
				devices = append(devices, fields[i+1])
			}
		}
	}
	return devices
}

// parseHostPrefixes returns the prefix of each address in the output of
// `ip -6 -o addr show scope global`, once each. parseHostPrefixes skips each namespace
// device, because the compiler names those devices with an interface match.
func parseHostPrefixes(output string) []string {
	var prefixes []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[2] != "inet6" || strings.HasPrefix(fields[1], "vh") {
			continue
		}
		_, ipnet, err := net.ParseCIDR(fields[3])
		if err != nil {
			continue
		}
		if p := ipnet.String(); !slices.Contains(prefixes, p) {
			prefixes = append(prefixes, p)
		}
	}
	return prefixes
}

// sysctlDevice returns the device name in the form that a sysctl key holds. sysctl reads
// a dot as a separator, therefore a VLAN device eth0.2 is eth0/2 in the key.
func sysctlDevice(device string) string {
	return strings.ReplaceAll(device, ".", "/")
}

// EnableForceForwarding sets force_forwarding on each uplink device and returns the
// devices whose value it changed.
// A reply from the internet enters the host on the uplink, and the kernel forwards an IPv6
// packet only when the input device forwards. force_forwarding leaves the acceptance of a
// router advertisement on the device unchanged, which net.ipv6.conf.all.forwarding does
// not. The test host measured both on 2026-10-05.
func (m *RealManager) EnableForceForwarding(uplinks []string) ([]string, error) {
	var changed []string
	for _, dev := range uplinks {
		key := "net.ipv6.conf." + sysctlDevice(dev) + ".force_forwarding"
		out, err := m.run("sysctl", "-n", key)
		if err != nil {
			return changed, fmt.Errorf("read %s: %v (%s)", key, err, out)
		}
		if strings.TrimSpace(string(out)) == "1" {
			continue
		}
		if out, err := m.run("sysctl", "-w", key+"=1"); err != nil {
			return changed, fmt.Errorf("set %s: %v (%s)", key, err, out)
		}
		changed = append(changed, dev)
	}
	return changed, nil
}

// DisableForceForwarding resets force_forwarding on each uplink device. A step that fails
// does not stop the remaining steps; DisableForceForwarding returns the failures together.
func (m *RealManager) DisableForceForwarding(uplinks []string) error {
	var errs []error
	for _, dev := range uplinks {
		key := "net.ipv6.conf." + sysctlDevice(dev) + ".force_forwarding"
		if out, err := m.run("sysctl", "-w", key+"=0"); err != nil {
			errs = append(errs, fmt.Errorf("reset %s: %v (%s)", key, err, out))
		}
	}
	return errors.Join(errs...)
}

// EnableAllForwarding sets net.ipv6.conf.all.forwarding to 1, and it returns each device
// whose accept_ra value it changed from 1 to 2.
// A device with accept_ra 1 ignores a router advertisement while the host forwards, so the
// host loses its own IPv6 default route when the route expires. accept_ra 2 accepts the
// advertisement in both states. EnableAllForwarding changes accept_ra first, because the
// kernel removes the default routes of the advertisements when forwarding starts.
func (m *RealManager) EnableAllForwarding() ([]string, error) {
	out, err := m.run("ip", "-o", "link", "show")
	if err != nil {
		return nil, fmt.Errorf("list the devices of the host: %v (%s)", err, out)
	}

	var changed []string
	for _, dev := range parseLinkNames(string(out)) {
		key := "net.ipv6.conf." + sysctlDevice(dev) + ".accept_ra"
		value, err := m.run("sysctl", "-n", key)
		if err != nil || strings.TrimSpace(string(value)) != "1" {
			// A device that holds IPv6 off carries no accept_ra key.
			continue
		}
		if out, err := m.run("sysctl", "-w", key+"=2"); err != nil {
			return changed, fmt.Errorf("set %s: %v (%s)", key, err, out)
		}
		changed = append(changed, dev)
	}

	if out, err := m.run("sysctl", "-w", "net.ipv6.conf.all.forwarding=1"); err != nil {
		return changed, fmt.Errorf("set net.ipv6.conf.all.forwarding: %v (%s)", err, out)
	}
	return changed, nil
}

// parseLinkNames returns the device names in the output of `ip -o link show`. A device
// inside a namespace shows as name@ifN on the host, so parseLinkNames removes the suffix.
func parseLinkNames(output string) []string {
	var names []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimSuffix(fields[1], ":"), "@")
		names = append(names, name)
	}
	return names
}

// EnsureIPv6Path gives the namespace an IPv6 address, an IPv6 default route, and a NAT66
// rule, and it returns the NAT66 rule when it wrote it.
// nsName is the name of the namespace and index is its veth index. forceForwarding sets
// force_forwarding on the host side veth device, which a kernel without
// net.ipv6.conf.all.forwarding needs.
// Each address command and route command replaces, so a tick on an unchanged host changes
// nothing. EnsureIPv6Path returns the first failure.
func (m *RealManager) EnsureIPv6Path(nsName string, index int, forceForwarding bool) ([]string, error) {
	hostVeth, nsVeth := VethNames(nsName)
	hostIP, nsIP, hostGW, prefix := VethIPv6(index)

	steps := [][]string{
		{"ip", "-6", "addr", "replace", hostIP, "dev", hostVeth, "nodad"},
		{"ip", "netns", "exec", nsName, "ip", "-6", "addr", "replace", nsIP, "dev", nsVeth, "nodad"},
		{"ip", "netns", "exec", nsName, "ip", "-6", "route", "replace", "default", "via", hostGW, "dev", nsVeth},
	}
	if forceForwarding {
		steps = append(steps, []string{"sysctl", "-w", "net.ipv6.conf." + sysctlDevice(hostVeth) + ".force_forwarding=1"})
	}
	for _, step := range steps {
		if out, err := m.run(step[0], step[1:]...); err != nil {
			return nil, fmt.Errorf("IPv6 path of %s: %s: %v (%s)", nsName, strings.Join(step, " "), err, out)
		}
	}

	if _, err := m.run("ip6tables", "-t", "nat", "-C", "POSTROUTING", "-s", prefix, "-j", "MASQUERADE"); err == nil {
		return nil, nil
	}
	if out, err := m.run("ip6tables", "-t", "nat", "-A", "POSTROUTING", "-s", prefix, "-j", "MASQUERADE"); err != nil {
		return nil, fmt.Errorf("add the NAT66 rule of %s: %v (%s)", nsName, err, out)
	}
	return []string{"ip6 nat POSTROUTING -s " + prefix + " -j MASQUERADE"}, nil
}

// vethTeardownRulesIPv6 returns the ip6tables rules that TeardownVeth deletes for nsName.
func vethTeardownRulesIPv6(nsName string) [][]string {
	_, _, _, prefix := VethIPv6(VethIndex(nsName))
	return [][]string{
		{"-t", "nat", "-D", "POSTROUTING", "-s", prefix, "-j", "MASQUERADE"},
	}
}
