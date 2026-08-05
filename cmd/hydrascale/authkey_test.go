package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNsTailscaleUpArgs_carries_the_file_path_and_not_the_key(t *testing.T) {
	args := nsTailscaleUpArgs("hydra-corp", "/var/lib/hydrascale/state/corp/tailscaled.sock",
		"/var/lib/hydrascale/state/corp/authkey-123")

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--auth-key=file:/var/lib/hydrascale/state/corp/authkey-123") {
		t.Errorf("the argument list holds no auth key file: %v", args)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--authkey=") {
			t.Errorf("the argument list holds the key in argv: %q", arg)
		}
	}
}

func TestStageAuthKey_writes_the_key_with_mode_0600(t *testing.T) {
	dir := t.TempDir()

	path, err := stageAuthKey(dir, "tskey-auth-kTESTSECRET123")
	if err != nil {
		t.Fatalf("stageAuthKey: %v", err)
	}
	defer os.Remove(path)

	if filepath.Dir(path) != dir {
		t.Errorf("stageAuthKey wrote to %q, want a file under %q", path, dir)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("the mode of the auth key file = %04o, want 0600", info.Mode().Perm())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "tskey-auth-kTESTSECRET123" {
		t.Errorf("the auth key file holds %q", body)
	}
}
