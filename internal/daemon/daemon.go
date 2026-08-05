package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"hydrascale/internal/execx"
)

// DefaultStateDir is the base directory for per-tailnet state.
const DefaultStateDir = "/var/lib/hydrascale/state"

// TailscaleStatus represents parsed tailscale status --json output.
//
// BackendState holds the state word that tailscaled reports, which is "Running" for a node
// that is logged in and "NeedsLogin" for a node that is not. AuthURL holds the address
// that authorizes the node, and tailscaled fills it for "NeedsLogin" only. The console
// shows both, therefore the daemon reads both rather than inventing a state word.
type TailscaleStatus struct {
	Self           StatusNode            `json:"Self"`
	Peer           map[string]StatusNode `json:"Peer"`
	MagicDNSSuffix string                `json:"MagicDNSSuffix"`
	BackendState   string                `json:"BackendState"`
	AuthURL        string                `json:"AuthURL"`
}

// StatusNode represents a node in tailscale status.
type StatusNode struct {
	HostName     string    `json:"HostName"`
	DNSName      string    `json:"DNSName"`
	OS           string    `json:"OS"`
	TailscaleIPs []string  `json:"TailscaleIPs"`
	AllowedIPs   []string  `json:"AllowedIPs"`
	Online       bool      `json:"Online"`
	LastSeen     time.Time `json:"LastSeen"`
}

// UnprotectedRecord is what the __nsdaemon helper writes when the overlay mount on /etc
// fails. Reason holds the mount error text. Allowed is true when
// dns.allow_unprotected let the child start without the overlay mount.
type UnprotectedRecord struct {
	Reason  string `json:"reason"`
	Allowed bool   `json:"allowed"`
}

// UnprotectedFilePath returns the file in which the __nsdaemon helper of a tailnet
// records an overlay mount failure.
func UnprotectedFilePath(tailnetID string) string {
	return filepath.Join(DefaultStateDir, tailnetID, "dns-unprotected")
}

// ReadUnprotected returns the record at path. The second result is false when the file
// is absent, which means that the overlay mount holds.
func ReadUnprotected(path string) (UnprotectedRecord, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return UnprotectedRecord{}, false
	}
	var rec UnprotectedRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return UnprotectedRecord{Reason: fmt.Sprintf("unreadable DNS protection record: %v", err)}, true
	}
	return rec, true
}

// Manager defines the interface for daemon lifecycle operations.
type Manager interface {
	Start(tailnetID, nsName string, allowUnprotected bool) error
	UnprotectedDNS(tailnetID string) (UnprotectedRecord, bool)
	Stop(nsName, tailnetID string) error
	CheckHealth(nsName, tailnetID string) (bool, error)
	GetSocketPath(tailnetID string) string
	AuthorizeDaemon(tailnetID, nsName, authKey, controlURL string) error
	RefreshDNSConfig(tailnetID, nsName string) error
	GetStatus(ctx context.Context, nsName, tailnetID string) (*TailscaleStatus, error)
}

// RealManager implements Manager using real system calls.
//
// Runner runs every command that completes. Starter starts tailscaled, which outlives the
// call. A test replaces both with an execx.Recorder and asserts the exact argument list.
// StateDir names the base directory of the per-tailnet state, so a test writes into a
// temporary directory.
type RealManager struct {
	Runner   execx.Runner
	Starter  execx.Starter
	StateDir string
}

// NewRealManager returns a new RealManager that runs a command on the host.
func NewRealManager() *RealManager {
	return &RealManager{
		Runner:   execx.OSRunner{},
		Starter:  execx.OSStarter{},
		StateDir: DefaultStateDir,
	}
}

// runner returns the command runner. A RealManager with no Runner runs on the host.
func (m *RealManager) runner() execx.Runner {
	if m.Runner == nil {
		return execx.OSRunner{}
	}
	return m.Runner
}

// starter returns the child starter. A RealManager with no Starter starts on the host.
func (m *RealManager) starter() execx.Starter {
	if m.Starter == nil {
		return execx.OSStarter{}
	}
	return m.Starter
}

// stateDir returns the base directory of the per-tailnet state.
func (m *RealManager) stateDir() string {
	if m.StateDir == "" {
		return DefaultStateDir
	}
	return m.StateDir
}

