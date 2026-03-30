package namespaces

import (
	"fmt"
	"os/exec"
	"strings"
)

// GetNamespaceName returns the namespace name for a given tailnet ID.
// Format: ns-<tailnet-id> as per HYPERPLAN.md specification.
func GetNamespaceName(tailnetID string) string {
	return fmt.Sprintf("ns-%s", tailnetID)
}

// CreateNamespace creates a new network namespace for the given tailnet ID.
// It follows the naming convention: ns-<tailnet-id>.
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
