package namespaces

import (
	"net"
	"strings"
	"testing"
)

// VethIPs returns the first two values with the prefix length, because `ip addr add`
// needs it, and the last two without it, because a caller that binds an address or names
// one to systemd-resolved needs the address alone.
//
// The DNS forwarder read the first value and tried to bind `10.200.1.45/30`, which is no
// address. This test holds the two forms apart.
func TestVethIPsReturnsThePrefixOnTheFirstTwoValuesOnly(t *testing.T) {
	hostIP, nsIP, hostGW, nsGW, err := VethIPs("10.200.0.0/16", 12)
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}

	for name, value := range map[string]string{"hostIP": hostIP, "nsIP": nsIP} {
		if !strings.Contains(value, "/") {
			t.Errorf("%s = %q, want a prefix length", name, value)
		}
	}
	for name, value := range map[string]string{"hostGW": hostGW, "nsGW": nsGW} {
		if net.ParseIP(value) == nil {
			t.Errorf("%s = %q, want an address that net.ParseIP reads", name, value)
		}
	}

	// The two forms name the same two addresses.
	if got, want := strings.TrimSuffix(hostIP, "/30"), hostGW; got != want {
		t.Errorf("hostIP without the prefix = %q, want %q", got, want)
	}
	if got, want := strings.TrimSuffix(nsIP, "/30"), nsGW; got != want {
		t.Errorf("nsIP without the prefix = %q, want %q", got, want)
	}
}
