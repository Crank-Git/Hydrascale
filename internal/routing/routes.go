package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"time"
)

// Manager defines the interface for route management operations.
type Manager interface {
	PollStatus(nsName, socketPath string) ([]Route, error)
	SyncRoutes(nsName string, routes []Route) error
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

// SyncRoutes synchronizes routes to the specified namespace.
func SyncRoutes(namespaceName string, routes []Route) error {
	for _, route := range routes {
		if err := addRoute(namespaceName, route.Network); err != nil {
			return fmt.Errorf("failed to add route %s: %w", route.Network, err)
		}
	}
	return nil
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
func HasExitNode(routes []Route) bool {
	for _, route := range routes {
		if !route.Natural && route.Network != "0.0.0.0/0" && route.Network != "::/0" {
			return true
		}
	}
	return false
}
