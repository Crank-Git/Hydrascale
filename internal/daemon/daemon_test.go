package daemon

import (
	"os"
	"testing"
)

func TestStartDaemon(t *testing.T) {
	// Skip actual daemon start in unit tests as it requires privileges and tailscaled binary
	t.Skip("Skipping daemon start test - requires privileges and tailscaled binary")
}

func TestStopDaemon(t *testing.T) {
	// Skip actual daemon stop in unit tests
	t.Skip("Skipping daemon stop test - requires privileges")
}

func TestReadState(t *testing.T) {
	// Create a temporary state file
	tmpfile, err := os.CreateTemp("", "tailscaled-state-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	// Write test state
	stateJSON := `{"version":1,"peerAPI":"http://127.0.0.1:41641"}`
	if err := os.WriteFile(tmpfile.Name(), []byte(stateJSON), 0644); err != nil {
		t.Fatalf("Failed to write temp state: %v", err)
	}

	// Read the state
	state, err := readState(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to read state: %v", err)
	}

	// Verify the state
	if state == nil {
		t.Fatalf("readState() returned nil")
	}
	if state.Version != 1 {
		t.Fatalf("Expected version 1, got %d", state.Version)
	}
	if state.PeerAPI != "http://127.0.0.1:41641" {
		t.Fatalf("Expected peerAPI 'http://127.0.0.1:41641', got '%s'", state.PeerAPI)
	}
}

func TestPollState(t *testing.T) {
	// Skip actual polling test as it requires a running daemon
	t.Skip("Skipping polling test - requires running daemon")
}
