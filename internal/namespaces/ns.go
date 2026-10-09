package namespaces

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"hydrascale/internal/access"
	"hydrascale/internal/execx"
)

// Manager defines the interface for network namespace operations.
type Manager interface {
	Create(tailnetID string, infraSubnet string) error
	Delete(nsName string, infraSubnet string) error
	List() ([]string, error)
	GetName(tailnetID string) string
	GetTailnetID(nsName string) string
	SetupVeth(nsName string, index int, infraSubnet string) error
	TeardownVeth(nsName string, infraSubnet string) error
}

// RealManager implements Manager using real system calls.
// Runner runs every command that RealManager sends to the host. A test replaces Runner
// with an execx.Recorder and asserts the exact argument list.
type RealManager struct {
	Runner execx.Runner
}

// NewRealManager returns a new RealManager that runs each command on the host.
func NewRealManager() *RealManager {
	return &RealManager{Runner: execx.OSRunner{}}
}

// runner returns the command runner. A RealManager with no Runner runs on the host.
func (m *RealManager) runner() execx.Runner {
	if m.Runner == nil {
		return execx.OSRunner{}
	}
	return m.Runner
}

// run executes name with args and returns the combined output.
// run returns an error when the command fails to start or exits non-zero.
func (m *RealManager) run(name string, args ...string) ([]byte, error) {
	return m.runner().Run(context.Background(), name, args...)
}

// ruleAbsent reports whether the output of an iptables delete states that the rule is not
// present. iptables exits non-zero for that state, and a teardown treats it as success,
// because an operator who already removed the rule reached the wanted result.
func ruleAbsent(output []byte) bool {
	text := string(output)
	return strings.Contains(text, "does a matching rule exist") ||
		strings.Contains(text, "No chain/target/match by that name")
}

// deleteRule runs one iptables delete and returns the failure.
// deleteRule returns nil when the command succeeds and when the rule is absent. Any other
// failure carries the command line and the output, because the operator needs the rule
// that stays on the host.
func (m *RealManager) deleteRule(name string, args ...string) error {
	out, err := m.run(name, args...)
	if err == nil || ruleAbsent(out) {
		return nil
	}
	return fmt.Errorf("%s %s: %v (%s)", name, strings.Join(args, " "), err, out)
}

// GetName returns the namespace name for a given tailnet ID.
func (m *RealManager) GetName(tailnetID string) string {
	return GetNamespaceName(tailnetID)
}

// GetTailnetID returns the tailnet ID from a namespace name.
func (m *RealManager) GetTailnetID(nsName string) string {
	return GetTailnetFromNamespace(nsName)
}

// GetNamespaceName returns the namespace name for a given tailnet ID.
// Format: ns-<tailnet-id> as per HYPERPLAN.md specification.
func GetNamespaceName(tailnetID string) string {
	return fmt.Sprintf("ns-%s", tailnetID)
}

// CreateNamespace creates a new network namespace for the given tailnet ID.
// After creating the namespace, it sets up a veth pair for DNS routing.
func CreateNamespace(tailnetID string, infraSubnet string) error {
	return NewRealManager().Create(tailnetID, infraSubnet)
}

// Create creates a new network namespace for the given tailnet ID.
// After creating the namespace, it sets up a veth pair for DNS routing.
func (m *RealManager) Create(tailnetID string, infraSubnet string) error {
	namespaceName := GetNamespaceName(tailnetID)

	output, err := m.run("ip", "netns", "add", namespaceName)
	if err != nil {
		return fmt.Errorf("failed to create namespace %q: %v (%s)", namespaceName, err, output)
	}

	log.Printf("Created namespace: %s", namespaceName)

	// Set up veth pair for DNS routing
	index := VethIndex(namespaceName)
	if err := m.SetupVeth(namespaceName, index, infraSubnet); err != nil {
		// Best effort cleanup: delete the namespace if veth setup fails
		_, _ = m.run("ip", "netns", "del", namespaceName)
		return fmt.Errorf("failed to setup veth for namespace %q: %v", namespaceName, err)
	}

	return nil
}

// DeleteNamespace deletes the network namespace with the given name.
// Tears down the veth pair before deleting the namespace.
func DeleteNamespace(namespaceName string, infraSubnet string) error {
	return NewRealManager().Delete(namespaceName, infraSubnet)
}

