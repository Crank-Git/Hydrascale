package namespaces

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
type RealManager struct{}

// NewRealManager returns a new RealManager.
func NewRealManager() *RealManager {
	return &RealManager{}
}

// GetName returns the namespace name for a given tailnet ID.
func (m *RealManager) GetName(tailnetID string) string {
	return GetNamespaceName(tailnetID)
}

// GetTailnetID returns the tailnet ID from a namespace name.
func (m *RealManager) GetTailnetID(nsName string) string {
	return GetTailnetFromNamespace(nsName)
}

// Create creates a new network namespace for the given tailnet ID.
func (m *RealManager) Create(tailnetID string, infraSubnet string) error {
	return CreateNamespace(tailnetID, infraSubnet)
}

// Delete deletes the network namespace with the given name.
func (m *RealManager) Delete(nsName string, infraSubnet string) error {
	return DeleteNamespace(nsName, infraSubnet)
}

// SetupVeth creates a veth pair between host and namespace for DNS routing.
func (m *RealManager) SetupVeth(nsName string, index int, infraSubnet string) error {
	return SetupVeth(nsName, index, infraSubnet)
}

// TeardownVeth removes the veth pair for a namespace.
func (m *RealManager) TeardownVeth(nsName string, infraSubnet string) error {
	return TeardownVeth(nsName, infraSubnet)
}

// List returns a list of all Hydrascale network namespaces.
func (m *RealManager) List() ([]string, error) {
	return ListNamespaces()
}

// GetNamespaceName returns the namespace name for a given tailnet ID.
// Format: ns-<tailnet-id> as per HYPERPLAN.md specification.
func GetNamespaceName(tailnetID string) string {
	return fmt.Sprintf("ns-%s", tailnetID)
}

// CreateNamespace creates a new network namespace for the given tailnet ID.
// After creating the namespace, it sets up a veth pair for DNS routing.
func CreateNamespace(tailnetID string, infraSubnet string) error {
	namespaceName := GetNamespaceName(tailnetID)

	cmd := exec.Command("ip", "netns", "add", namespaceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create namespace %q: %v (%s)", namespaceName, err, output)
	}

	log.Printf("Created namespace: %s", namespaceName)

	// Set up veth pair for DNS routing
	index := VethIndex(namespaceName)
	if err := SetupVeth(namespaceName, index, infraSubnet); err != nil {
		// Best effort cleanup: delete the namespace if veth setup fails
		_ = exec.Command("ip", "netns", "del", namespaceName).Run()
		return fmt.Errorf("failed to setup veth for namespace %q: %v", namespaceName, err)
	}

	return nil
}

// DeleteNamespace deletes the network namespace with the given name.
// Tears down the veth pair before deleting the namespace.
func DeleteNamespace(namespaceName string, infraSubnet string) error {
	// Tear down veth pair first (best effort - namespace deletion will clean up anyway)
	if err := TeardownVeth(namespaceName, infraSubnet); err != nil {
		log.Printf("Warning: veth teardown for %s: %v", namespaceName, err)
	}

	cmd := exec.Command("ip", "netns", "del", namespaceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete namespace %q: %s", namespaceName, output)
	}

	log.Printf("Deleted namespace: %s", namespaceName)
	return nil
}

// ListNamespaces returns a list of all network namespaces (excluding default).
func ListNamespaces() ([]string, error) {
	cmd := exec.Command("ip", "netns", "list")
	output, err := cmd.Output()
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
	hostVeth, nsVeth := VethNames(nsName)
	hostIP, nsIP, hostGW, _, err := VethIPs(infraSubnet, index)
	if err != nil {
		return err
	}

	// Create veth pair
	cmd := exec.Command("ip", "link", "add", hostVeth, "type", "veth", "peer", "name", nsVeth)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create veth pair: %v (%s)", err, out)
	}

	// Move namespace side into the namespace
	cmd = exec.Command("ip", "link", "set", nsVeth, "netns", nsName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to move %s into namespace %s: %v (%s)", nsVeth, nsName, err, out)
	}

	// Assign IP to host side
	cmd = exec.Command("ip", "addr", "add", hostIP, "dev", hostVeth)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to assign IP to %s: %v (%s)", hostVeth, err, out)
	}

	// Bring up host side
	cmd = exec.Command("ip", "link", "set", hostVeth, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up %s: %v (%s)", hostVeth, err, out)
	}

	// Assign IP to namespace side
	cmd = exec.Command("ip", "netns", "exec", nsName, "ip", "addr", "add", nsIP, "dev", nsVeth)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to assign IP to %s in namespace: %v (%s)", nsVeth, err, out)
	}

	// Bring up namespace side
	cmd = exec.Command("ip", "netns", "exec", nsName, "ip", "link", "set", nsVeth, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up %s in namespace: %v (%s)", nsVeth, err, out)
	}

	// Add default route inside namespace so tailscaled can reach the internet
	cmd = exec.Command("ip", "netns", "exec", nsName, "ip", "route", "add", "default", "via", hostGW, "dev", nsVeth)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add default route in namespace %s: %v (%s)", nsName, err, out)
	}

	// Enable IP forwarding on host for this veth
	cmd = exec.Command("sysctl", "-w", "net.ipv4.conf."+hostVeth+".forwarding=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to enable forwarding on %s: %v (%s)", hostVeth, err, out)
	}

	// Add iptables FORWARD rules so namespace traffic isn't dropped (e.g. by Docker's DROP policy)
	// Use -C (check) before -I (insert) to avoid duplicates and errors on retry.
	if exec.Command("iptables", "-C", "FORWARD", "-i", hostVeth, "-j", "ACCEPT").Run() != nil {
		cmd = exec.Command("iptables", "-I", "FORWARD", "1", "-i", hostVeth, "-j", "ACCEPT")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add FORWARD rule for %s: %v (%s)", hostVeth, err, out)
		}
	}
	if exec.Command("iptables", "-C", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT").Run() != nil {
		cmd = exec.Command("iptables", "-I", "FORWARD", "1", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add FORWARD return rule for %s: %v (%s)", hostVeth, err, out)
		}
	}

	// Add iptables masquerade so namespace traffic can reach the internet
	if exec.Command("iptables", "-t", "nat", "-C", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE").Run() != nil {
		cmd = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add masquerade rule for %s: %v (%s)", nsIP, err, out)
		}
	}

	log.Printf("Set up veth pair for namespace %s with IPs from %s", nsName, infraSubnet)
	return nil
}

