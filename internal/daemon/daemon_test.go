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