// Delete deletes the network namespace with the given name.
// Delete tears down the veth pair before it deletes the namespace.
// A step that fails does not stop the remaining steps. Delete collects every failure and
// returns the failures together.
func (m *RealManager) Delete(namespaceName string, infraSubnet string) error {
	var errs []error

	if err := m.TeardownVeth(namespaceName, infraSubnet); err != nil {
		errs = append(errs, fmt.Errorf("tear down the veth pair of %s: %w", namespaceName, err))
	}

	if output, err := m.run("ip", "netns", "del", namespaceName); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete namespace %q: %s", namespaceName, output))
	}

	// Clean up /etc/netns/<ns>/ (resolv.conf override and the directory itself).
	netnsDir := filepath.Join("/etc/netns", namespaceName)
	if err := removeIfPresent(filepath.Join(netnsDir, "resolv.conf")); err != nil {
		errs = append(errs, err)
	}
	if err := removeIfPresent(netnsDir); err != nil {
		errs = append(errs, err)
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	log.Printf("Deleted namespace: %s", namespaceName)
	return nil
}

// removeIfPresent removes path and returns the failure.
// removeIfPresent returns nil when the path does not exist, because a teardown that runs
// twice reaches the wanted result on the second run.
func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// ListNamespaces returns a list of all network namespaces (excluding default).
func ListNamespaces() ([]string, error) {
	return NewRealManager().List()
}

// List returns a list of all network namespaces (excluding default).
func (m *RealManager) List() ([]string, error) {
	output, err := m.run("ip", "netns", "list")
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %v", err)
	}

	// ip netns list outputs "ns-foo (id: 0)" per line.
	// Take only the first field per line to avoid junk tokens.
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name := strings.Fields(line)[0]
		if name != "default" && len(name) > 0 {
			result = append(result, name)
		}
	}

	return result, nil
}

// GetTailnetFromNamespace returns the tailnet ID associated with a namespace name.
func GetTailnetFromNamespace(namespaceName string) string {
	if len(namespaceName) > 3 && namespaceName[:3] == "ns-" {
		return namespaceName[3:]
	}
	return ""
}

// VethIndex returns a deterministic index (1-254) for a namespace name.
// The index selects a /30 block within the configured infra subnet.
func VethIndex(nsName string) int {
	h := sha256.Sum256([]byte(nsName))
	v := int(binary.BigEndian.Uint32(h[:4]))
	// Map to 1-254 range (avoid 0 and 255)
	return (v % 254) + 1
}

// VethNames returns a pair of interface names (host, namespace) that fit
// within the Linux 15-character IFNAMSIZ limit.  Format: "vh<hex>" / "vn<hex>"
// where <hex> is derived from the namespace name.
func VethNames(nsName string) (host, ns string) {
	h := sha256.Sum256([]byte(nsName))
	tag := fmt.Sprintf("%x", h[:6]) // 12 hex chars → "vh" + 12 = 14, fits in 15
	return "vh" + tag, "vn" + tag
}

// VethIPs calculates the IPs for a veth pair given an infra subnet and an index.
func VethIPs(infraSubnet string, index int) (hostIP, nsIP, hostGW, nsGW string, err error) {
	_, ipnet, err := net.ParseCIDR(infraSubnet)
	if err != nil {
		return "", "", "", "", err
	}

	// Use the index to pick a /30 subnet within the infraSubnet
	// index is 1-based.
	// 10.200.0.0/16 -> 10.200.0.0/30 (idx 1), 10.200.0.4/30 (idx 2), etc.
	base := binary.BigEndian.Uint32(ipnet.IP.To4())
	offset := uint32(index-1) * 4

	uHostIP := base + offset + 1
	uNsIP := base + offset + 2

	ip1 := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip1, uHostIP)
	ip2 := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip2, uNsIP)

	if !ipnet.Contains(ip1) || !ipnet.Contains(ip2) {
		return "", "", "", "", fmt.Errorf("veth index %d out of range for subnet %s", index, infraSubnet)
	}

	return ip1.String() + "/30", ip2.String() + "/30", ip1.String(), ip2.String(), nil
}

// SetupVeth creates a veth pair between host and namespace for DNS routing.
// Host side: vh<hash> with IP from infraSubnet
// Namespace side: vn<hash> with IP from infraSubnet
// MagicDNS (100.100.100.100) is reached via the DNS forwarder, not a host route.
func SetupVeth(nsName string, index int, infraSubnet string) error {
	return NewRealManager().SetupVeth(nsName, index, infraSubnet)
}

