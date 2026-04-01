package namespaces

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// Manager defines the interface for network namespace operations.
type Manager interface {
	Create(tailnetID string) error
	Delete(nsName string) error
	List() ([]string, error)
	GetName(tailnetID string) string
	GetTailnetID(nsName string) string
	SetupVeth(nsName string, index int) error
	TeardownVeth(nsName string) error
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
func (m *RealManager) Create(tailnetID string) error {
	return CreateNamespace(tailnetID)
}

// Delete deletes the network namespace with the given name.
func (m *RealManager) Delete(nsName string) error {
	return DeleteNamespace(nsName)
}

// SetupVeth creates a veth pair between host and namespace for DNS routing.
func (m *RealManager) SetupVeth(nsName string, index int) error {
	return SetupVeth(nsName, index)
}

// TeardownVeth removes the veth pair for a namespace.
func (m *RealManager) TeardownVeth(nsName string) error {
	return TeardownVeth(nsName)
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
func CreateNamespace(tailnetID string) error {
	namespaceName := GetNamespaceName(tailnetID)

	cmd := exec.Command("ip", "netns", "add", namespaceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create namespace %q: %v (%s)", namespaceName, err, output)
	}

	log.Printf("Created namespace: %s", namespaceName)

	// Set up veth pair for DNS routing
	index := VethIndex(namespaceName)
	if err := SetupVeth(namespaceName, index); err != nil {
		// Best effort cleanup: delete the namespace if veth setup fails
		_ = exec.Command("ip", "netns", "del", namespaceName).Run()
		return fmt.Errorf("failed to setup veth for namespace %q: %v", namespaceName, err)
	}

	return nil
}

// DeleteNamespace deletes the network namespace with the given name.
// Tears down the veth pair before deleting the namespace.
func DeleteNamespace(namespaceName string) error {
	// Tear down veth pair first (best effort - namespace deletion will clean up anyway)
	if err := TeardownVeth(namespaceName); err != nil {
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

// VethIndex returns a deterministic index (1-254) for a namespace name,
// used to pick the /30 subnet: 10.200.{index}.0/30.
func VethIndex(nsName string) int {
	h := sha256.Sum256([]byte(nsName))
	v := int(binary.BigEndian.Uint32(h[:4]))
	// Map to 1-254 range (avoid 0 and 255)
	return (v%254) + 1
}

// SetupVeth creates a veth pair between host and namespace for DNS routing.
// Host side: veth-<nsName> with IP 10.200.N.1/30
// Namespace side: veth-<nsName>-ns with IP 10.200.N.2/30
// Adds host route: 100.100.100.100 via 10.200.N.2 dev veth-<nsName>
func SetupVeth(nsName string, index int) error {
	hostVeth := fmt.Sprintf("veth-%s", nsName)
	nsVeth := fmt.Sprintf("veth-%s-ns", nsName)
	hostIP := fmt.Sprintf("10.200.%d.1/30", index)
	nsIP := fmt.Sprintf("10.200.%d.2/30", index)
	nsGW := fmt.Sprintf("10.200.%d.2", index)

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

	// Add host route: 100.100.100.100 via namespace side
	cmd = exec.Command("ip", "route", "add", "100.100.100.100", "via", nsGW, "dev", hostVeth)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add route for 100.100.100.100 via %s: %v (%s)", nsGW, err, out)
	}

	log.Printf("Set up veth pair for namespace %s (10.200.%d.0/30)", nsName, index)
	return nil
}

// TeardownVeth removes the veth pair for a namespace.
// Deleting the host side automatically removes the peer.
func TeardownVeth(nsName string) error {
	hostVeth := fmt.Sprintf("veth-%s", nsName)

	// Remove the host route first (best effort)
	index := VethIndex(nsName)
	nsGW := fmt.Sprintf("10.200.%d.2", index)
	delRoute := exec.Command("ip", "route", "del", "100.100.100.100", "via", nsGW, "dev", hostVeth)
	_ = delRoute.Run() // ignore error if route doesn't exist

	// Delete the veth pair (deleting one end removes both)
	cmd := exec.Command("ip", "link", "del", hostVeth)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete veth %s: %v (%s)", hostVeth, err, out)
	}

	log.Printf("Tore down veth pair for namespace %s", nsName)
	return nil
}