// socketPath returns the tailscaled socket path of a tailnet.
func (m *RealManager) socketPath(tailnetID string) string {
	return filepath.Join(m.stateDir(), tailnetID, "tailscaled.sock")
}

// unprotectedFilePath returns the file in which the __nsdaemon helper of a tailnet
// records an overlay mount failure.
func (m *RealManager) unprotectedFilePath(tailnetID string) string {
	return filepath.Join(m.stateDir(), tailnetID, "dns-unprotected")
}

func (m *RealManager) UnprotectedDNS(tailnetID string) (UnprotectedRecord, bool) {
	return ReadUnprotected(m.unprotectedFilePath(tailnetID))
}

func (m *RealManager) GetSocketPath(tailnetID string) string {
	return m.socketPath(tailnetID)
}

// GetStatus returns parsed tailscale status for a tailnet.
// The provided context is used as the parent; a 5-second hard timeout is applied on top.
func GetStatus(ctx context.Context, namespaceName string, tailnetID string) (*TailscaleStatus, error) {
	return NewRealManager().GetStatus(ctx, namespaceName, tailnetID)
}

// GetStatus returns parsed tailscale status for a tailnet.
// The provided context is used as the parent; a 5-second hard timeout is applied on top.
func (m *RealManager) GetStatus(ctx context.Context, namespaceName string, tailnetID string) (*TailscaleStatus, error) {
	socketPath := m.socketPath(tailnetID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	output, err := m.runner().Run(ctx, "ip", "netns", "exec", namespaceName,
		"tailscale", "--socket="+socketPath, "status", "--json")
	if err != nil {
		return nil, fmt.Errorf("failed to get tailscale status for %s: %w", tailnetID, err)
	}

	var status TailscaleStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status JSON for %s: %w", tailnetID, err)
	}

	return &status, nil
}

// StartDaemon launches tailscaled inside a network namespace.
// It uses cmd.Start() to avoid blocking and writes the PID to a file.
func StartDaemon(tailnetID string, namespaceName string, allowUnprotected bool) error {
	return NewRealManager().Start(tailnetID, namespaceName, allowUnprotected)
}

// Start launches tailscaled inside a network namespace.
// Start returns before the child exits, and it writes the process identifier to a file.
func (m *RealManager) Start(tailnetID string, namespaceName string, allowUnprotected bool) error {
	stateDir := filepath.Join(m.stateDir(), tailnetID)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fmt.Errorf("failed to create state dir: %w", err)
	}

	socketPath := filepath.Join(stateDir, "tailscaled.sock")
	// Remove stale socket from previous run
	os.Remove(socketPath)
	stateFile := filepath.Join(stateDir, "tailscaled.state")

	// Launch tailscaled through the hydrascale __nsdaemon helper, which mounts a
	// per-namespace overlay on /etc first. tailscaled replaces /etc/resolv.conf
	// via rename; a single-file bind-mount can't contain that, so writes would
	// otherwise land in the host's shared /etc and clobber host DNS. The overlay
	// keeps every /etc write namespace-local. See issue #28.
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate hydrascale binary: %w", err)
	}
	etcUpper := filepath.Join(stateDir, "etc-upper")
	etcWork := filepath.Join(stateDir, "etc-work")
	// The helper writes this file when the overlay mount fails, so remove the record of
	// the previous launch. A record that stays behind reports a failure that is over.
	unprotectedFile := m.unprotectedFilePath(tailnetID)
	if err := os.Remove(unprotectedFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove the DNS protection record for %s: %w", tailnetID, err)
	}
	args := []string{
		"netns", "exec", namespaceName,
		self, "__nsdaemon",
		"--etc-upper", etcUpper,
		"--etc-work", etcWork,
		"--unprotected-file", unprotectedFile,
	}
	if allowUnprotected {
		args = append(args, "--allow-unprotected")
	}
	args = append(args,
		"--",
		"tailscaled",
		"--state="+stateFile,
		"--socket="+socketPath,
		"--statedir="+stateDir,
	)

	// Kill any existing daemon before starting a new one
	m.cleanupExistingDaemon(tailnetID)

	// Setpgid detaches the process group; Pdeathsig ensures tailscaled
	// is killed if hydrascale dies unexpectedly (e.g. SIGKILL).
	child, err := m.starter().Start(execx.Spec{
		Name: "ip",
		Args: args,
		Env:  minimalChildEnv(),
		SysProcAttr: &syscall.SysProcAttr{
			Setpgid:   true,
			Pdeathsig: syscall.SIGTERM,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to start tailscaled in namespace %q: %w", namespaceName, err)
	}

	// Write PID file
	pidPath := filepath.Join(stateDir, "tailscaled.pid")
	pid := child.Pid()
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0600); err != nil {
		// Kill the process if we can't track it
		child.Kill()
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Release the process so it doesn't become a zombie
	go child.Wait()

	log.Printf("tailscaled started in namespace %q (PID %d)", namespaceName, pid)
	return nil
}

// removePIDFile removes the PID file and returns the failure.
// removePIDFile returns nil when the file is already gone. A PID file that stays holds a
// number that the operating system can give to another process, and the next stop then
// sends SIGTERM to that process. See issue #69.
func removePIDFile(pidPath string) error {
	if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove the PID file %s: %w", pidPath, err)
	}
	return nil
}