// SetupVeth creates a veth pair between host and namespace for DNS routing.
// Host side: vh<hash> with IP from infraSubnet
// Namespace side: vn<hash> with IP from infraSubnet
// MagicDNS (100.100.100.100) is reached via the DNS forwarder, not a host route.
func (m *RealManager) SetupVeth(nsName string, index int, infraSubnet string) error {
	hostVeth, nsVeth := VethNames(nsName)
	hostIP, nsIP, hostGW, _, err := VethIPs(infraSubnet, index)
	if err != nil {
		return err
	}

	// Create veth pair
	if out, err := m.run("ip", "link", "add", hostVeth, "type", "veth", "peer", "name", nsVeth); err != nil {
		return fmt.Errorf("failed to create veth pair: %v (%s)", err, out)
	}

	// Move namespace side into the namespace
	if out, err := m.run("ip", "link", "set", nsVeth, "netns", nsName); err != nil {
		return fmt.Errorf("failed to move %s into namespace %s: %v (%s)", nsVeth, nsName, err, out)
	}

	// Assign IP to host side
	if out, err := m.run("ip", "addr", "add", hostIP, "dev", hostVeth); err != nil {
		return fmt.Errorf("failed to assign IP to %s: %v (%s)", hostVeth, err, out)
	}

	// Bring up host side
	if out, err := m.run("ip", "link", "set", hostVeth, "up"); err != nil {
		return fmt.Errorf("failed to bring up %s: %v (%s)", hostVeth, err, out)
	}

	// Assign IP to namespace side
	if out, err := m.run("ip", "netns", "exec", nsName, "ip", "addr", "add", nsIP, "dev", nsVeth); err != nil {
		return fmt.Errorf("failed to assign IP to %s in namespace: %v (%s)", nsVeth, err, out)
	}

	// Bring up namespace side
	if out, err := m.run("ip", "netns", "exec", nsName, "ip", "link", "set", nsVeth, "up"); err != nil {
		return fmt.Errorf("failed to bring up %s in namespace: %v (%s)", nsVeth, err, out)
	}

	// Add default route inside namespace so tailscaled can reach the internet
	if out, err := m.run("ip", "netns", "exec", nsName, "ip", "route", "add", "default", "via", hostGW, "dev", nsVeth); err != nil {
		return fmt.Errorf("failed to add default route in namespace %s: %v (%s)", nsName, err, out)
	}

	// Enable IP forwarding on host for this veth
	if out, err := m.run("sysctl", "-w", "net.ipv4.conf."+hostVeth+".forwarding=1"); err != nil {
		return fmt.Errorf("failed to enable forwarding on %s: %v (%s)", hostVeth, err, out)
	}

	// SetupVeth writes no FORWARD rule. Version 0.9 wrote `-i vh<hash> -j ACCEPT`, which
	// tested the input interface alone and therefore accepted a packet to every
	// destination. That rule is the finding SA-8 and the finding SA-9. internal/access now
	// owns every forward rule, in the chain HYDRASCALE-FWD.

	// The masquerade rule gives the namespace a path to the internet. The reconciler runs
	// EnsureForwardPath on every tick, so a namespace that loses the rule gets it back.
	if _, err := m.EnsureForwardPath(nsName, index, infraSubnet); err != nil {
		return err
	}

	log.Printf("Set up veth pair for namespace %s with IPs from %s", nsName, infraSubnet)
	return nil
}

// EnsureForwardPath writes each host rule of the forward path of the namespace that the
// host does not hold, and it returns the rules that it wrote.
// nsName is the name of the namespace, index is the veth index of the namespace, and
// infraSubnet is the subnet that holds every veth pair.
// The forward path of a namespace is the masquerade rule in the nat table. SetupVeth
// writes that rule only when it creates the veth pair, therefore an operator firewall that
// reloads removes the rule of a namespace that keeps running, and no setup path returns
// it. The reconciler runs EnsureForwardPath on every tick, which costs one command for
// each namespace.
// EnsureForwardPath returns an error when the write fails.
//
// The forward path also holds the listen forward rule, which sends inbound UDP for the
// listen port of the namespace to the namespace. See ListenPort.
func (m *RealManager) EnsureForwardPath(nsName string, index int, infraSubnet string) ([]string, error) {
	_, nsIP, _, nsAddr, err := VethIPs(infraSubnet, index)
	if err != nil {
		return nil, err
	}

	var written []string
	for _, rule := range [][]string{
		{"POSTROUTING", "-s", nsIP, "-j", "MASQUERADE"},
		listenForward(nsAddr, ListenPort(index)),
	} {
		wrote, err := m.ensureNATRule("iptables", nsName, rule)
		if err != nil {
			return written, err
		}
		if wrote {
			written = append(written, "nat "+strings.Join(rule, " "))
		}
	}
	return written, nil
}

