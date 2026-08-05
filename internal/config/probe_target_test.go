package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeProbeTargetConfig writes a configuration file that declares one tailnet and the
// probe target, and it returns the path.
func writeProbeTargetConfig(t *testing.T, target string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "version: 2\nprobe_target: " + target + "\ntailnets:\n  - id: havoc\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write the configuration file: %v", err)
	}
	return path
}

func TestTheConfigurationFileCarriesTheProbeTarget(t *testing.T) {
	cfg, err := LoadConfig(writeProbeTargetConfig(t, "1.1.1.1"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ProbeTarget != "1.1.1.1" {
		t.Errorf("probe_target is %q, want 1.1.1.1", cfg.ProbeTarget)
	}
}

func TestAFileWithNoProbeTargetLeavesTheFieldEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("version: 2\ntailnets:\n  - id: havoc\n"), 0644); err != nil {
		t.Fatalf("write the configuration file: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ProbeTarget != "" {
		t.Errorf("probe_target is %q, want the empty value that selects the default gateway", cfg.ProbeTarget)
	}
}

func TestAProbeTargetThatIsANameIsRejected(t *testing.T) {
	if _, err := LoadConfig(writeProbeTargetConfig(t, "one.one.one.one")); err == nil {
		t.Fatal("LoadConfig accepted a name as the probe target")
	}
}