// StopDaemon stops the tailscaled process for a specific tailnet.
// It reads the PID file, validates the process, sends SIGTERM, and waits.
// StopDaemon returns the failure of the PID file removal together with the failure of the
// step that ran before it.
func StopDaemon(namespaceName string, tailnetID string) error {
	return NewRealManager().Stop(namespaceName, tailnetID)
}

// Stop stops the tailscaled process of a tailnet.
// Stop returns the failure of the PID file removal together with the failure of the step
// that ran before it.
func (m *RealManager) Stop(namespaceName string, tailnetID string) error {
	stateDir := filepath.Join(m.stateDir(), tailnetID)
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

	switch readPIDState(pid) {
	case pidExited:
		// systemd sends SIGTERM to every process of the control group, so tailscaled
		// exits before this path runs. That is a stop, not a stale PID file.
		log.Printf("tailscaled for %s already stopped (PID %d exited)", tailnetID, pid)
		return removePIDFile(pidPath)
	case pidIsOtherProcess:
		return errors.Join(
			fmt.Errorf("stale PID %d for %s (process is not tailscaled)", pid, tailnetID),
			removePIDFile(pidPath))
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return errors.Join(
			fmt.Errorf("process %d not found for %s: %w", pid, tailnetID, err),
			removePIDFile(pidPath))
	}

	// Send SIGTERM for graceful shutdown
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return errors.Join(
			fmt.Errorf("failed to send SIGTERM to %d: %w", pid, err),
			removePIDFile(pidPath))
	}

	// Wait up to 5 seconds for graceful shutdown
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			// Force kill
			var errs []error
			if err := proc.Signal(syscall.SIGKILL); err != nil {
				errs = append(errs, fmt.Errorf("failed to send SIGKILL to %d: %w", pid, err))
			}
			errs = append(errs, removePIDFile(pidPath))
			log.Printf("tailscaled for %s force-killed (PID %d)", tailnetID, pid)
			return errors.Join(errs...)
		case <-tick.C:
			// Check if process is still running
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				// Process is gone
				log.Printf("tailscaled for %s stopped (PID %d)", tailnetID, pid)
				return removePIDFile(pidPath)
			}
		}
	}
}

// CheckHealth checks if the tailscaled daemon in a namespace is healthy.
// Returns true if the daemon responds to status queries within the timeout.
func CheckHealth(namespaceName string, tailnetID string) (bool, error) {
	return NewRealManager().CheckHealth(namespaceName, tailnetID)
}

// CheckHealth reports whether the tailscaled daemon in a namespace answers a status
// query within the deadline.
func (m *RealManager) CheckHealth(namespaceName string, tailnetID string) (bool, error) {
	socketPath := m.socketPath(tailnetID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := m.runner().Run(ctx, "ip", "netns", "exec", namespaceName,
		"tailscale", "--socket="+socketPath, "status", "--json")
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

// minimalChildEnv returns the environment of a child process of the daemon.
// The daemon holds an auth key in HYDRASCALE_AUTHKEY_<ID> for each tailnet that uses
// one. A child that inherits the complete environment carries the key of every tailnet
// into /proc/<pid>/environ. See SA-20.
func minimalChildEnv() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
	}
}

