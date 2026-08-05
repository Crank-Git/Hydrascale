package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConsoleConfig writes body to a temporary configuration file and returns the path.
func writeConsoleConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestAConfigurationFileWithNoConsoleBlockServesTheLoopbackConsole(t *testing.T) {
	// A version 0.9 file holds no console key, so the defaults must make the console
	// serve 127.0.0.1:9443.
	path := writeConsoleConfig(t, "version: 2\ntailnets: []\nresolver:\n  mode: unified\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.ConsoleEnabled() {
		t.Error("ConsoleEnabled() = false, want true for a file that holds no console key")
	}
	if got := cfg.ConsoleBindAddress(); got != DefaultConsoleBindAddress {
		t.Errorf("ConsoleBindAddress() = %q, want %q", got, DefaultConsoleBindAddress)
	}
}

func TestAConfigurationFileCanDisableTheConsole(t *testing.T) {
	path := writeConsoleConfig(t, "version: 2\ntailnets: []\nresolver:\n  mode: unified\nconsole:\n  enabled: false\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ConsoleEnabled() {
		t.Error("ConsoleEnabled() = true, want false for console.enabled: false")
	}
}

func TestTheLoaderRefusesANonLoopbackConsoleBindAddress(t *testing.T) {
	path := writeConsoleConfig(t, "version: 2\ntailnets: []\nresolver:\n  mode: unified\nconsole:\n  bind_address: 0.0.0.0:9443\n")

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig accepted console.bind_address: 0.0.0.0:9443, want an error")
	}
	if !strings.Contains(err.Error(), "console.bind_address") {
		t.Errorf("the error %q does not name the configuration key console.bind_address", err)
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Errorf("the error %q does not state that the console must bind a loopback address", err)
	}
}

func TestValidateConsoleBindAddressAcceptsEachLoopbackAddress(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:9443", "[::1]:9443", "127.0.0.2:9443"} {
		if err := ValidateConsoleBindAddress(addr); err != nil {
			t.Errorf("ValidateConsoleBindAddress(%q) = %v, want nil", addr, err)
		}
	}
}

func TestValidateConsoleBindAddressRefusesAnAddressThatIsNotLoopback(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:9443", "192.168.1.10:9443", "[::]:9443", "localhost:9443", "127.0.0.1", ""} {
		if err := ValidateConsoleBindAddress(addr); err == nil {
			t.Errorf("ValidateConsoleBindAddress(%q) = nil, want an error", addr)
		}
	}
}
