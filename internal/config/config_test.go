package config

import (
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
