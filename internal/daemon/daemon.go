package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DefaultStateDir is the base directory for per-tailnet state.
const DefaultStateDir = "/var/lib/hydrascale/state"

// Manager defines the interface for daemon lifecycle operations.
type Manager interface {
	Start(tailnetID, nsName string) error
	Stop(nsName, tailnetID string) error
	CheckHealth(nsName, tailnetID string) (bool, error)
	GetSocketPath(tailnetID string) string
	AuthorizeDaemon(tailnetID, nsName, authKey string) error
}

// RealManager implements Manager using real system calls.
type RealManager struct{}

// NewRealManager returns a new RealManager.
func NewRealManager() *RealManager {
	return &RealManager{}
}

func (m *RealManager) Start(tailnetID, nsName string) error {
	return StartDaemon(tailnetID, nsName)
}

func (m *RealManager) Stop(nsName, tailnetID string) error {
	return StopDaemon(nsName, tailnetID)
}

func (m *RealManager) CheckHealth(nsName, tailnetID string) (bool, error) {
	return CheckHealth(nsName, tailnetID)
}

func (m *RealManager) GetSocketPath(tailnetID string) string {
	return SocketPath(tailnetID)
}

func (m *RealManager) AuthorizeDaemon(tailnetID, nsName, authKey string) error {
	return AuthorizeDaemon(tailnetID, nsName, authKey)
}

// StartDaemon launches tailscaled inside a network namespace.
// It uses cmd.Start() to avoid blocking and writes the PID to a file.
func StartDaemon(tailnetID string, namespaceName string) error {
	stateDir := filepath.Join(DefaultStateDir, tailnetID)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state dir: %w", err)
	}

	socketPath := filepath.Join(stateDir, "tailscaled.sock")
	stateFile := filepath.Join(stateDir, "tailscaled.state")

	args := []string{
		"netns", "exec", namespaceName,
		"tailscaled",
		"--state=" + stateFile,
		"--socket=" + socketPath,
		"--statedir=" + stateDir,
	}

	cmd := exec.Command("ip", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	// Detach from parent process group so tailscaled survives
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start tailscaled in namespace %q: %w", namespaceName, err)
	}

	// Write PID file
	pidPath := filepath.Join(stateDir, "tailscaled.pid")
	pid := cmd.Process.Pid
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0600); err != nil {
		// Kill the process if we can't track it
		cmd.Process.Kill()
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Release the process so it doesn't become a zombie
	go cmd.Wait()

	log.Printf("tailscaled started in namespace %q (PID %d)", namespaceName, pid)
	return nil
}

// StopDaemon stops the tailscaled process for a specific tailnet.
// It reads the PID file, validates the process, sends SIGTERM, and waits.
func StopDaemon(namespaceName string, tailnetID string) error {
	stateDir := filepath.Join(DefaultStateDir, tailnetID)
	pidPath := filepath.Join(stateDir, "tailscaled.pid")

	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No PID file means daemon is already stopped
			log.Printf("tailscaled for %s already stopped (no PID file)", tailnetID)
			return nil
		}
		return fmt.Errorf("failed to read PID file for %s: %w", tailnetID, err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return fmt.Errorf("invalid PID in file for %s: %w", tailnetID, err)
	}

	// Validate PID is actually tailscaled
	if !validatePID(pid) {
		os.Remove(pidPath)
		return fmt.Errorf("stale PID %d for %s (process is not tailscaled)", pid, tailnetID)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(pidPath)
		return fmt.Errorf("process %d not found for %s: %w", pid, tailnetID, err)
	}

	// Send SIGTERM for graceful shutdown
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		os.Remove(pidPath)
		return fmt.Errorf("failed to send SIGTERM to %d: %w", pid, err)
	}

	// Wait up to 5 seconds for graceful shutdown
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			// Force kill
			proc.Signal(syscall.SIGKILL)
			os.Remove(pidPath)
			log.Printf("tailscaled for %s force-killed (PID %d)", tailnetID, pid)
			return nil
		case <-tick.C:
			// Check if process is still running
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				// Process is gone
				os.Remove(pidPath)
				log.Printf("tailscaled for %s stopped (PID %d)", tailnetID, pid)
				return nil
			}
		}
	}
}

// CheckHealth checks if the tailscaled daemon in a namespace is healthy.
// Returns true if the daemon responds to status queries within the timeout.
func CheckHealth(namespaceName string, tailnetID string) (bool, error) {
	stateDir := filepath.Join(DefaultStateDir, tailnetID)
	socketPath := filepath.Join(stateDir, "tailscaled.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", namespaceName,
		"tailscale", "--socket="+socketPath, "status", "--json")

	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("health check failed for %s: %w", tailnetID, err)
	}

	// Verify we got valid JSON back
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return false, fmt.Errorf("invalid status JSON for %s: %w", tailnetID, err)
	}

	return true, nil
}

// AuthorizeDaemon waits for the tailscaled socket to become available,
// then runs tailscale up with the provided auth key.
func AuthorizeDaemon(tailnetID, nsName, authKey string) error {
	socketPath := SocketPath(tailnetID)

	// Poll for socket existence (up to 30s, 500ms intervals)
	deadline := time.After(30 * time.Second)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	socketReady := false
	for !socketReady {
		select {
		case <-deadline:
			return fmt.Errorf("timeout waiting for tailscaled socket for %s", tailnetID)
		case <-tick.C:
			if _, err := os.Stat(socketPath); err == nil {
				socketReady = true
			}
		}
	}

	// Run tailscale up with auth key
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", nsName,
		"tailscale", "--socket="+socketPath, "up", "--authkey="+authKey)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale up failed for %s: %v (%s)", tailnetID, err, output)
	}

	log.Printf("Authorized tailnet %s in namespace %s", tailnetID, nsName)
	return nil
}

// SocketPath returns the tailscaled socket path for a given tailnet.
func SocketPath(tailnetID string) string {
	return filepath.Join(DefaultStateDir, tailnetID, "tailscaled.sock")
}

// validatePID checks that a PID belongs to a tailscaled process
// by reading /proc/<pid>/cmdline.
func validatePID(pid int) bool {
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "tailscaled")
}

