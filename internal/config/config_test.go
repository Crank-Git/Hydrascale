package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_V2(t *testing.T) {
	tmp := writeTemp(t, `
version: 2
tailnets:
  - id: "test-tailnet"
    exit_node: "exit.example.com"
    auth_key: "tskey-auth-xxx"
resolver:
  mode: "unified"
  bind_address: "127.0.0.53:5354"
reconciler:
  interval: "15s"
`)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Version != 2 {
		t.Errorf("Version = %d, want 2", cfg.Version)
	}
	if len(cfg.Tailnets) != 1 {
		t.Fatalf("len(Tailnets) = %d, want 1", len(cfg.Tailnets))
	}
	if cfg.Tailnets[0].ID != "test-tailnet" {
		t.Errorf("Tailnet ID = %q, want %q", cfg.Tailnets[0].ID, "test-tailnet")
	}
	if cfg.Tailnets[0].ExitNode != "exit.example.com" {
		t.Errorf("ExitNode = %q, want %q", cfg.Tailnets[0].ExitNode, "exit.example.com")
	}
	if cfg.Tailnets[0].AuthKey != "tskey-auth-xxx" {
		t.Errorf("AuthKey = %q, want %q", cfg.Tailnets[0].AuthKey, "tskey-auth-xxx")
	}
	if cfg.Resolver.Mode != "unified" {
		t.Errorf("Resolver.Mode = %q, want %q", cfg.Resolver.Mode, "unified")
	}
	if cfg.Resolver.BindAddress != "127.0.0.53:5354" {
		t.Errorf("Resolver.BindAddress = %q, want %q", cfg.Resolver.BindAddress, "127.0.0.53:5354")
	}
	if cfg.Reconciler.Interval.Seconds() != 15 {
		t.Errorf("Reconciler.Interval = %v, want 15s", cfg.Reconciler.Interval)
	}
}

func TestLoadConfig_V1Migration(t *testing.T) {
	// v1 config has no version field
	tmp := writeTemp(t, `
tailnets:
  - id: "old-tailnet"
resolver:
  mode: "unified"
`)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Version != 2 {
		t.Errorf("Version = %d, want 2 (auto-migrated)", cfg.Version)
	}
	if cfg.Reconciler.Interval.Seconds() != 10 {
		t.Errorf("Reconciler.Interval = %v, want 10s (default)", cfg.Reconciler.Interval)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmp := writeTemp(t, `{{{invalid yaml`)
	_, err := LoadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadConfig_InvalidInterval(t *testing.T) {
	tmp := writeTemp(t, `
version: 2
tailnets: []
reconciler:
  interval: "not-a-duration"
`)
	_, err := LoadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for invalid interval")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
	if cfg.Version != 2 {
		t.Errorf("Version = %d, want 2", cfg.Version)
	}
	if len(cfg.Tailnets) != 0 {
		t.Errorf("len(Tailnets) = %d, want 0", len(cfg.Tailnets))
	}
	if cfg.Resolver.Mode != "unified" {
		t.Errorf("Resolver.Mode = %q, want %q", cfg.Resolver.Mode, "unified")
	}
	if cfg.Reconciler.Interval.Seconds() != 10 {
		t.Errorf("Reconciler.Interval = %v, want 10s", cfg.Reconciler.Interval)
	}
}

func TestSaveConfig_Atomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.Tailnets = append(cfg.Tailnets, Tailnet{ID: "saved-tailnet"})

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Read back
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if len(loaded.Tailnets) != 1 || loaded.Tailnets[0].ID != "saved-tailnet" {
		t.Errorf("Saved config mismatch: got %+v", loaded.Tailnets)
	}
	if loaded.Version != 2 {
		t.Errorf("Version = %d, want 2", loaded.Version)
	}
}

func TestSaveConfig_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.yaml")

	cfg := DefaultConfig()
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}
}

func TestSaveConfig_SetsVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{Tailnets: []Tailnet{}}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.Version != 2 {
		t.Errorf("Version = %d, want 2", loaded.Version)
	}
}

func TestLoadConfig_InvalidTailnetID(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"path traversal", "../../etc/passwd"},
		{"spaces", "has spaces"},
		{"empty", ""},
		{"slash", "foo/bar"},
		{"starts with dot", ".hidden"},
		{"starts with dash", "-bad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := fmt.Sprintf("version: 2\ntailnets:\n  - id: %q\n", tt.id)
			tmp := writeTemp(t, content)
			_, err := LoadConfig(tmp)
			if err == nil {
				t.Errorf("expected error for tailnet ID %q", tt.id)
			}
		})
	}
}

