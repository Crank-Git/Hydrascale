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

// validAliasPattern restricts a tailnet alias to a letter, a digit, a hyphen and an
// underscore. The set excludes the dot that an ID accepts, because an operator types an
// alias by hand and a short flat name reads back without a question.
// An alias reaches no path and no command argument: ResolveTailnetRef answers with the ID
// of the tailnet, and the daemon builds every namespace name, device name and state
// directory from that ID. The pattern therefore bounds the shape of a name, and it is not
// the guard that keeps a path safe.
var validAliasPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`)

// validDNSLabelPattern restricts an alias to one label of a domain name, which carries a
// letter, a digit and a hyphen, and which starts and ends with a letter or a digit.
// resolver.resolve_aliases builds a domain name from an alias, and a domain name takes no
// underscore, therefore the key narrows the alias to this pattern.
var validDNSLabelPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// aliasDomainSuffix is the parent domain of an alias zone. internal/dns holds the same
// value as dns.AliasDomainSuffix; this package states it again rather than import that
// package, which would make a cycle.
const aliasDomainSuffix = "ts.internal"

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
	// Alias is a second name for this tailnet. Every command that takes a tailnet ID takes
	// the alias as well. An alias holds a letter, a digit, a hyphen and an underscore.
	// The daemon reads the alias in two places only: it compares the alias to answer with
	// the ID of the tailnet, and it prints the alias. No path, no device name and no
	// command argument carries it.
	Alias string `yaml:"alias,omitempty"`
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

	// ResolveAliases makes the daemon answer the short name <host>.<alias>.ts.internal
	// for each tailnet that holds an alias. The daemon answers the name from the peer
	// table of that tailnet, and it answers no other name of that domain.
	// An unset key keeps the DNS behaviour that the daemon holds without it.
	ResolveAliases bool `yaml:"resolve_aliases,omitempty"`
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
	// ProbeTarget is the address that each namespace sends one packet to, so that the
	// status response reports measured reachability. An empty value selects
	// reach.DefaultTarget, which is public. An operator who accepts no packet to a third
	// party declares an address inside a tailnet here. An address on the local network
	// reports unreachable, because the default rule set denies every private destination.
	ProbeTarget string `yaml:"probe_target,omitempty"`
	// Access holds the local rule set. The field is a pointer, because a nil value means
	// that the file holds no access key, which is what the version 0.9 migration detects.
	Access *access.RuleSet `yaml:"access,omitempty"`
	// Console holds the console listener settings. Every key has a default, so a version
	// 0.9 file that holds no console key serves the console on the loopback address.
	Console ConsoleConfig `yaml:"console,omitempty"`
}

// AccessMode returns the mode that the daemon applies the local rule set in.
// AccessMode returns access.ModeEnforce when the file holds no access key, and when the
// access block holds no mode key.
func (c *Config) AccessMode() string {
	if c.Access == nil {
		return access.ModeEnforce
	}
	return c.Access.EffectiveMode()
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
	if err := cfg.ValidateTailnetNames(); err != nil {
		return nil, err
	}
	for _, tn := range cfg.Tailnets {
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

	// Validate the console bind address. The loader reads the effective value, so a file
	// that holds no console key gets the loopback default rather than an error.
	if err := ValidateConsoleBindAddress(cfg.ConsoleBindAddress()); err != nil {
		return nil, err
	}

	// The daemon passes the probe target to `ping` inside a namespace, therefore the file
	// declares an address and never a name. A name needs a resolver inside the namespace,
	// and a failed lookup would report a broken path.
	if cfg.ProbeTarget != "" && net.ParseIP(cfg.ProbeTarget) == nil {
		return nil, fmt.Errorf("invalid probe_target %q: declare an IP address", cfg.ProbeTarget)
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

// ValidateTailnetNames checks the ID and the alias of every tailnet. An ID is unique among
// the IDs, an alias is unique among the aliases, and an alias is the ID of no tailnet. An
// ID obeys validIDPattern and an alias obeys validAliasPattern.
// LoadConfig and SaveConfig both call ValidateTailnetNames. A writer that appends a tailnet
// therefore cannot store a file that the loader refuses, which would leave every later
// command without a configuration. See issue for `hydrascale add <alias>`.
// ValidateTailnetNames returns the first failure that it finds.
func (c *Config) ValidateTailnetNames() error {
	ids := make(map[string]bool, len(c.Tailnets))
	for _, tn := range c.Tailnets {
		if tn.ID == "" {
			return fmt.Errorf("tailnet ID cannot be empty")
		}
		if !validIDPattern.MatchString(tn.ID) {
			return fmt.Errorf("invalid tailnet ID %q: must match [a-zA-Z0-9._-], start with alphanumeric, max 63 chars", tn.ID)
		}
		if ids[tn.ID] {
			return fmt.Errorf("duplicate tailnet ID %q", tn.ID)
		}
		ids[tn.ID] = true
	}

	// The alias check runs after the loop above. An alias must differ from the ID of every
	// tailnet, and not only from the IDs that the file declares before it.
	aliases := make(map[string]string, len(c.Tailnets))
	for _, tn := range c.Tailnets {
		if tn.Alias == "" {
			continue
		}
		if !validAliasPattern.MatchString(tn.Alias) {
			return fmt.Errorf("invalid alias %q of tailnet %q: must match [a-zA-Z0-9_-], start with alphanumeric, max 63 chars", tn.Alias, tn.ID)
		}
		if ids[tn.Alias] {
			return fmt.Errorf("alias %q of tailnet %q is the ID of a tailnet", tn.Alias, tn.ID)
		}
		// resolve_aliases builds the domain <alias>.ts.internal, and a domain name folds
		// case, therefore two aliases that differ by case alone name one zone. The check
		// runs only when the key is set, so a file that leaves the key out keeps every
		// alias that it holds now.
		key := tn.Alias
		if c.Resolver.ResolveAliases {
			key = strings.ToLower(tn.Alias)
			for id := range ids {
				if strings.EqualFold(id, tn.Alias) {
					return fmt.Errorf("alias %q of tailnet %q is the ID of the tailnet %q, which differs by case alone: resolver.resolve_aliases builds a domain name, and a domain name folds case", tn.Alias, tn.ID, id)
				}
			}
		}
		// An alias takes an underscore, and a DNS label does not. resolve_aliases turns
		// each alias into the domain <alias>.ts.internal, therefore it narrows the alias
		// to a DNS label. The check runs only when the key is set, so a file that leaves
		// the key out keeps every alias that it holds now.
		if c.Resolver.ResolveAliases && !validDNSLabelPattern.MatchString(tn.Alias) {
			return fmt.Errorf("alias %q of tailnet %q is not a DNS label: resolver.resolve_aliases builds the domain %s.%s, so the alias takes a letter, a digit and a hyphen, and it takes no underscore", tn.Alias, tn.ID, tn.Alias, aliasDomainSuffix)
		}
		if other, ok := aliases[key]; ok {
			return fmt.Errorf("duplicate alias %q: tailnet %q and tailnet %q both hold it", tn.Alias, other, tn.ID)
		}
		aliases[key] = tn.ID
	}
	return nil
}

// ResolveTailnetRef returns the ID of the tailnet that ref names. ref is an ID or an
// alias, and an ID wins over an alias. The loader rejects a file in which an alias equals
// an ID, therefore the two never name different tailnets.
// ResolveTailnetRef returns an error when no tailnet holds ref.
func (c *Config) ResolveTailnetRef(ref string) (string, error) {
	for _, tn := range c.Tailnets {
		if tn.ID == ref {
			return tn.ID, nil
		}
	}
	for _, tn := range c.Tailnets {
		if tn.Alias != "" && tn.Alias == ref {
			return tn.ID, nil
		}
	}
	return "", fmt.Errorf("no tailnet holds the ID or the alias %q", ref)
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
	// A file that holds a name collision stops every later command, because each one loads
	// the file first. SaveConfig therefore refuses the write rather than store that state.
	if err := cfg.ValidateTailnetNames(); err != nil {
		return err
	}

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