// buildTailscaleUpArgs constructs the argument list for `tailscale up`.
// When controlURL is non-empty, --login-server is appended (for Headscale).
func buildTailscaleUpArgs(socketPath, controlURL, authKeyFile string) []string {
	// --accept-dns=true is required so tailscaled enables its MagicDNS proxy
	// at 100.100.100.100:53 inside the namespace. The hostaccess forwarder
	// routes per-tailnet queries to that endpoint via DNAT on the veth.
	args := []string{"tailscale", "--socket=" + socketPath, "up", "--accept-dns=true"}
	// `tailscale up` reads the auth key from the --auth-key flag, NOT from the
	// TS_AUTHKEY environment variable (that is consumed by tailscaled, not the
	// CLI) — passing it via the env makes `up` fall back to interactive login
	// and time out. The file: form keeps the key out of argv (ps) and the
	// environment. See issue #31.
	if authKeyFile != "" {
		args = append(args, "--auth-key=file:"+authKeyFile)
	}
	if controlURL != "" {
		args = append(args, "--login-server="+controlURL)
	}
	return args
}

// AuthorizeDaemon waits for the tailscaled socket to become available,
// then runs tailscale up with the provided auth key.
func AuthorizeDaemon(tailnetID, nsName, authKey, controlURL string) error {
	return NewRealManager().AuthorizeDaemon(tailnetID, nsName, authKey, controlURL)
}

// AuthorizeDaemon waits for the tailscaled socket to become available,
// then runs tailscale up with the provided auth key.
func (m *RealManager) AuthorizeDaemon(tailnetID, nsName, authKey, controlURL string) error {
	socketPath := m.socketPath(tailnetID)

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

	// tailscale up takes the key from --auth-key, not TS_AUTHKEY. Stage it in a
	// 0600 temp file under the (root-only) state dir and pass --auth-key=file:<path>
	// so the secret never appears in argv or the environment. See issue #31.
	var authKeyFile string
	if authKey != "" {
		authKeyFile = filepath.Join(m.stateDir(), tailnetID, "authkey")
		f, err := os.OpenFile(authKeyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return fmt.Errorf("stage auth key for %s: %w", tailnetID, err)
		}
		defer os.Remove(authKeyFile)
		if _, err := f.WriteString(authKey); err != nil {
			f.Close()
			return fmt.Errorf("write auth key for %s: %w", tailnetID, err)
		}
		f.Close()
	}

	tsArgs := buildTailscaleUpArgs(socketPath, controlURL, authKeyFile)
	cmdArgs := append([]string{"netns", "exec", nsName}, tsArgs...)
	output, err := m.runner().Run(ctx, "ip", cmdArgs...)
	if err != nil {
		// Redact auth key from error output to prevent leaking it in logs
		sanitized := strings.ReplaceAll(string(output), authKey, "[REDACTED]")
		return fmt.Errorf("tailscale up failed for %s: %v (%s)", tailnetID, err, sanitized)
	}

	log.Printf("Authorized tailnet %s in namespace %s", tailnetID, nsName)
	return nil
}

// RefreshDNSConfig forces tailscaled to re-initialize its DNS configuration
// by toggling --accept-dns off and back on. This is required after a daemon
// restart from an existing state file: tailscaled caches its resolver chain
// in memory and does not re-read /etc/resolv.conf on startup. If the previous
// chain was wedged (e.g. only contained 100.100.100.100, which tailscaled
// filters as a self-loop), the MagicDNS proxy returns SERVFAIL for every
// query — including names it could answer locally from the peer table.
//
// The off→on transition causes tailscaled to rebuild the resolver chain from
// the current namespace resolv.conf, picking up real upstreams and unwedging
// the proxy. The toggle only produces a real teardown/rebuild once the
// backend is in the Running state; if we fire it earlier, the pref changes
// just become the initial prefs applied when the netmap eventually arrives,
// and no DNS rebuild happens. So poll for BackendState=Running first.
func RefreshDNSConfig(tailnetID, nsName string) error {
	return NewRealManager().RefreshDNSConfig(tailnetID, nsName)
}