func TestLoadConfig_DuplicateTailnetID(t *testing.T) {
	tmp := writeTemp(t, `
version: 2
tailnets:
  - id: "dupe"
  - id: "dupe"
`)
	_, err := LoadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for duplicate tailnet ID")
	}
}

func TestLoadConfig_ValidTailnetIDs(t *testing.T) {
	ids := []string{"team-prod", "devops", "my.tailnet", "test_123", "a"}
	for _, id := range ids {
		content := fmt.Sprintf("version: 2\ntailnets:\n  - id: %q\n", id)
		tmp := writeTemp(t, content)
		_, err := LoadConfig(tmp)
		if err != nil {
			t.Errorf("valid ID %q rejected: %v", id, err)
		}
	}
}

func TestResolveAuthKey_EnvOverride(t *testing.T) {
	t.Setenv("HYDRASCALE_AUTHKEY_MY_TAILNET", "env-key-123")
	got := ResolveAuthKey("my-tailnet", "config-key-456")
	if got != "env-key-123" {
		t.Errorf("ResolveAuthKey = %q, want %q (env should override config)", got, "env-key-123")
	}
}

func TestResolveAuthKey_ConfigFallback(t *testing.T) {
	// Ensure no env var is set
	t.Setenv("HYDRASCALE_AUTHKEY_FALLBACK_NET", "")
	os.Unsetenv("HYDRASCALE_AUTHKEY_FALLBACK_NET")
	got := ResolveAuthKey("fallback-net", "config-key-789")
	if got != "config-key-789" {
		t.Errorf("ResolveAuthKey = %q, want %q (should fall back to config)", got, "config-key-789")
	}
}

func TestResolveAuthKey_Neither(t *testing.T) {
	os.Unsetenv("HYDRASCALE_AUTHKEY_EMPTY_NET")
	got := ResolveAuthKey("empty-net", "")
	if got != "" {
		t.Errorf("ResolveAuthKey = %q, want empty string", got)
	}
}

func TestLoadConfigHostAccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
version: 2
host_access: true
tailnets:
  - id: havoc
    host_access: true
  - id: personal
host_dns:
  mode: hosts
reconciler:
  interval: 10s
`
	os.WriteFile(path, []byte(content), 0644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.HostAccess {
		t.Error("expected global HostAccess to be true")
	}
	if cfg.HostDNS.Mode != "hosts" {
		t.Errorf("expected HostDNS.Mode=hosts, got %q", cfg.HostDNS.Mode)
	}
	if cfg.Tailnets[0].HostAccess == nil || !*cfg.Tailnets[0].HostAccess {
		t.Error("expected havoc HostAccess to be true")
	}
	if cfg.Tailnets[1].HostAccess != nil {
		t.Error("expected personal HostAccess to be nil (inherit global)")
	}
	// Test helper methods
	if !cfg.TailnetHostAccess("havoc") {
		t.Error("TailnetHostAccess should return true for havoc")
	}
	if !cfg.TailnetHostAccess("personal") {
		t.Error("TailnetHostAccess should return true for personal (inherits global)")
	}
	if cfg.EffectiveHostDNSMode() != "hosts" {
		t.Errorf("expected EffectiveHostDNSMode=hosts, got %q", cfg.EffectiveHostDNSMode())
	}
}

func TestLoadConfigHostAccessDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
version: 2
tailnets:
  - id: test
reconciler:
  interval: 10s
`
	os.WriteFile(path, []byte(content), 0644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HostAccess {
		t.Error("expected global HostAccess to default to false")
	}
	if cfg.HostDNS.Mode != "" {
		t.Errorf("expected empty HostDNS.Mode when not set, got %q", cfg.HostDNS.Mode)
	}
	if cfg.TailnetHostAccess("test") {
		t.Error("TailnetHostAccess should return false when global is false")
	}
	if cfg.EffectiveHostDNSMode() != "" {
		t.Errorf("expected empty EffectiveHostDNSMode when no host access, got %q", cfg.EffectiveHostDNSMode())
	}
}

// writeTemp creates a temp file with the given content and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "hydrascale-test-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}
