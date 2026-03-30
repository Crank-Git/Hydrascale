package routing

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
)

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
func PollStatus(namespaceName string, timeout int) ([]Route, error) {
	cmd := exec.Command("ip", "netns", "exec", namespaceName, "tailscaled", "--status-json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get tailscaled status: %v (%s)", err, string(output))
	}

	var status Status
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status JSON: %v", err)
	}

	return status.Routes, nil
}

// parseCIDR parses a network string (like "192.168.1.0/24") into IP and mask.
// Returns the network IP and the prefix length.
func parseCIDR(network string) (net.IP, int, error) {
	if ip, ipnet, err := net.ParseCIDR(network); err == nil {
		ones, _ := ipnet.Mask.Size()
		return ip, ones, nil
	}
	return nil, 0, fmt.Errorf("invalid CIDR format: %s", network)
}

// SyncRoutes synchronizes routes to the specified namespace using netlink.
func SyncRoutes(namespaceName string, routes []Route) error {
	// Add each route
	for _, route := range routes {
		if err := addRoute(namespaceName, route.Network); err != nil {
			return fmt.Errorf("failed to add route %s: %w", route.Network, err)
		}
	}

	return nil
}

// addRoute adds a single route to the namespace using ip route add.
func addRoute(namespaceName, destination string) error {
	// Parse the destination to validate it's a proper CIDR
	if _, _, err := parseCIDR(destination); err != nil {
		return fmt.Errorf("invalid route destination %s: %w", destination, err)
	}

	// Add the route using ip route add
	return exec.Command("ip", "netns", "exec", namespaceName, "route", "add", destination).Run()
}

// deleteRoute removes a single route from the namespace using ip route del.
func deleteRoute(namespaceName, destination string) error {
	// Remove the route using ip route del
	return exec.Command("ip", "netns", "exec", namespaceName, "route", "del", destination).Run()
}

// GetDefaultRoute extracts the default route from Tailscale status.
// This is used for exit node handling.
func GetDefaultRoute(routes []Route) string {
	for _, route := range routes {
		if route.Network == "0.0.0.0/0" || route.Network == "::/0" {
			return route.Network
		}
	}
	return ""
}

// HasExitNode checks if any routes indicate an exit node is configured.
// In Tailscale, exit node routes have specific characteristics.
func HasExitNode(routes []Route) bool {
	// Simple heuristic: look for non-natural routes that aren't the default
	for _, route := range routes {
		if !route.Natural && route.Network != "0.0.0.0/0" && route.Network != "::/0" {
			return true
		}
	}
	return false
}
