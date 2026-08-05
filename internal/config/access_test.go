package config

import (
	"strings"
	"testing"
)

func TestLoadConfigAccessBlock(t *testing.T) {
	t.Run("leaves the rule set absent when the file holds no access key", func(t *testing.T) {
		tmp := writeTemp(t, `
version: 2
tailnets:
  - id: "alpha"
resolver:
  mode: "unified"
`)
		cfg, err := LoadConfig(tmp)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.Access != nil {
			t.Errorf("Access = %+v, want nil", cfg.Access)
		}
	})

	t.Run("reads the mode and the rules", func(t *testing.T) {
		tmp := writeTemp(t, `
version: 2
tailnets:
  - id: "alpha"
  - id: "beta"
access:
  mode: "observe"
  rules:
    - from: "alpha"
      to: "beta"
      ports: ["tcp/22"]
    - from: "alpha"
      to: "internet"
`)
		cfg, err := LoadConfig(tmp)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.Access == nil {
			t.Fatal("Access = nil, want a rule set")
		}
		if cfg.Access.Mode != "observe" {
			t.Errorf("Mode = %q, want %q", cfg.Access.Mode, "observe")
		}
		if len(cfg.Access.Rules) != 2 {
			t.Fatalf("len(Rules) = %d, want 2", len(cfg.Access.Rules))
		}
		if cfg.Access.Rules[0].Ports[0] != "tcp/22" {
			t.Errorf("Ports[0] = %q, want %q", cfg.Access.Rules[0].Ports[0], "tcp/22")
		}
	})

	t.Run("rejects a rule that names a tailnet that the file does not declare", func(t *testing.T) {
		tmp := writeTemp(t, `
version: 2
tailnets:
  - id: "alpha"
access:
  rules:
    - from: "alpha"
      to: "gamma"
`)
		_, err := LoadConfig(tmp)
		if err == nil {
			t.Fatal("LoadConfig returned no error, want an error")
		}
		if !strings.Contains(err.Error(), "gamma") {
			t.Errorf("error = %q, want the identifier gamma in the message", err)
		}
	})
}
