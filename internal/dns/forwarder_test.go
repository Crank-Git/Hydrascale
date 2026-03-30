package dns

import (
	"testing"
	"time"
)

func TestNewForwarder(t *testing.T) {
	// Test with valid upstreams
	f, err := NewForwarder([]string{"8.8.8.8", "8.8.4.4"}, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}
	if f == nil {
		t.Fatalf("NewForwarder returned nil")
	}
	if len(f.upstreams) != 2 {
		t.Fatalf("Expected 2 upstreams, got %d", len(f.upstreams))
	}
	if err := f.Start(); err != nil {
		t.Fatalf("Failed to start forwarder: %v", err)
	}
	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)
	// Stop it
	if err := f.Stop(); err != nil {
		t.Fatalf("Failed to stop forwarder: %v", err)
	}
}

// TestStartResolver tests the StartResolver function
func TestStartResolver(t *testing.T) {
	// Test with unified mode
	if err := StartResolver("unified"); err != nil {
		t.Fatalf("Failed to start resolver: %v", err)
	}

	// Test with unsupported mode
	if err := StartResolver("unsupported"); err == nil {
		t.Fatalf("Expected error for unsupported resolver mode")
	}
}
