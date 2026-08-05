package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestBuildTailscaleUpArgs(t *testing.T) {
	tests := []struct {
		name        string
		socketPath  string
		controlURL  string
		authKeyFile string
		wantArgs    []string
	}{
		{
			name:       "no key, no control URL (interactive/browser login)",
			socketPath: "/tmp/test.sock",
			controlURL: "",
			wantArgs:   []string{"tailscale", "--socket=/tmp/test.sock", "up", "--accept-dns=true"},
		},
		{
			name:        "with auth key file",
			socketPath:  "/tmp/test.sock",
			authKeyFile: "/var/lib/hydrascale/state/corp/authkey-123",
			wantArgs:    []string{"tailscale", "--socket=/tmp/test.sock", "up", "--accept-dns=true", "--auth-key=file:/var/lib/hydrascale/state/corp/authkey-123"},
		},
		{
			name:       "with control URL (Headscale)",
			socketPath: "/tmp/test.sock",
			controlURL: "https://headscale.example.com",
			wantArgs:   []string{"tailscale", "--socket=/tmp/test.sock", "up", "--accept-dns=true", "--login-server=https://headscale.example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTailscaleUpArgs(tt.socketPath, tt.controlURL, tt.authKeyFile)
			if len(got) != len(tt.wantArgs) {
				t.Fatalf("buildTailscaleUpArgs() = %v, want %v", got, tt.wantArgs)
			}
			for i := range got {
				if got[i] != tt.wantArgs[i] {
					t.Errorf("arg[%d] = %q, want %q", i, got[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestStatusNode_ParsesPeerFields(t *testing.T) {
	// A trimmed sample of `tailscale status --json`. The GUI peers table needs
	// OS, LastSeen and AllowedIPs parsed off each node, so guard the JSON tags.
	raw := `{
		"MagicDNSSuffix": "tail1234.ts.net",
		"Self": {"HostName": "myhost", "DNSName": "myhost.tail1234.ts.net.", "OS": "linux", "TailscaleIPs": ["100.64.1.5"], "Online": true},
		"Peer": {
			"nodekey:abc": {"HostName": "orin", "DNSName": "orin.tail1234.ts.net.", "OS": "linux", "TailscaleIPs": ["100.64.1.9"], "AllowedIPs": ["100.64.1.9/32", "192.168.1.0/24"], "Online": true, "LastSeen": "2026-07-01T12:00:00Z"}
		}
	}`
	var st TailscaleStatus
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if st.MagicDNSSuffix != "tail1234.ts.net" {
		t.Errorf("MagicDNSSuffix: got %q", st.MagicDNSSuffix)
	}
	p, ok := st.Peer["nodekey:abc"]
	if !ok {
		t.Fatal("peer nodekey:abc missing")
	}
	if p.OS != "linux" {
		t.Errorf("OS: got %q, want linux", p.OS)
	}
	if len(p.AllowedIPs) != 2 || p.AllowedIPs[1] != "192.168.1.0/24" {
		t.Errorf("AllowedIPs: got %v", p.AllowedIPs)
	}
	want := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	if !p.LastSeen.Equal(want) {
		t.Errorf("LastSeen: got %v, want %v", p.LastSeen, want)
	}
}

// unreachablePID is above the largest value that pid_max accepts on Linux, so
// /proc holds no entry for it. It stands for a process that exited.
const unreachablePID = 1 << 30

func TestStopReportsNoFailureWhenTheTailscaledProcessAlreadyExited(t *testing.T) {
	base := t.TempDir()
	pidPath := writePIDFile(t, base, "corp", unreachablePID)

	m := &RealManager{StateDir: base}
	if err := m.Stop("ns-corp", "corp"); err != nil {
		t.Errorf("Stop reported a failure for an exited process: %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("the PID file stayed behind at %s", pidPath)
	}
}

func TestStopReportsAStalePIDWhenThePIDNamesAnotherProcess(t *testing.T) {
	base := t.TempDir()
	// The test binary is not tailscaled, so its own PID names another process.
	pidPath := writePIDFile(t, base, "corp", os.Getpid())

	m := &RealManager{StateDir: base}
	err := m.Stop("ns-corp", "corp")
	if err == nil {
		t.Fatal("Stop reported no failure for a PID that names another process")
	}
	if !strings.Contains(err.Error(), "process is not tailscaled") {
		t.Errorf("Stop returned %q, want the stale PID message", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("the PID file stayed behind at %s", pidPath)
	}
}

// writePIDFile writes pid into the PID file of a tailnet under base, and returns the path.
func writePIDFile(t *testing.T, base, tailnetID string, pid int) string {
	t.Helper()

	stateDir := filepath.Join(base, tailnetID)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatalf("create the state directory: %v", err)
	}
	pidPath := filepath.Join(stateDir, "tailscaled.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0600); err != nil {
		t.Fatalf("write the PID file: %v", err)
	}
	return pidPath
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
