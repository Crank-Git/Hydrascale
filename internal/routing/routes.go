package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

// Manager defines the interface for route management operations.
type Manager interface {
	PollStatus(nsName, socketPath string) ([]Route, error)
	SyncRoutes(nsName string, routes []Route) error
	ListRoutes(nsName string) ([]string, error)
}

// RealManager implements Manager using real system calls.
type RealManager struct{}

// NewRealManager returns a new RealManager.
func NewRealManager() *RealManager {
	return &RealManager{}
}

func (m *RealManager) PollStatus(nsName, socketPath string) ([]Route, error) {
	return PollStatus(nsName, socketPath)
}

func (m *RealManager) SyncRoutes(nsName string, routes []Route) error {
	return SyncRoutes(nsName, routes)
}

func (m *RealManager) ListRoutes(nsName string) ([]string, error) {
	return ListRoutes(nsName)
}

// Route represents a Tailscale route.
type Route struct {
	Network                      string `json:"network,omitempty"`
	Natural                      bool   `json:"natural,omitempty"`
	AutonomousRouteAdvertisement bool   `json:"autonomousRouteAdvertisement,omitempty"`
}

// Status represents the Tailscale daemon status.
type Status struct {
	Routes []Route `json:"routes,omitempty"`
	Self   Self    `json:"self,omitempty"`
}

// Self represents the self information in Tailscale status.
type Self struct {
	Links []Link `json:"links,omitempty"`
}

// Link represents a network link in Tailscale status.
type Link struct {
	NetAddress []string `json:"netAddress,omitempty"`
}

// PollStatus polls the Tailscale daemon for routes in the given namespace.
// Uses tailscale status --json via ip netns exec with a 5s timeout.
func PollStatus(namespaceName string, socketPath string) ([]Route, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", namespaceName,
		"tailscale", "--socket="+socketPath, "status", "--json")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get tailscale status: %w", err)
	}

	var status Status
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status JSON: %w", err)
	}

	return status.Routes, nil
}

// parseCIDR parses a network string (like "192.168.1.0/24") into IP and mask.
func parseCIDR(network string) (net.IP, int, error) {
	if ip, ipnet, err := net.ParseCIDR(network); err == nil {
		ones, _ := ipnet.Mask.Size()
		return ip, ones, nil
	}
	return nil, 0, fmt.Errorf("invalid CIDR format: %s", network)
}

// ListRoutes returns the route destinations currently configured in the namespace.
// It parses the output of "ip netns exec <ns> ip route show".
func ListRoutes(nsName string) ([]string, error) {
	output, err := exec.Command("ip", "netns", "exec", nsName, "ip", "route", "show").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list routes in %s: %w", nsName, err)
	}
	return parseRouteOutput(string(output)), nil
}

// parseRouteOutput extracts destination CIDRs from ip route show output.
// Each line starts with the destination (e.g. "10.0.0.0/8 via 192.168.1.1 dev eth0").
// Lines starting with "default" are skipped since they aren't CIDR routes we manage.
func parseRouteOutput(output string) []string {
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
		// Skip default routes and non-CIDR entries
		if dest == "default" {
			continue
		}
		// Validate it looks like a CIDR
		if _, _, err := net.ParseCIDR(dest); err == nil {
			routes = append(routes, dest)
		}
	}
	return routes
}

// SyncRoutes synchronizes routes to the specified namespace using a diff-based approach.
// It compares desired routes against actual routes, adding missing and removing stale ones.
// Individual failures are collected and returned as a combined error.
func SyncRoutes(namespaceName string, routes []Route) error {
	actual, err := ListRoutes(namespaceName)
	if err != nil {
		return fmt.Errorf("failed to list current routes: %w", err)
	}

	// Build sets for diffing
	desiredSet := make(map[string]bool, len(routes))
	for _, r := range routes {
		desiredSet[r.Network] = true
	}
	actualSet := make(map[string]bool, len(actual))
	for _, a := range actual {
		actualSet[a] = true
	}

	var errs []error

	// Add missing routes (desired minus actual)
	for _, r := range routes {
		if !actualSet[r.Network] {
			if err := addRoute(namespaceName, r.Network); err != nil {
				errs = append(errs, fmt.Errorf("failed to add route %s: %w", r.Network, err))
			}
		}
	}

	// Remove stale routes (actual minus desired)
	for _, a := range actual {
		if !desiredSet[a] {
			if err := deleteRoute(namespaceName, a); err != nil {
				errs = append(errs, fmt.Errorf("failed to delete route %s: %w", a, err))
			}
		}
	}

	return errors.Join(errs...)
}

// addRoute adds a single route to the namespace using ip route add.
func addRoute(namespaceName, destination string) error {
	if _, _, err := parseCIDR(destination); err != nil {
		return fmt.Errorf("invalid route destination %s: %w", destination, err)
	}
	return exec.Command("ip", "netns", "exec", namespaceName, "route", "add", destination).Run()
}

// deleteRoute removes a single route from the namespace.
func deleteRoute(namespaceName, destination string) error {
	return exec.Command("ip", "netns", "exec", namespaceName, "route", "del", destination).Run()
}

// GetDefaultRoute extracts the default route from Tailscale status.
func GetDefaultRoute(routes []Route) string {
	for _, route := range routes {
		if route.Network == "0.0.0.0/0" || route.Network == "::/0" {
			return route.Network
		}
	}
	return ""
}

// HasExitNode checks if any routes indicate an exit node is configured.
// In Tailscale's status JSON, "natural" routes are the node's own addresses.
// Non-natural routes that aren't default routes (0.0.0.0/0 or ::/0) indicate
// advertised subnet routes, which implies an exit node or subnet router.
func HasExitNode(routes []Route) bool {
	for _, route := range routes {
		if !route.Natural && route.Network != "0.0.0.0/0" && route.Network != "::/0" {
			return true
		}
	}
	return false
}