// ListenPortBase is the port that the host tailscaled uses by default. The listen port of
// a namespace follows it, so that no namespace takes the port of the host tailscaled.
const ListenPortBase = 41641

// ListenPort returns the UDP port of the tailscaled of the namespace with the veth index.
// The host sends inbound UDP for that port to the namespace, so a peer reaches the
// namespace without a hole that its own packets opened. A namespace behind the NAT of the
// host lost a direct connection when the conntrack entry expired, which issue #404
// measured. The port follows from the veth index alone, as the veth addresses do, so a
// teardown needs no configuration file.
func ListenPort(index int) int {
	return ListenPortBase + index
}

// listenForward returns the nat rule, without its operation, that sends inbound UDP for
// port to addr. addr is the address of the namespace, and an IPv6 address carries
// brackets. The addrtype match limits the rule to a packet for the host itself, so a
// packet that the host forwards for a container keeps its destination.
func listenForward(addr string, port int) []string {
	p := strconv.Itoa(port)
	return []string{"PREROUTING", "!", "-i", "vh+", "-m", "addrtype", "--dst-type", "LOCAL",
		"-p", "udp", "--dport", p, "-j", "DNAT", "--to-destination", addr + ":" + p}
}

// ensureNATRule writes the rule into the nat table of tool when the table does not hold
// it, and it reports whether it wrote the rule. rule holds no operation. nsName names the
// namespace in an error.
func (m *RealManager) ensureNATRule(tool, nsName string, rule []string) (bool, error) {
	check := append([]string{"-t", "nat", "-C"}, rule...)
	if _, err := m.run(tool, check...); err == nil {
		return false, nil
	}
	add := append([]string{"-t", "nat", "-A"}, rule...)
	if out, err := m.run(tool, add...); err != nil {
		return false, fmt.Errorf("add the nat rule %s of %s: %v (%s)", strings.Join(rule, " "), nsName, err, out)
	}
	return true, nil
}

// TeardownVeth removes the veth pair for a namespace.
// Deleting the host side automatically removes the peer.
func TeardownVeth(nsName string, infraSubnet string) error {
	return NewRealManager().TeardownVeth(nsName, infraSubnet)
}

// TeardownVeth removes the veth pair for a namespace.
// Deleting the host side automatically removes the peer.
// A step that fails does not stop the remaining steps. TeardownVeth collects every failure
// and returns the failures together. A rule that survives its namespace is a path that the
// operator did not declare.
func (m *RealManager) TeardownVeth(nsName string, infraSubnet string) error {
	hostVeth, _ := VethNames(nsName)

	// PlanRemoval names the same rules, so the console dialog and this teardown never
	// disagree about what the removal deletes.
	errs := m.teardownVethRules(nsName, infraSubnet)

	// Delete the veth pair (deleting one end removes both)
	if out, err := m.run("ip", "link", "del", hostVeth); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete veth %s: %v (%s)", hostVeth, err, out))
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	log.Printf("Tore down veth pair for namespace %s", nsName)
	return nil
}

// SetupHostAccess adds namespace-side iptables rules for host access:
// - Masquerade on tailscale0 so host traffic is forwarded to peers
// - DNS DNAT on veth so MagicDNS queries from host reach 100.100.100.100
// - DNAT on tailscale0 for each published port, see syncPublishedPorts
// - /etc/netns/NAME/resolv.conf for MagicDNS inside the namespace
// The sync adds each rule that is missing and deletes each stale published port rule.
func SetupHostAccess(nsName string, index int, infraSubnet string, publish []string, ipv6 bool) error {
	return NewRealManager().SetupHostAccess(nsName, index, infraSubnet, publish, ipv6)
}

// ErrPublishedPorts marks an error of SetupHostAccess that a published port rule caused.
// The other host access rules hold when a published port rule fails, so the caller records
// the error and continues.
var ErrPublishedPorts = errors.New("published ports")

