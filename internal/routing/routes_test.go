package routing

import (
	"net"
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
		{"192.168.1.0/33", nil, 0, true}, // invalid mask
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

func TestHasExitNode(t *testing.T) {
	// No exit node - all natural routes
	routes := []Route{
		{Network: "192.168.1.0/24", Natural: true},
		{Network: "0.0.0.0/0", Natural: true}, // default route
	}
	if got := HasExitNode(routes); got {
		t.Errorf("HasExitNode() = %v, want false", got)
	}

	// Has exit node - non-natural route that's not default
	routes = []Route{
		{Network: "192.168.1.0/24", Natural: true},
		{Network: "0.0.0.0/0", Natural: true},   // default route
		{Network: "10.0.0.0/8", Natural: false}, // exit node route
	}
	if got := HasExitNode(routes); !got {
		t.Errorf("HasExitNode() = %v, want true", got)
	}
}
