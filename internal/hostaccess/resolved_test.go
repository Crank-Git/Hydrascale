package hostaccess

import (
	"testing"
)

// TestResolvedManager_NilWhenUnavailable exercises RegisterDomains and handles
// gracefully when systemd-resolved is not available (CI/test environments).
func TestResolvedManager_NilWhenUnavailable(t *testing.T) {
	rm := NewResolvedManager()
	err := rm.RegisterDomains([]string{"example.com"})
	// Either succeeds (resolved is running) or returns a clear error — no panic.
	if err != nil {
		t.Logf("RegisterDomains returned expected error (resolved unavailable): %v", err)
	} else {
		t.Log("RegisterDomains succeeded (systemd-resolved is active)")
		// Clean up if it actually registered.
		rm.DeregisterAll()
	}
}

// TestResolvedManager_DeregisterIdempotent ensures DeregisterAll with nothing
// registered does not panic or error.
func TestResolvedManager_DeregisterIdempotent(t *testing.T) {
	rm := NewResolvedManager()
	rm.DeregisterAll() // should be a no-op
	rm.DeregisterAll() // calling twice must also be safe
}

// TestResolvedManager_EmptyDomains verifies that passing nil/empty domains is
// a no-op that returns no error.
func TestResolvedManager_EmptyDomains(t *testing.T) {
	rm := NewResolvedManager()
	if err := rm.RegisterDomains(nil); err != nil {
		t.Errorf("expected nil error for nil domains, got: %v", err)
	}
	if err := rm.RegisterDomains([]string{}); err != nil {
		t.Errorf("expected nil error for empty domains, got: %v", err)
	}
}