// TeardownVeth removes the veth pair for a namespace.
// Deleting the host side automatically removes the peer.
func TeardownVeth(nsName string, infraSubnet string) error {
	hostVeth, _ := VethNames(nsName)

	// Remove iptables rules (best effort)
	index := VethIndex(nsName)
	_, nsIP, _, _, err := VethIPs(infraSubnet, index)
	if err == nil {
		delFwd1 := exec.Command("iptables", "-D", "FORWARD", "-i", hostVeth, "-j", "ACCEPT")
		_ = delFwd1.Run()
		delFwd2 := exec.Command("iptables", "-D", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
		_ = delFwd2.Run()
		delNat := exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE")
		_ = delNat.Run()
	}

	// Delete the veth pair (deleting one end removes both)
	cmd := exec.Command("ip", "link", "del", hostVeth)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete veth %s: %v (%s)", hostVeth, err, out)
	}

	log.Printf("Tore down veth pair for namespace %s", nsName)
	return nil
}

// SetupHostAccess adds namespace-side iptables rules for host access:
// - Masquerade on tailscale0 so host traffic is forwarded to peers
// - DNS DNAT on veth so MagicDNS queries from host reach 100.100.100.100
// - /etc/netns/NAME/resolv.conf for MagicDNS inside the namespace
// All rules are idempotent (check before insert).
func SetupHostAccess(nsName string, index int, infraSubnet string) error {
	_, nsVeth := VethNames(nsName)
	_, nsIPRange, _, _, err := VethIPs(infraSubnet, index)
	if err != nil {
		return err
	}

	// Masquerade on tailscale0
	if exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-C", "POSTROUTING", "-s", nsIPRange, "-o", "tailscale0", "-j", "MASQUERADE").Run() != nil {
		cmd := exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-A", "POSTROUTING", "-s", nsIPRange, "-o", "tailscale0", "-j", "MASQUERADE")
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("host-access: failed to add tailscale0 masquerade in %s: %v (%s)", nsName, err, out)
		}
	}

	// DNS DNAT UDP
	if exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-C", "PREROUTING", "-i", nsVeth, "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53").Run() != nil {
		cmd := exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-A", "PREROUTING", "-i", nsVeth, "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53")
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("host-access: failed to add DNS DNAT (UDP) in %s: %v (%s)", nsName, err, out)
		}
	}

	// DNS DNAT TCP
	if exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-C", "PREROUTING", "-i", nsVeth, "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53").Run() != nil {
		cmd := exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-A", "PREROUTING", "-i", nsVeth, "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53")
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("host-access: failed to add DNS DNAT (TCP) in %s: %v (%s)", nsName, err, out)
		}
	}

	// /etc/netns/NAME/resolv.conf
	netnsDir := filepath.Join("/etc/netns", nsName)
	if err := os.MkdirAll(netnsDir, 0755); err != nil {
		log.Printf("host-access: failed to create %s: %v", netnsDir, err)
	} else {
		resolvPath := filepath.Join(netnsDir, "resolv.conf")
		if err := os.WriteFile(resolvPath, []byte("nameserver 100.100.100.100\n"), 0644); err != nil {
			log.Printf("host-access: failed to write %s: %v", resolvPath, err)
		}
	}

	log.Printf("Set up host access rules for namespace %s", nsName)
	return nil
}

// TeardownHostAccess removes namespace-side host access iptables rules and resolv.conf.
func TeardownHostAccess(nsName string, index int, infraSubnet string) {
	_, nsVeth := VethNames(nsName)
	_, nsIPRange, _, _, err := VethIPs(infraSubnet, index)
	if err != nil {
		log.Printf("host-access: failed to get veth IPs for teardown: %v", err)
		return
	}

	exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-D", "POSTROUTING", "-s", nsIPRange, "-o", "tailscale0", "-j", "MASQUERADE").Run()
	exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-D", "PREROUTING", "-i", nsVeth, "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53").Run()
	exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-D", "PREROUTING", "-i", nsVeth, "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53").Run()

	resolvPath := filepath.Join("/etc/netns", nsName, "resolv.conf")
	os.Remove(resolvPath)
	os.Remove(filepath.Join("/etc/netns", nsName))
}
