package namespaces

import (
	"fmt"
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
func CreateNamespace(tailnetID string) error {
	namespaceName := GetNamespaceName(tailnetID)

	cmd := exec.Command("ip", "netns", "add", namespaceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create namespace %q: %v (%s)", namespaceName, err, output)
	}

	fmt.Printf("Created namespace: %s\n", namespaceName)
	return nil
}

// DeleteNamespace deletes the network namespace with the given name.
func DeleteNamespace(namespaceName string) error {
	cmd := exec.Command("ip", "netns", "del", namespaceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete namespace %q: %s", namespaceName, output)
	}

	fmt.Printf("Deleted namespace: %s\n", namespaceName)
	return nil
}

// ListNamespaces returns a list of all network namespaces (excluding default).
func ListNamespaces() ([]string, error) {
	cmd := exec.Command("ip", "netns", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %v", err)
	}

	nameArray := strings.Fields(string(output))
	var result []string
	for _, ns := range nameArray {
		if ns != "default" && len(ns) > 0 {
			result = append(result, ns)
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
