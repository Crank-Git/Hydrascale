package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"hydrascale/internal/execx"
)

// startCall returns the one command that Start runs for the tailnet corp in the namespace
// ns-corp, with the state directory under base.
func startCall(t *testing.T, base string) execx.Call {
	t.Helper()

	stateDir := filepath.Join(base, "corp")
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	return execx.Call{Name: "ip", Args: []string{
		"netns", "exec", "ns-corp",
		self, "__nsdaemon",
		"--etc-upper", filepath.Join(stateDir, "etc-upper"),
		"--etc-work", filepath.Join(stateDir, "etc-work"),
		"--unprotected-file", filepath.Join(stateDir, "dns-unprotected"),
		"--",
		"tailscaled",
		"--state=" + filepath.Join(stateDir, "tailscaled.state"),
		"--socket=" + filepath.Join(stateDir, "tailscaled.sock"),
		"--statedir=" + stateDir,
	}}
}

func TestStartRunsTheFullCommandListInOrder(t *testing.T) {
	base := t.TempDir()
	stateDir := filepath.Join(base, "corp")
	want := startCall(t, base)

	rec := execx.NewRecorder(t)
	rec.ScriptStart(execx.StartResult{Pid: 4321}, want.Name, want.Args...)

	m := &RealManager{Runner: rec, Starter: rec, StateDir: base}
	if err := m.Start("corp", "ns-corp", false); err != nil {
		t.Fatalf("Start: %v", err)
	}

	got := rec.Calls()
	if len(got) != 1 {
		t.Fatalf("Start ran %d commands, want 1", len(got))
	}
	if got[0].String() != want.String() {
		t.Errorf("command = %q, want %q", got[0].String(), want.String())
	}

	pid, err := os.ReadFile(filepath.Join(stateDir, "tailscaled.pid"))
	if err != nil {
		t.Fatalf("read the PID file: %v", err)
	}
	if strings.TrimSpace(string(pid)) != "4321" {
		t.Errorf("the PID file holds %q, want %q", pid, "4321")
	}
}

func TestStartKeepsTheProcessAttributesOfTheChild(t *testing.T) {
	base := t.TempDir()
	t.Setenv("HYDRASCALE_AUTHKEY_CORP", "tskey-auth-kTESTSECRET123")
	want := startCall(t, base)

	rec := execx.NewRecorder(t)
	rec.ScriptStart(execx.StartResult{Pid: 4321}, want.Name, want.Args...)

	m := &RealManager{Runner: rec, Starter: rec, StateDir: base}
	if err := m.Start("corp", "ns-corp", false); err != nil {
		t.Fatalf("Start: %v", err)
	}

	specs := rec.Specs()
	if len(specs) != 1 {
		t.Fatalf("Start started %d children, want 1", len(specs))
	}
	spec := specs[0]

	if spec.SysProcAttr == nil {
		t.Fatal("Start set no process attributes, and Pdeathsig stops an orphaned tailscaled")
	}
	if !spec.SysProcAttr.Setpgid {
		t.Error("Start set no Setpgid on the child")
	}
	if spec.SysProcAttr.Pdeathsig != syscall.SIGTERM {
		t.Errorf("Pdeathsig = %v, want SIGTERM", spec.SysProcAttr.Pdeathsig)
	}
	for _, entry := range spec.Env {
		if strings.HasPrefix(entry, "HYDRASCALE_") {
			t.Errorf("the child environment carries %q", entry)
		}
		if strings.Contains(entry, "tskey-auth-kTESTSECRET123") {
			t.Errorf("the child environment carries an auth key value: %q", entry)
		}
	}
}

func TestAuthorizeDaemonKeepsTheAuthKeyOutOfTheArgumentList(t *testing.T) {
	const authKey = "tskey-auth-kTESTSECRET123"

	base := t.TempDir()
	stateDir := filepath.Join(base, "corp")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatalf("create the state directory: %v", err)
	}
	socketPath := filepath.Join(stateDir, "tailscaled.sock")
	if err := os.WriteFile(socketPath, nil, 0600); err != nil {
		t.Fatalf("create the socket file: %v", err)
	}
	authKeyPath := filepath.Join(stateDir, "authkey")

	want := execx.Call{Name: "ip", Args: []string{
		"netns", "exec", "ns-corp",
		"tailscale", "--socket=" + socketPath, "up", "--accept-dns=true",
		"--auth-key=file:" + authKeyPath,
	}}

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, want.Name, want.Args...)

	m := &RealManager{Runner: rec, Starter: rec, StateDir: base}
	if err := m.AuthorizeDaemon("corp", "ns-corp", authKey, ""); err != nil {
		t.Fatalf("AuthorizeDaemon: %v", err)
	}

	got := rec.Calls()
	if len(got) != 1 {
		t.Fatalf("AuthorizeDaemon ran %d commands, want 1:\n%v", len(got), got)
	}
	if got[0].String() != want.String() {
		t.Errorf("command = %q, want %q", got[0].String(), want.String())
	}
	for _, a := range got[0].Args {
		if strings.Contains(a, authKey) {
			t.Errorf("the argument list carries the auth key: %q", a)
		}
	}
	if _, err := os.Stat(authKeyPath); !os.IsNotExist(err) {
		t.Errorf("the auth key file stayed behind at %s", authKeyPath)
	}
}

func TestGetStatusRunsTheStatusCommandThroughTheRunner(t *testing.T) {
	base := t.TempDir()
	socketPath := filepath.Join(base, "corp", "tailscaled.sock")

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(`{"MagicDNSSuffix":"corp.ts.net"}`)},
		"ip", "netns", "exec", "ns-corp",
		"tailscale", "--socket="+socketPath, "status", "--json")

	m := &RealManager{Runner: rec, Starter: rec, StateDir: base}
	status, err := m.GetStatus(t.Context(), "ns-corp", "corp")
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.MagicDNSSuffix != "corp.ts.net" {
		t.Errorf("MagicDNSSuffix = %q, want %q", status.MagicDNSSuffix, "corp.ts.net")
	}
	if len(rec.Calls()) != 1 {
		t.Errorf("GetStatus ran %d commands, want 1", len(rec.Calls()))
	}
}

func TestARealManagerWithNoRunnerUsesTheOSRunner(t *testing.T) {
	m := &RealManager{}
	if _, ok := m.runner().(execx.OSRunner); !ok {
		t.Errorf("runner() = %T, want execx.OSRunner", m.runner())
	}
	if _, ok := m.starter().(execx.OSStarter); !ok {
		t.Errorf("starter() = %T, want execx.OSStarter", m.starter())
	}
	if m.stateDir() != DefaultStateDir {
		t.Errorf("stateDir() = %q, want %q", m.stateDir(), DefaultStateDir)
	}
}
