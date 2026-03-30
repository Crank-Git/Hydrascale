package config

import (
	"os"
	"testing"
)

func TestLoadConfigFromFlag(t *testing.T) {
	// Create a temporary config file
	tmpfile, err := os.CreateTemp("", "hydrascale-test-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	// Write test config
	configYAML := `
tailnets:
  - id: "test-tailnet"
    exit_node: "exit.example.com"
resolver:
  mode: "unified"
`
	if err := os.WriteFile(tmpfile.Name(), []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}

	// Load the config
	cfg, err := LoadConfigFromFlag(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the config
	if len(cfg.Tailnets) != 1 {
		t.Fatalf("Expected 1 tailnet, got %d", len(cfg.Tailnets))
	}
	if cfg.Tailnets[0].ID != "test-tailnet" {
		t.Fatalf("Expected tailnet ID 'test-tailnet', got '%s'", cfg.Tailnets[0].ID)
	}
	if cfg.Tailnets[0].ExitNode != "exit.example.com" {
		t.Fatalf("Expected exit node 'exit.example.com', got '%s'", cfg.Tailnets[0].ExitNode)
	}
	if cfg.Resolver.Mode != "unified" {
		t.Fatalf("Expected resolver mode 'unified', got '%s'", cfg.Resolver.Mode)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatalf("DefaultConfig() returned nil")
	}
	if len(cfg.Tailnets) != 0 {
		t.Fatalf("Expected empty tailnets in default config, got %d", len(cfg.Tailnets))
	}
	if cfg.Resolver.Mode != "unified" {
		t.Fatalf("Expected resolver mode 'unified' in default config, got '%s'", cfg.Resolver.Mode)
	}
}
