package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Settings is the user's persisted connection configuration.
type Settings struct {
	Mode         string `json:"mode"`         // "" (not connected) | "socket" | "mock"
	SocketPath   string `json:"socketPath"`   // used when Mode=="socket" and no SSH host
	SSHHost      string `json:"sshHost"`      // when set, the app forwards RemoteSocket over ssh
	RemoteSocket string `json:"remoteSocket"` // daemon socket path on the SSH host
	RefreshSec   int    `json:"refreshSec"`   // auto-refresh interval in seconds; 0 = off
}

func defaultSettings() Settings {
	return Settings{
		Mode:         "", // not connected by default — never demo data
		SocketPath:   "/var/lib/hydrascale/api.sock",
		RemoteSocket: "/var/lib/hydrascale/api.sock",
		RefreshSec:   30,
	}
}

// coerceMock enforces "mock is developer-only, never a default": mock is used
// solely when the flag is set; a persisted mode:"mock" without the flag (from an
// older build) is migrated to "" so returning users are never stuck on demo data.
func coerceMock(mode string, mockFlag bool) string {
	if mockFlag {
		return "mock"
	}
	if mode == "mock" {
		return ""
	}
	return mode
}

func settingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Hydrascale", "settings.json")
}

func loadSettings() Settings {
	s := defaultSettings()
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, &s)
	}
	// Mock (demo) data is opt-in for the developer only, never a default: it is
	// reachable solely via HYDRASCALE_MOCK=1. A persisted mode:"mock" from an
	// older build is migrated away unless the flag is set, so returning users are
	// never stuck on demo data.
	s.Mode = coerceMock(s.Mode, os.Getenv("HYDRASCALE_MOCK") == "1")
	// Dev/test override: HYDRASCALE_SOCKET forces socket mode at a path.
	if sock := os.Getenv("HYDRASCALE_SOCKET"); sock != "" {
		s.Mode = "socket"
		s.SocketPath = sock
		s.SSHHost = ""
	}
	return s
}

func saveSettings(s Settings) error {
	p := settingsPath()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0600)
}

// tunnel is an `ssh -N -L` local socket forward to a remote daemon socket.
type tunnel struct {
	cmd       *exec.Cmd
	localSock string
	dir       string // private 0700 temp dir holding localSock
}

func (t *tunnel) close() {
	if t == nil {
		return
	}
	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}
	if t.dir != "" {
		os.RemoveAll(t.dir)
	} else {
		os.Remove(t.localSock)
	}
}

// startTunnel forwards remoteSock on sshHost to a local unix socket and waits
// (briefly) for the forward to come up.
func startTunnel(sshHost, remoteSock string) (*tunnel, error) {
	// Guard against argv flag smuggling: sshHost/remoteSock come from user
	// settings (settings.json), and a value like "-oProxyCommand=..." would be
	// parsed by ssh as an option → arbitrary command execution. Reject flag-like
	// and whitespace-bearing input, and end ssh's option parsing with "--".
	if sshHost == "" || strings.HasPrefix(sshHost, "-") || strings.ContainsAny(sshHost, " \t\r\n") {
		return nil, fmt.Errorf("invalid ssh host")
	}
	if !strings.HasPrefix(remoteSock, "/") || strings.ContainsAny(remoteSock, " \t\r\n:") {
		return nil, fmt.Errorf("invalid remote socket path (must be an absolute path with no ':' )")
	}
	// Put the forwarded socket in a private, unpredictable 0700 dir rather than a
	// fixed name in shared /tmp — the latter is squattable/symlink-race prone on
	// Linux (a local attacker could pre-create the path and redirect the forward).
	dir, err := os.MkdirTemp("", "hydrascale-tunnel-")
	if err != nil {
		return nil, fmt.Errorf("create tunnel dir: %w", err)
	}
	local := filepath.Join(dir, "api.sock")
	cmd := exec.Command("ssh", "-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "StreamLocalBindUnlink=yes",
		"-o", "BatchMode=yes",
		"-L", local+":"+remoteSock, "--", sshHost)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("start ssh: %w", err)
	}
	// Poll for the forwarded socket to appear.
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(local); err == nil {
			return &tunnel{cmd: cmd, localSock: local, dir: dir}, nil
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("ssh exited before forward was ready (check host/keys)")
		}
		time.Sleep(100 * time.Millisecond)
	}
	cmd.Process.Kill()
	os.RemoveAll(dir)
	return nil, fmt.Errorf("timed out waiting for ssh forward to %s", sshHost)
}

// errSource is a DataSource that surfaces a connection error in the UI.
type errSource struct{ err error }

func (e errSource) Dashboard() (Dashboard, error)               { return Dashboard{}, e.err }
func (e errSource) TailnetDetail(string) (TailnetDetail, error) { return TailnetDetail{}, e.err }
func (e errSource) AddTailnet(AddTailnetRequest) error          { return e.err }
func (e errSource) RemoveTailnet(string) error                  { return e.err }