// SetupHostAccess adds namespace-side iptables rules for host access:
// - net.ipv4.ip_forward=1 so the namespace forwards the packets of the host
// - Masquerade on tailscale0 so host traffic is forwarded to peers
// - DNS DNAT on veth so MagicDNS queries from host reach 100.100.100.100
// - DNAT on tailscale0 for each published port, see syncPublishedPorts
// - /etc/netns/NAME/resolv.conf for MagicDNS inside the namespace
// The sync adds each rule that is missing and deletes each stale published port rule.
//
// publish holds the tailnets[].publish entries of the tailnet. ipv6 is true when the
// namespace holds the IPv6 path of FR-access-29. SetupHostAccess writes the IPv6 published
// port rules only then.
//
// The caller reaches this function only for a tailnet whose host_access is true, therefore
// a tailnet that the operator keeps isolated holds net.ipv4.ip_forward=0.
//
// SetupHostAccess returns an error when the forwarding write fails. It returns an error
// that wraps ErrPublishedPorts when a published port rule fails.
func (m *RealManager) SetupHostAccess(nsName string, index int, infraSubnet string, publish []string, ipv6 bool) error {
	_, nsVeth := VethNames(nsName)
	_, nsIPRange, hostIP, _, err := VethIPs(infraSubnet, index)
	if err != nil {
		return err
	}

	// A packet of the host arrives on the veth device and leaves on tailscale0, which is a
	// forward step. A new namespace holds net.ipv4.ip_forward=0, so the kernel dropped that
	// packet before POSTROUTING and the masquerade rule below reached nothing. Host access
	// and the unified resolver both failed for that reason. See issue #272.
	// This step returns its error, because every rule below it reaches no packet without it.
	if out, err := m.run("ip", "netns", "exec", nsName, "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return fmt.Errorf("host-access: enable forwarding in %s: %v (%s)", nsName, err, out)
	}

	// Masquerade on tailscale0
	if _, err := m.run("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-C", "POSTROUTING", "-s", nsIPRange, "-o", "tailscale0", "-j", "MASQUERADE"); err != nil {
		if out, err := m.run("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-A", "POSTROUTING", "-s", nsIPRange, "-o", "tailscale0", "-j", "MASQUERADE"); err != nil {
			log.Printf("host-access: failed to add tailscale0 masquerade in %s: %v (%s)", nsName, err, out)
		}
	}

	// DNS DNAT UDP
	if _, err := m.run("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-C", "PREROUTING", "-i", nsVeth, "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53"); err != nil {
		if out, err := m.run("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-A", "PREROUTING", "-i", nsVeth, "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53"); err != nil {
			log.Printf("host-access: failed to add DNS DNAT (UDP) in %s: %v (%s)", nsName, err, out)
		}
	}

	// DNS DNAT TCP
	if _, err := m.run("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-C", "PREROUTING", "-i", nsVeth, "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53"); err != nil {
		if out, err := m.run("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-A", "PREROUTING", "-i", nsVeth, "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53"); err != nil {
			log.Printf("host-access: failed to add DNS DNAT (TCP) in %s: %v (%s)", nsName, err, out)
		}
	}

	families := []natFamily{{command: "iptables", host: hostIP}}
	if ipv6 {
		_, _, hostIPv6, _ := VethIPv6(index)
		families = append(families, natFamily{command: "ip6tables", host: "[" + hostIPv6 + "]"})
	}
	publishErr := m.syncPublishedPorts(nsName, publish, families)

	if err := WriteNamespaceResolvConf(nsName); err != nil {
		log.Printf("host-access: failed to write resolv.conf for %s: %v", nsName, err)
	}

	if publishErr != nil {
		return fmt.Errorf("%w in %s: %w", ErrPublishedPorts, nsName, publishErr)
	}
	log.Printf("Set up host access rules for namespace %s", nsName)
	return nil
}

// natFamily holds the command of one address family and the host side veth address in
// the form that --to-destination takes. An IPv6 address carries brackets, because a colon
// and the port follow it.
type natFamily struct {
	command string
	host    string
}

