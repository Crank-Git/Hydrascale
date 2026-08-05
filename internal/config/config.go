// Package config provides configuration management for Hydrascale.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"hydrascale/internal/access"
)

// validIDPattern restricts tailnet IDs to safe characters.
// Prevents path traversal and shell argument issues.
var validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`)

// DefaultConfigPath is the default location for the Hydrascale config file.
// Config lives in /etc (declarative system config); runtime state and the API
// socket live under /var/lib/hydrascale. This matches the systemd unit, so the
// CLI and the service always read the same file.
const DefaultConfigPath = "/etc/hydrascale/config.yaml"

// DefaultSecretsPath is the default location of the secrets file.
// The daemon reads a credential from this file at mode 0600 and owner root.
const DefaultSecretsPath = "/etc/hydrascale/secrets.yaml"

// Tailnet represents a single Tailscale tailnet configuration.
type Tailnet struct {
	ID       string `yaml:"id"`
	ExitNode string `yaml:"exit_node,omitempty"`
	// AuthKey carries the tag json:"-", because GET /api/status encodes this struct and
	// returned the key to every caller of the control socket. See SA-1.
	AuthKey    string `yaml:"auth_key,omitempty" json:"-"`
	HostAccess *bool  `yaml:"host_access,omitempty"`
	ControlURL string `yaml:"control_url,omitempty"`
}

// HostDNSConfig holds DNS configuration for host access.
type HostDNSConfig struct {
	Mode string `yaml:"mode,omitempty"` // "hosts" (default) or "resolved"
}

// DNSConfig holds the DNS protection settings.
type DNSConfig struct {
	// AllowUnprotected lets a namespace start when the overlay mount on /etc fails.
	// The default is false, so a host that cannot mount OverlayFS fails loudly.
	AllowUnprotected bool `yaml:"allow_unprotected,omitempty"`
}

// Mesh is a stub for forward compatibility with Phase 2 mesh mode.
type Mesh struct {
	Enabled bool `yaml:"enabled"`
}

// ReconcilerConfig holds reconciler-specific settings.
type ReconcilerConfig struct {
	Interval    time.Duration `yaml:"-"`
	RawInterval string        `yaml:"interval,omitempty"`
}

// ResolverConfig represents DNS resolver configuration.
type ResolverConfig struct {
	Mode        string `yaml:"mode"`
	BindAddress string `yaml:"bind_address,omitempty"`
}

// Config represents the Hydrascale service configuration.
type Config struct {
	Version     int              `yaml:"version,omitempty"`
	HostAccess  bool             `yaml:"host_access,omitempty"`
	ControlURL  string           `yaml:"control_url,omitempty"`
	Tailnets    []Tailnet        `yaml:"tailnets"`
	Resolver    ResolverConfig   `yaml:"resolver"`
	Reconciler  ReconcilerConfig `yaml:"reconciler,omitempty"`
	Mesh        Mesh             `yaml:"mesh,omitempty"`
	EventLog    string           `yaml:"event_log,omitempty"`
	HostDNS     HostDNSConfig    `yaml:"host_dns,omitempty"`
	DNS         DNSConfig        `yaml:"dns,omitempty"`
	InfraSubnet string           `yaml:"infra_subnet,omitempty"` // Default: 10.200.0.0/16
	// SocketGroup, when set, makes the API control socket group-accessible:
	// the daemon chowns /var/lib/hydrascale + api.sock to root:<group> with
	// group-traversable/rw modes. Add a trusted user to that group to let it
	// reach the API (e.g. an SSH-forwarded remote GUI) without being root.
	SocketGroup string `yaml:"socket_group,omitempty"`
	// SecretsFile names the root-only credential store. LoadConfig applies
	// DefaultSecretsPath when the key is absent.
	SecretsFile string `yaml:"secrets_file,omitempty"`
	// Access holds the local rule set. The field is a pointer, because a nil value means
	// that the file holds no access key, which is what the version 0.9 migration detects.
	Access *access.RuleSet `yaml:"access,omitempty"`
}

// TailnetHostAccess returns whether host access is enabled for a specific tailnet,
// resolving per-tailnet override against the global default.
func (c *Config) TailnetHostAccess(tailnetID string) bool {
	for _, tn := range c.Tailnets {
		if tn.ID == tailnetID {
			if tn.HostAccess != nil {
				return *tn.HostAccess
			}
			return c.HostAccess
		}
	}
	return c.HostAccess
}

// EffectiveHostDNSMode returns the DNS mode to use. Defaults to "hosts" if host_access
// is enabled for any tailnet and no mode is specified.
func (c *Config) EffectiveHostDNSMode() string {
	if c.HostDNS.Mode != "" {
		return c.HostDNS.Mode
	}
	for _, tn := range c.Tailnets {
		if c.TailnetHostAccess(tn.ID) {
			return "hosts"
		}
	}
	return ""
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

		if err := ValidateControlURL(tn.ControlURL); err != nil {
			return nil, fmt.Errorf("tailnet %q: %w", tn.ID, err)
		}
	}

	// Validate the local rule set against the tailnets that this file declares.
	if cfg.Access != nil {
		ids := make([]string, 0, len(cfg.Tailnets))
		for _, tn := range cfg.Tailnets {
			ids = append(ids, tn.ID)
		}
		if err := cfg.Access.Validate(ids); err != nil {
			return nil, fmt.Errorf("invalid access block: %w", err)
		}
	}

	// Validate global control_url
	if err := ValidateControlURL(cfg.ControlURL); err != nil {
		return nil, fmt.Errorf("global %w", err)
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

	if cfg.SecretsFile == "" {
		cfg.SecretsFile = DefaultSecretsPath
	}

	// Validate and default infra_subnet
	if cfg.InfraSubnet == "" {
		cfg.InfraSubnet = "10.200.0.0/16"
	} else {
		ip, ipnet, err := net.ParseCIDR(cfg.InfraSubnet)
		if err != nil {
			return nil, fmt.Errorf("invalid infra_subnet %q: %w", cfg.InfraSubnet, err)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("infra_subnet must be an IPv4 CIDR, got %q", cfg.InfraSubnet)
		}
		ones, bits := ipnet.Mask.Size()
		if bits != 32 || ones > 16 {
			return nil, fmt.Errorf("infra_subnet %q is too small, must be at least /16", cfg.InfraSubnet)
		}
	}

	return &cfg, nil
}

// DefaultConfig returns a default v2 configuration.
func DefaultConfig() *Config {
	return &Config{
		Version:     2,
		InfraSubnet: "10.200.0.0/16",
		SecretsFile: DefaultSecretsPath,
		Tailnets:    []Tailnet{},
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

// isLoopbackHost reports whether host holds a loopback IP address.
// host is the host component of a URL, with an optional port.
// isLoopbackHost rejects a name such as localhost, because a name resolves to an address
// that the operator can change.
func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else {
		host = strings.Trim(host, "[]")
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ValidateResolverMode checks that mode is empty or a mode that the resolver runs.
// An empty value keeps the current mode. The resolver runs the mode unified only
// (internal/dns/forwarder.go:233-236).
func ValidateResolverMode(mode string) error {
	if mode == "" || mode == "unified" {
		return nil
	}
	return fmt.Errorf("invalid resolver mode %q: the daemon runs the mode unified only", mode)
}

// SafeStateDir returns the state directory of a tailnet under base, and it reports
// whether the daemon can operate on that directory.
// SafeStateDir rejects an identifier that IsValidID rejects, and it rejects a directory
// that is not a direct child of base. A caller that gets false must change nothing.
func SafeStateDir(base, id string) (string, bool) {
	if !IsValidID(id) {
		return "", false
	}
	dir := filepath.Join(base, id)
	if filepath.Dir(dir) != filepath.Clean(base) {
		return "", false
	}
	return dir, true
}

// AuthKeyEnvVar returns the environment variable name that overrides the auth
// key for a tailnet: HYDRASCALE_AUTHKEY_<ID>, where <ID> is uppercased with
// dashes replaced by underscores (e.g. "corp-prod" -> HYDRASCALE_AUTHKEY_CORP_PROD).
func AuthKeyEnvVar(id string) string {
	return "HYDRASCALE_AUTHKEY_" + strings.ToUpper(strings.ReplaceAll(id, "-", "_"))
}

// ResolveAuthKey returns the auth key for a tailnet, checking env var first, then config.
// Env var format: HYDRASCALE_AUTHKEY_<ID> where ID is uppercased with dashes replaced by underscores.
func ResolveAuthKey(tailnetID string, configKey string) string {
	envKey := AuthKeyEnvVar(tailnetID)
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return configKey
}

// ValidateControlURL checks that a control URL is empty or a valid https URL with a host.
// ValidateControlURL also accepts the http scheme when the host is a loopback address,
// because a Headscale server on the same host carries no traffic across a network.
// Every caller uses this one function, so the control API and the loader apply one rule.
func ValidateControlURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid control_url %q: %w", raw, err)
	}
	if u.Scheme == "http" && isLoopbackHost(u.Host) {
		return nil
	}
	if u.Scheme != "https" {
		return fmt.Errorf("control_url %q must use https scheme, unless the host is a loopback address", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("control_url %q has no host", raw)
	}
	return nil
}

// ResolveControlURL returns the per-tailnet control URL if set, otherwise the global default.
func ResolveControlURL(perTailnet, global string) string {
	if perTailnet != "" {
		return perTailnet
	}
	return global
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
