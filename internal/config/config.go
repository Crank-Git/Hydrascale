// Package config provides configuration management for Hydrascale.
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// validIDPattern restricts tailnet IDs to safe characters.
// Prevents path traversal and shell argument issues.
var validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`)

// DefaultConfigPath is the default location for the Hydrascale config file.
const DefaultConfigPath = "/var/lib/hydrascale/config.yaml"

// Tailnet represents a single Tailscale tailnet configuration.
type Tailnet struct {
	ID       string `yaml:"id"`
	ExitNode string `yaml:"exit_node,omitempty"`
	AuthKey  string `yaml:"auth_key,omitempty"`
}

// Mesh is a stub for forward compatibility with Phase 2 mesh mode.
type Mesh struct {
	Enabled bool `yaml:"enabled"`
}

// ReconcilerConfig holds reconciler-specific settings.
type ReconcilerConfig struct {
	Interval time.Duration `yaml:"-"`
	RawInterval string    `yaml:"interval,omitempty"`
}

// ResolverConfig represents DNS resolver configuration.
type ResolverConfig struct {
	Mode        string `yaml:"mode"`
	BindAddress string `yaml:"bind_address,omitempty"`
}

// Config represents the Hydrascale service configuration.
type Config struct {
	Version    int              `yaml:"version,omitempty"`
	Tailnets   []Tailnet        `yaml:"tailnets"`
	Resolver   ResolverConfig   `yaml:"resolver"`
	Reconciler ReconcilerConfig `yaml:"reconciler,omitempty"`
	Mesh       Mesh             `yaml:"mesh,omitempty"`
	EventLog   string           `yaml:"event_log,omitempty"`
}

// LoadConfig reads and parses a YAML configuration file.
// If the file contains a v1 config (no version field), it is auto-migrated to v2.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate tailnet IDs
	seen := make(map[string]bool, len(cfg.Tailnets))
	for _, tn := range cfg.Tailnets {
		if tn.ID == "" {
			return nil, fmt.Errorf("tailnet ID cannot be empty")
		}
		if !validIDPattern.MatchString(tn.ID) {
			return nil, fmt.Errorf("invalid tailnet ID %q: must match [a-zA-Z0-9._-], start with alphanumeric, max 63 chars", tn.ID)
		}
		if seen[tn.ID] {
			return nil, fmt.Errorf("duplicate tailnet ID %q", tn.ID)
		}
		seen[tn.ID] = true
	}

	// Validate DNS bind address
	if err := ValidateBindAddress(cfg.Resolver.BindAddress); err != nil {
		return nil, err
	}

	// Auto-migrate v1 to v2
	if cfg.Version == 0 {
		cfg.Version = 2
		if cfg.Resolver.Mode == "" {
			cfg.Resolver.Mode = "unified"
		}
	}

	// Parse reconciler interval
	if cfg.Reconciler.RawInterval != "" {
		d, err := time.ParseDuration(cfg.Reconciler.RawInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid reconciler interval %q: %w", cfg.Reconciler.RawInterval, err)
		}
		cfg.Reconciler.Interval = d
	}
	if cfg.Reconciler.Interval == 0 {
		cfg.Reconciler.Interval = 10 * time.Second
	}

	return &cfg, nil
}

// DefaultConfig returns a default v2 configuration.
func DefaultConfig() *Config {
	return &Config{
		Version:  2,
		Tailnets: []Tailnet{},
		Resolver: ResolverConfig{
			Mode: "unified",
		},
		Reconciler: ReconcilerConfig{
			Interval:    10 * time.Second,
			RawInterval: "10s",
		},
	}
}

// IsValidID reports whether id is a valid tailnet ID.
func IsValidID(id string) bool {
	return validIDPattern.MatchString(id)
}

// ValidateBindAddress checks that addr is empty or a loopback host:port.
func ValidateBindAddress(addr string) error {
	if addr == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid bind_address %q: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("bind_address must be loopback, got %q", host)
	}
	return nil
}

// ResolveAuthKey returns the auth key for a tailnet, checking env var first, then config.
// Env var format: HYDRASCALE_AUTHKEY_<ID> where ID is uppercased with dashes replaced by underscores.
func ResolveAuthKey(tailnetID string, configKey string) string {
	envKey := "HYDRASCALE_AUTHKEY_" + strings.ToUpper(strings.ReplaceAll(tailnetID, "-", "_"))
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return configKey
}

// SaveConfig writes the config to disk atomically (temp file + rename).
func SaveConfig(path string, cfg *Config) error {
	// Ensure the reconciler raw interval is set
	if cfg.Reconciler.Interval > 0 && cfg.Reconciler.RawInterval == "" {
		cfg.Reconciler.RawInterval = cfg.Reconciler.Interval.String()
	}
	if cfg.Version == 0 {
		cfg.Version = 2
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write to temp file in same directory (for atomic rename)
	tmp, err := os.CreateTemp(dir, ".hydrascale-config-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename config file: %w", err)
	}

	return nil
}