// syncPublishedPorts makes the DNAT rules on tailscale0 inside the namespace equal the
// publish list, in each address family of families.
// syncPublishedPorts adds each missing rule with -C and then -A, and it deletes each DNAT
// rule on tailscale0 that the list does not name. It runs every step and returns the
// failures together, so a failure in one family keeps the rules of the other family.
func (m *RealManager) syncPublishedPorts(nsName string, publish []string, families []natFamily) error {
	var errs []error
	var ports []access.PublishedPort
	for _, entry := range publish {
		p, err := access.ParsePublishPort(entry)
		if err != nil {
			errs = append(errs, fmt.Errorf("publish entry %q: %w", entry, err))
			continue
		}
		ports = append(ports, p)
	}

	for _, f := range families {
		present, err := m.publishedRules(nsName, f.command)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		want := make(map[string]bool, len(ports))
		for _, p := range ports {
			rule := publishedRule(p, f.host)
			want[strings.Join(rule, " ")] = true
			if _, err := m.run("ip", nsNatArgs(nsName, f.command, "-C", rule)...); err == nil {
				continue
			}
			args := nsNatArgs(nsName, f.command, "-A", rule)
			if out, err := m.run("ip", args...); err != nil {
				errs = append(errs, fmt.Errorf("ip %s: %v (%s)", strings.Join(args, " "), err, out))
			}
		}
		for _, rule := range present {
			if want[strings.Join(rule, " ")] {
				continue
			}
			if err := m.deleteRule("ip", nsNatArgs(nsName, f.command, "-D", rule)...); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// publishedRule returns the rule of one published port, from the chain name on, in the
// form that `iptables -S` prints. That form holds the -m match, so a rule that the chain
// holds compares equal to the rule that the list names.
// host is the destination address in the form of natFamily.host.
func publishedRule(p access.PublishedPort, host string) []string {
	port := strconv.Itoa(p.Number)
	return []string{"PREROUTING", "-i", "tailscale0", "-p", p.Protocol, "-m", p.Protocol, "--dport", port,
		"-j", "DNAT", "--to-destination", host + ":" + port}
}

// nsNatArgs returns the arguments of `ip` for one command on the nat table inside the
// namespace. op is the operation letter, and rule starts with the chain name.
func nsNatArgs(nsName, command, op string, rule []string) []string {
	return append([]string{"netns", "exec", nsName, command, "-t", "nat", op}, rule...)
}

// publishedRules returns each DNAT rule on tailscale0 in the nat PREROUTING chain of the
// namespace, from the chain name on.
// The DNS DNAT rules match the veth device, so the result holds none of them. command is
// iptables or ip6tables. publishedRules returns an error when the chain read fails.
func (m *RealManager) publishedRules(nsName, command string) ([][]string, error) {
	args := []string{"netns", "exec", nsName, command, "-t", "nat", "-S", "PREROUTING"}
	out, err := m.run("ip", args...)
	if err != nil {
		return nil, fmt.Errorf("ip %s: %v (%s)", strings.Join(args, " "), err, out)
	}

	var rules [][]string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "-A" || fields[1] != "PREROUTING" {
			continue
		}
		if hasPair(fields, "-i", "tailscale0") && hasPair(fields, "-j", "DNAT") {
			rules = append(rules, fields[1:])
		}
	}
	return rules, nil
}

// hasPair reports whether fields holds the option flag and value next to each other.
func hasPair(fields []string, flag, value string) bool {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == flag && fields[i+1] == value {
			return true
		}
	}
	return false
}

// WriteNamespaceResolvConf creates /etc/netns/<nsName>/resolv.conf so that
// `ip netns exec` bind-mounts it over /etc/resolv.conf for processes spawned
// in the namespace. Without this file, tailscaled --accept-dns=true rewrites
// the host's /etc/resolv.conf on startup (pointing it at 100.100.100.100,
// which only works inside the namespace), breaking host DNS — see issue #22.
//
// The contents are real, non-loopback host upstreams (resolveHostUpstreams).
// Tailscaled's DNS proxy needs at least one reachable upstream to answer any
// query, even names it can satisfy locally from its peer table; an empty or
// loopback-only chain produces SERVFAIL. 127.0.0.0/8 is filtered because
// loopback on the host is unreachable from inside the namespace. If no usable
// upstream is found, 1.1.1.1 is used as a last resort.
//
// Idempotent: safe to call repeatedly.
func WriteNamespaceResolvConf(nsName string) error {
	netnsDir := filepath.Join("/etc/netns", nsName)
	if err := os.MkdirAll(netnsDir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", netnsDir, err)
	}
	resolvPath := filepath.Join(netnsDir, "resolv.conf")
	upstreams := resolveHostUpstreams()
	var buf strings.Builder
	for _, ns := range upstreams {
		buf.WriteString("nameserver ")
		buf.WriteString(ns)
		buf.WriteString("\n")
	}
	if err := os.WriteFile(resolvPath, []byte(buf.String()), 0644); err != nil {
		return fmt.Errorf("write %s: %w", resolvPath, err)
	}
	return nil
}

// resolveHostUpstreams returns DNS upstreams reachable from inside a namespace.
// It prefers /run/systemd/resolve/resolv.conf (real upstreams, bypassing the
// systemd-resolved stub) and falls back to /etc/resolv.conf. Loopback addresses
// are filtered out because 127.0.0.x on the host is not reachable from inside
// the namespace. If no usable upstream is found, 1.1.1.1 is returned as a
// last-resort default.
func resolveHostUpstreams() []string {
	candidates := []string{
		"/run/systemd/resolve/resolv.conf",
		"/etc/resolv.conf",
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var found []string
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "nameserver") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			ip := net.ParseIP(fields[1])
			if ip == nil || ip.IsLoopback() {
				continue
			}
			found = append(found, ip.String())
		}
		if len(found) > 0 {
			return found
		}
	}
	return []string{"1.1.1.1"}
}

