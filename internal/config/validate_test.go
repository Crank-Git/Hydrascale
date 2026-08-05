package config

import "testing"

func TestValidateControlURL_accepts_an_http_url_on_a_loopback_address(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:8080",
		"http://127.0.0.1",
		"http://[::1]:8080",
	} {
		if err := ValidateControlURL(raw); err != nil {
			t.Errorf("ValidateControlURL(%q) = %v, want nil", raw, err)
		}
	}
}

func TestValidateControlURL_rejects_an_http_url_on_a_remote_host(t *testing.T) {
	for _, raw := range []string{
		"http://example.com",
		"http://headscale.example.com:8080",
		"http://localhost:8080",
		"http://10.0.0.1:8080",
	} {
		if err := ValidateControlURL(raw); err == nil {
			t.Errorf("ValidateControlURL(%q) = nil, want an error", raw)
		}
	}
}

func TestValidateResolverMode_accepts_the_unified_mode_and_an_empty_value(t *testing.T) {
	for _, mode := range []string{"", "unified"} {
		if err := ValidateResolverMode(mode); err != nil {
			t.Errorf("ValidateResolverMode(%q) = %v, want nil", mode, err)
		}
	}
}

func TestValidateResolverMode_rejects_an_unknown_mode(t *testing.T) {
	for _, mode := range []string{"split", "Unified", "off"} {
		if err := ValidateResolverMode(mode); err == nil {
			t.Errorf("ValidateResolverMode(%q) = nil, want an error", mode)
		}
	}
}

func TestSafeStateDir_returns_the_directory_of_a_valid_tailnet_identifier(t *testing.T) {
	dir, ok := SafeStateDir("/var/lib/hydrascale/state", "alpha")
	if !ok {
		t.Fatal("SafeStateDir rejected the identifier alpha")
	}
	if dir != "/var/lib/hydrascale/state/alpha" {
		t.Errorf("dir = %q, want %q", dir, "/var/lib/hydrascale/state/alpha")
	}
}

func TestSafeStateDir_rejects_an_identifier_that_leaves_the_state_directory(t *testing.T) {
	for _, id := range []string{"../../tmp/x", "..", "a/b", "", "/etc"} {
		if _, ok := SafeStateDir("/var/lib/hydrascale/state", id); ok {
			t.Errorf("SafeStateDir accepted the identifier %q", id)
		}
	}
}
