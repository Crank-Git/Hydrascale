package routing

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestParseCIDR(t *testing.T) {
	tests := []struct {
		input    string
		wantIP   net.IP
		wantBits int
		wantErr  bool
	}{
		{"192.168.1.0/24", net.IPv4(192, 168, 1, 0), 24, false},
		{"10.0.0.0/8", net.IPv4(10, 0, 0, 0), 8, false},
		{"2001:db8::/32", net.ParseIP("2001:db8::"), 32, false},
		{"invalid", nil, 0, true},
		{"192.168.1.0/33", nil, 0, true},
	}

	for _, tt := range tests {
		gotIP, gotBits, err := parseCIDR(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseCIDR(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			if !gotIP.Equal(tt.wantIP) {
				t.Errorf("parseCIDR(%q) IP = %v, want %v", tt.input, gotIP, tt.wantIP)
			}
			if gotBits != tt.wantBits {
				t.Errorf("parseCIDR(%q) bits = %d, want %d", tt.input, gotBits, tt.wantBits)
			}
		}
	}
}

func TestGetDefaultRoute(t *testing.T) {
	routes := []Route{
		{Network: "192.168.1.0/24"},
		{Network: "0.0.0.0/0"},
		{Network: "10.0.0.0/8"},
	}
	if got := GetDefaultRoute(routes); got != "0.0.0.0/0" {
		t.Errorf("GetDefaultRoute() = %q, want %q", got, "0.0.0.0/0")
	}
}

func TestGetDefaultRoute_IPv6(t *testing.T) {
	routes := []Route{
		{Network: "::/0"},
	}
	if got := GetDefaultRoute(routes); got != "::/0" {
		t.Errorf("GetDefaultRoute() = %q, want %q", got, "::/0")
	}
}

func TestGetDefaultRoute_None(t *testing.T) {
	routes := []Route{
		{Network: "192.168.1.0/24"},
	}
	if got := GetDefaultRoute(routes); got != "" {
		t.Errorf("GetDefaultRoute() = %q, want empty", got)
	}
}

func TestHasExitNode(t *testing.T) {
	// No exit node - all natural routes
	routes := []Route{
		{Network: "192.168.1.0/24", Natural: true},
		{Network: "0.0.0.0/0", Natural: true},
	}
	if got := HasExitNode(routes); got {
		t.Errorf("HasExitNode() = %v, want false", got)
	}

	// Has exit node - non-natural route that's not default
	routes = []Route{
		{Network: "192.168.1.0/24", Natural: true},
		{Network: "0.0.0.0/0", Natural: true},
		{Network: "10.0.0.0/8", Natural: false},
	}
	if got := HasExitNode(routes); !got {
		t.Errorf("HasExitNode() = %v, want true", got)
	}
}

func TestHasExitNode_Empty(t *testing.T) {
	if HasExitNode(nil) {
		t.Error("HasExitNode(nil) = true, want false")
	}
}

// --- Mock-based route sync tests ---

// mockRoutingState tracks calls to add/delete/list route operations.
type mockRoutingState struct {
	// actual routes currently in the "namespace"
	actual []string
	// recorded calls
	added   []string
	deleted []string
	// inject errors: destination -> error
	addErrors    map[string]error
	deleteErrors map[string]error
}

func newMockRoutingState(actual ...string) *mockRoutingState {
	return &mockRoutingState{
		actual:       actual,
		addErrors:    make(map[string]error),
		deleteErrors: make(map[string]error),
	}
}

// syncRoutesWithMock performs the same diff logic as SyncRoutes but uses mock functions.
func syncRoutesWithMock(m *mockRoutingState, desired []Route) error {
	// Build sets for diffing
	desiredSet := make(map[string]bool, len(desired))
	for _, r := range desired {
		desiredSet[r.Network] = true
	}
	actualSet := make(map[string]bool, len(m.actual))
	for _, a := range m.actual {
		actualSet[a] = true
	}

	var errs []error

	// Add missing routes
	for _, r := range desired {
		if !actualSet[r.Network] {
			m.added = append(m.added, r.Network)
			if err, ok := m.addErrors[r.Network]; ok {
				errs = append(errs, fmt.Errorf("failed to add route %s: %w", r.Network, err))
			}
		}
	}

	// Remove stale routes
	for _, a := range m.actual {
		if !desiredSet[a] {
			m.deleted = append(m.deleted, a)
			if err, ok := m.deleteErrors[a]; ok {
				errs = append(errs, fmt.Errorf("failed to delete route %s: %w", a, err))
			}
		}
	}

	return errors.Join(errs...)
}

func TestParseRouteOutput(t *testing.T) {
	output := strings.Join([]string{
		"10.0.0.0/8 via 192.168.1.1 dev eth0",
		"172.16.0.0/12 dev tailscale0 scope link",
		"default via 192.168.1.1 dev eth0",
		"192.168.100.0/24 dev eth1 proto kernel scope link src 192.168.100.1",
	}, "\n")

	got := parseRouteOutput(output)
	want := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.100.0/24"}

	if len(got) != len(want) {
		t.Fatalf("parseRouteOutput() returned %d routes, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseRouteOutput()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestListRoutes_ParsesOutput(t *testing.T) {
	// Test the parseRouteOutput function which is the core of ListRoutes
	// (ListRoutes itself calls exec which needs root)
	output := "10.0.0.0/8 via 10.0.0.1 dev eth0\n172.16.0.0/12 dev tun0\ndefault via 10.0.0.1 dev eth0\n"
	got := parseRouteOutput(output)
	want := []string{"10.0.0.0/8", "172.16.0.0/12"}

	if !slices.Equal(got, want) {
		t.Errorf("parseRouteOutput() = %v, want %v", got, want)
	}
}

func TestSyncRoutes_AddsAndRemoves(t *testing.T) {
	// actual has B, C. desired has A, B. Should add A, remove C, keep B.
	m := newMockRoutingState("10.0.0.0/8", "192.168.1.0/24")
	desired := []Route{
		{Network: "172.16.0.0/12"},
		{Network: "10.0.0.0/8"},
	}

	err := syncRoutesWithMock(m, desired)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !slices.Equal(m.added, []string{"172.16.0.0/12"}) {
		t.Errorf("added = %v, want [172.16.0.0/12]", m.added)
	}
	if !slices.Equal(m.deleted, []string{"192.168.1.0/24"}) {
		t.Errorf("deleted = %v, want [192.168.1.0/24]", m.deleted)
	}
}

func TestSyncRoutes_NoChanges(t *testing.T) {
	m := newMockRoutingState("10.0.0.0/8", "172.16.0.0/12")
	desired := []Route{
		{Network: "10.0.0.0/8"},
		{Network: "172.16.0.0/12"},
	}

	err := syncRoutesWithMock(m, desired)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.added) != 0 {
		t.Errorf("added = %v, want empty", m.added)
	}
	if len(m.deleted) != 0 {
		t.Errorf("deleted = %v, want empty", m.deleted)
	}
}

func TestSyncRoutes_PartialFailure(t *testing.T) {
	// actual has C. desired has A, B. A fails to add. B should still be added.
	m := newMockRoutingState("192.168.1.0/24")
	m.addErrors["10.0.0.0/8"] = fmt.Errorf("command failed")

	desired := []Route{
		{Network: "10.0.0.0/8"},
		{Network: "172.16.0.0/12"},
	}

	err := syncRoutesWithMock(m, desired)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Both routes should have been attempted
	sort.Strings(m.added)
	wantAdded := []string{"10.0.0.0/8", "172.16.0.0/12"}
	if !slices.Equal(m.added, wantAdded) {
		t.Errorf("added = %v, want %v", m.added, wantAdded)
	}

	// The stale route should still be removed
	if !slices.Equal(m.deleted, []string{"192.168.1.0/24"}) {
		t.Errorf("deleted = %v, want [192.168.1.0/24]", m.deleted)
	}

	// Error message should mention the failed route
	if !strings.Contains(err.Error(), "10.0.0.0/8") {
		t.Errorf("error should mention failed route, got: %v", err)
	}
}

func TestSyncRoutes_EmptyDesired(t *testing.T) {
	// actual has routes, desired is empty -> remove all
	m := newMockRoutingState("10.0.0.0/8", "172.16.0.0/12")

	err := syncRoutesWithMock(m, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.added) != 0 {
		t.Errorf("added = %v, want empty", m.added)
	}
	sort.Strings(m.deleted)
	wantDeleted := []string{"10.0.0.0/8", "172.16.0.0/12"}
	if !slices.Equal(m.deleted, wantDeleted) {
		t.Errorf("deleted = %v, want %v", m.deleted, wantDeleted)
	}
}

func TestSyncRoutes_EmptyActual(t *testing.T) {
	// actual is empty, desired has routes -> add all
	m := newMockRoutingState()
	desired := []Route{
		{Network: "10.0.0.0/8"},
		{Network: "172.16.0.0/12"},
	}

	err := syncRoutesWithMock(m, desired)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantAdded := []string{"10.0.0.0/8", "172.16.0.0/12"}
	if !slices.Equal(m.added, wantAdded) {
		t.Errorf("added = %v, want %v", m.added, wantAdded)
	}
	if len(m.deleted) != 0 {
		t.Errorf("deleted = %v, want empty", m.deleted)
	}
}