// TeardownHostAccess removes the three namespace-side host access iptables rules, and each
// published port rule in both address families.
// TeardownHostAccess returns the failed deletes together, and it treats a rule that is
// already absent as success.
//
// TeardownHostAccess keeps /etc/netns/<ns>/resolv.conf. Every namespace needs that file,
// host access does not create it, and Delete removes it with the namespace. See issue #22.
func TeardownHostAccess(nsName string, index int, infraSubnet string) error {
	return NewRealManager().TeardownHostAccess(nsName, index, infraSubnet)
}

// TeardownHostAccess removes the three namespace-side host access iptables rules, and each
// published port rule in both address families.
// TeardownHostAccess returns the failed deletes together, and it treats a rule that is
// already absent as success.
func (m *RealManager) TeardownHostAccess(nsName string, index int, infraSubnet string) error {
	_, nsVeth := VethNames(nsName)
	_, nsIPRange, _, _, err := VethIPs(infraSubnet, index)
	if err != nil {
		return fmt.Errorf("host-access: veth IPs for %s: %w", nsName, err)
	}

	// The operator set host_access to false, therefore the namespace forwards no packet of
	// the host again. A sysctl write is not a rule delete, so it does not run through
	// deleteRule, whose "rule does not exist" result carries no meaning here. See issue #272.
	var forwarding error
	if out, err := m.run("ip", "netns", "exec", nsName, "sysctl", "-w", "net.ipv4.ip_forward=0"); err != nil {
		forwarding = fmt.Errorf("host-access: disable forwarding in %s: %v (%s)", nsName, err, out)
	}

	errs := []error{
		forwarding,
		m.deleteRule("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-D", "POSTROUTING", "-s", nsIPRange, "-o", "tailscale0", "-j", "MASQUERADE"),
		m.deleteRule("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-D", "PREROUTING", "-i", nsVeth, "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53"),
		m.deleteRule("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-D", "PREROUTING", "-i", nsVeth, "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53"),
	}
	// The teardown reads both families, whatever the IPv6 state is now, because an IPv6
	// rule that an earlier sync wrote stays when the IPv6 path goes off.
	for _, command := range []string{"iptables", "ip6tables"} {
		rules, err := m.publishedRules(nsName, command)
		if err != nil {
			errs = append(errs, fmt.Errorf("host-access: %w", err))
			continue
		}
		for _, rule := range rules {
			errs = append(errs, m.deleteRule("ip", nsNatArgs(nsName, command, "-D", rule)...))
		}
	}
	return errors.Join(errs...)
}

// hostVethPattern matches a host-side veth device name that VethNames returns.
// ReapStaleRules removes a rule that names such a device only, so an operator rule on
// another interface keeps its place.
var hostVethPattern = regexp.MustCompile(`^vh[0-9a-f]{12}$`)

