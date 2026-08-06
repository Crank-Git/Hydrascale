package config

import (
	"fmt"
	"net"
)

// DefaultConsoleBindAddress is the address that the console serves when the
// configuration file holds no console.bind_address key.
const DefaultConsoleBindAddress = "127.0.0.1:9443"

// ConsoleConfig holds the console listener settings.
// Enabled is a pointer, because a version 0.9 file holds no console key and an absent key
// means true. A false value disables the listener.
type ConsoleConfig struct {
	Enabled     *bool  `yaml:"enabled,omitempty"`
	BindAddress string `yaml:"bind_address,omitempty"`
}

// ConsoleEnabled reports whether the daemon opens the console listener.
// ConsoleEnabled returns true when the file holds no console.enabled key.
func (c *Config) ConsoleEnabled() bool {
	if c.Console.Enabled == nil {
		return true
	}
	return *c.Console.Enabled
}

// ConsoleBindAddress returns the address that the console listener binds.
// ConsoleBindAddress returns DefaultConsoleBindAddress when the file holds no
// console.bind_address key.
func (c *Config) ConsoleBindAddress() string {
	if c.Console.BindAddress == "" {
		return DefaultConsoleBindAddress
	}
	return c.Console.BindAddress
}

// ValidateConsoleBindAddress checks that addr is a loopback host and a port.
// The console has no authentication, so a listener on a non-loopback address gives every
// host on the network control of a root daemon. See the section "The console has no
// authentication" of docs/specs/spec.md.
//
// ValidateConsoleBindAddress refuses a name such as localhost, because a name resolves to
// an address that the operator can change.
func ValidateConsoleBindAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("console.bind_address is required")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid console.bind_address %q: %w", addr, err)
	}
	if port == "" {
		return fmt.Errorf("invalid console.bind_address %q: the address holds no port", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("invalid console.bind_address %q: the console must bind a loopback address, such as %s", addr, DefaultConsoleBindAddress)
	}
	return nil
}
