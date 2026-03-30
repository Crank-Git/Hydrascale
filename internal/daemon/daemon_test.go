package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestStartDaemon(t *testing.T) {
	t.Skip("Skipping daemon start test - requires privileges and tailscaled binary")
}

func TestStopDaemon(t *testing.T) {
	t.Skip("Skipping daemon stop test - requires privileges")
}

func TestCheckHealth(t *testing.T) {
	t.Skip("Skipping health check test - requires privileges and tailscaled")
}

func TestReadState(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "tailscaled-state-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	stateJSON := `{"version":1,"peerAPI":"http://127.0.0.1:41641"}`
	if err := os.WriteFile(tmpfile.Name(), []byte(stateJSON), 0644); err != nil {
		t.Fatalf("Failed to write temp state: %v", err)
	}

	state, err := readState(tmpfile.Name())
	if err != nil {
		t.Fatalf("readState: %v", err)
	}
	if state.Version != 1 {
		t.Errorf("Version = %d, want 1", state.Version)
	}
	if state.PeerAPI != "http://127.0.0.1:41641" {
		t.Errorf("PeerAPI = %q, want %q", state.PeerAPI, "http://127.0.0.1:41641")
	}
}

func TestReadState_InvalidJSON(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "tailscaled-state-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if err := os.WriteFile(tmpfile.Name(), []byte("{invalid"), 0644); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	_, err = readState(tmpfile.Name())
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadState_MissingFile(t *testing.T) {
	_, err := readState("/nonexistent/state.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidatePID(t *testing.T) {
	// Our own process should be findable in /proc
	pid := os.Getpid()
	// validatePID checks for "tailscaled" in cmdline, so our process won't match
	if validatePID(pid) {
		t.Error("validatePID should return false for non-tailscaled process")
	}

	// Non-existent PID
	if validatePID(999999999) {
		t.Error("validatePID should return false for non-existent PID")
	}
}

func TestSocketPath(t *testing.T) {
	got := SocketPath("team-prod")
	want := filepath.Join(DefaultStateDir, "team-prod", "tailscaled.sock")
	if got != want {
		t.Errorf("SocketPath = %q, want %q", got, want)
	}
}

func TestStopDaemon_StalePID(t *testing.T) {
	// Create a temp state dir with a stale PID file
	dir := t.TempDir()
	origStateDir := DefaultStateDir
	// We can't easily override DefaultStateDir in a test, so test validatePID directly
	_ = dir

	// Write a PID file pointing to a non-existent process
	pidDir := filepath.Join(dir, "stale-tailnet")
	os.MkdirAll(pidDir, 0755)
	pidPath := filepath.Join(pidDir, "tailscaled.pid")
	os.WriteFile(pidPath, []byte(strconv.Itoa(999999999)), 0644)

	_ = origStateDir
	// The actual StopDaemon uses DefaultStateDir which we can't override in unit tests
	// This validates the PID validation logic works correctly
	if validatePID(999999999) {
		t.Error("stale PID should not validate")
	}
}

func TestPollState(t *testing.T) {
	t.Skip("Skipping polling test - requires running daemon")
}