// RefreshDNSConfig forces tailscaled to re-initialize its DNS configuration by toggling
// --accept-dns off and back on. See the note on the package function.
func (m *RealManager) RefreshDNSConfig(tailnetID, nsName string) error {
	socketPath := m.socketPath(tailnetID)

	// Phase 1: wait for socket to appear (StartDaemon is non-blocking).
	sockDeadline := time.After(30 * time.Second)
	sockTick := time.NewTicker(500 * time.Millisecond)
	defer sockTick.Stop()

	sockReady := false
	for !sockReady {
		select {
		case <-sockDeadline:
			return fmt.Errorf("timeout waiting for tailscaled socket for %s", tailnetID)
		case <-sockTick.C:
			if _, err := os.Stat(socketPath); err == nil {
				sockReady = true
			}
		}
	}

	// Phase 2: wait for BackendState=Running. Login + netmap typically takes
	// 2–10 seconds after socket creation; 60s gives slow networks headroom.
	runDeadline := time.After(60 * time.Second)
	runTick := time.NewTicker(500 * time.Millisecond)
	defer runTick.Stop()

	running := false
	for !running {
		select {
		case <-runDeadline:
			return fmt.Errorf("timeout waiting for tailscaled Running state for %s", tailnetID)
		case <-runTick.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			output, err := m.runner().Run(ctx, "ip", "netns", "exec", nsName,
				"tailscale", "--socket="+socketPath, "status", "--json")
			cancel()
			if err != nil {
				continue
			}
			var s struct {
				BackendState string `json:"BackendState"`
			}
			if json.Unmarshal(output, &s) != nil {
				continue
			}
			if s.BackendState == "Running" {
				running = true
			}
		}
	}

	// Phase 3: real flip-flop against a stable, connected daemon.
	for _, val := range []string{"false", "true"} {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		output, err := m.runner().Run(ctx, "ip", "netns", "exec", nsName,
			"tailscale", "--socket="+socketPath, "set", "--accept-dns="+val)
		cancel()
		if err != nil {
			return fmt.Errorf("tailscale set --accept-dns=%s failed for %s: %v (%s)",
				val, tailnetID, err, strings.TrimSpace(string(output)))
		}
	}

	log.Printf("Refreshed DNS config for tailnet %s (accept-dns flip-flop)", tailnetID)
	return nil
}

// SocketPath returns the tailscaled socket path for a given tailnet.
func SocketPath(tailnetID string) string {
	return filepath.Join(DefaultStateDir, tailnetID, "tailscaled.sock")
}

// cleanupExistingDaemon kills any running tailscaled for a tailnet before
// starting a new one. Prevents orphan accumulation on restarts.
func (m *RealManager) cleanupExistingDaemon(tailnetID string) {
	stateDir := filepath.Join(m.stateDir(), tailnetID)
	pidPath := filepath.Join(stateDir, "tailscaled.pid")

	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return // no PID file, nothing to clean up
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		os.Remove(pidPath)
		return
	}

	if !validatePID(pid) {
		os.Remove(pidPath)
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(pidPath)
		return
	}

	log.Printf("Cleaning up existing tailscaled for %s (PID %d)", tailnetID, pid)
	proc.Signal(syscall.SIGTERM)

	// Wait up to 3 seconds for it to die
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			proc.Signal(syscall.SIGKILL)
			os.Remove(pidPath)
			return
		case <-tick.C:
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				os.Remove(pidPath)
				return
			}
		}
	}
}

// pidState names what a PID identifies now.
type pidState int

const (
	// pidIsTailscaled means the PID identifies a tailscaled process.
	pidIsTailscaled pidState = iota
	// pidExited means the PID identifies no process.
	pidExited
	// pidIsOtherProcess means the PID identifies a process that is not tailscaled.
	pidIsOtherProcess
)

// readPIDState returns the state of a PID from /proc/<pid>/cmdline.
// A missing /proc entry is a process that exited, which is the normal result of a stop.
// Any other read failure counts as another process, because the caller must not send a
// signal to a PID that it cannot identify.
func readPIDState(pid int) pidState {
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		if os.IsNotExist(err) {
			return pidExited
		}
		return pidIsOtherProcess
	}
	if strings.Contains(string(data), "tailscaled") {
		return pidIsTailscaled
	}
	return pidIsOtherProcess
}

// validatePID reports whether a PID identifies a tailscaled process.
func validatePID(pid int) bool {
	return readPIDState(pid) == pidIsTailscaled
}