// ReapStaleRules removes each FORWARD rule that names a host veth device that is gone, and
// it returns the number of rules that it removed.
// ReapStaleRules returns the failed deletes together. The daemon runs it at start, because
// a rule that outlives its namespace lets traffic through a device name that a later
// namespace can take.
func (m *RealManager) ReapStaleRules() (int, error) {
	live, err := m.liveDevices()
	if err != nil {
		return 0, err
	}

	out, err := m.run("iptables", "-S", "FORWARD")
	if err != nil {
		return 0, fmt.Errorf("iptables -S FORWARD: %v (%s)", err, out)
	}

	var errs []error
	removed := 0
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "-A" || fields[1] != "FORWARD" {
			continue
		}
		if !namesAMissingDevice(fields, live) {
			continue
		}
		args := append([]string{"-D", "FORWARD"}, fields[2:]...)
		if err := m.deleteRule("iptables", args...); err != nil {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(errs...)
}

// legacyRuleForms holds the two FORWARD rules that version 0.9 wrote for each host veth
// device. The first element of each form is the position of the device name.
// RemoveLegacyForwardRules matches the whole form, so an operator rule that names the same
// device but carries another match keeps its place.
var legacyRuleForms = [][]string{
	{"-i", "", "-j", "ACCEPT"},
	{"-o", "", "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
}

// RemoveLegacyForwardRules removes each FORWARD rule that version 0.9 wrote for a host veth
// device, and it returns the number of rules that it removed.
// RemoveLegacyForwardRules returns the failed deletes together. The daemon runs it at
// start, because a host that upgrades keeps the rules of the version that it ran, and the
// rule `-i vh<hash> -j ACCEPT` is the finding SA-8 and the finding SA-9.
func (m *RealManager) RemoveLegacyForwardRules() (int, error) {
	out, err := m.run("iptables", "-S", "FORWARD")
	if err != nil {
		return 0, fmt.Errorf("iptables -S FORWARD: %v (%s)", err, out)
	}

	var errs []error
	removed := 0
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "-A" || fields[1] != "FORWARD" {
			continue
		}
		if !isLegacyForwardRule(fields[2:]) {
			continue
		}
		args := append([]string{"-D", "FORWARD"}, fields[2:]...)
		if err := m.deleteRule("iptables", args...); err != nil {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(errs...)
}

// isLegacyForwardRule reports whether the rule arguments equal one form that version 0.9
// wrote, with a host veth device name in the position of the device.
func isLegacyForwardRule(args []string) bool {
	for _, form := range legacyRuleForms {
		if len(args) != len(form) {
			continue
		}
		match := true
		for i := range form {
			if i == 1 {
				match = match && hostVethPattern.MatchString(args[i])
				continue
			}
			if args[i] != form[i] {
				match = false
			}
		}
		if match {
			return true
		}
	}
	return false
}

// NamespaceForwarding returns the value of net.ipv4.ip_forward inside the namespace.
// nsName is the name of the namespace.
// The finding SA-48 states that the containment of a packet inside the second namespace is
// a kernel default, not a rule. The daemon reads the value, so that a change of the default
// becomes an event that the operator sees.
// NamespaceForwarding returns an error when the command fails.
func (m *RealManager) NamespaceForwarding(nsName string) (string, error) {
	out, err := m.run("ip", "netns", "exec", nsName, "sysctl", "-n", "net.ipv4.ip_forward")
	if err != nil {
		return "", fmt.Errorf("read net.ipv4.ip_forward in %s: %v (%s)", nsName, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// namesAMissingDevice reports whether the rule matches an input interface or an output
// interface that is a host veth device that live does not hold.
func namesAMissingDevice(fields []string, live map[string]bool) bool {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] != "-i" && fields[i] != "-o" {
			continue
		}
		device := fields[i+1]
		if hostVethPattern.MatchString(device) && !live[device] {
			return true
		}
	}
	return false
}

// liveDevices returns the name of each network device that the host holds.
// A line of `ip -o link show` reads "7: vh0123@if6: <BROADCAST> mtu 1500 ...", so the
// device name is the second field without the trailing colon and without the peer suffix.
func (m *RealManager) liveDevices() (map[string]bool, error) {
	out, err := m.run("ip", "-o", "link", "show")
	if err != nil {
		return nil, fmt.Errorf("ip -o link show: %v (%s)", err, out)
	}

	live := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimSuffix(fields[1], ":")
		if at := strings.Index(name, "@"); at >= 0 {
			name = name[:at]
		}
		live[name] = true
	}
	return live, nil
}
