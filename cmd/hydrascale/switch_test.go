package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/namespaces"
)

// configuredTailnet is the identifier that the test configuration file holds.
const configuredTailnet = "personal"

// runSwitch runs the command against a configuration file that holds one tailnet.
// runSwitch returns the text that the command printed and the error that it returned.
func runSwitch(t *testing.T, tailnetID string) (string, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "version: 1\ntailnets:\n  - id: " + configuredTailnet + "\nresolver:\n  mode: unified\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) = %v, want no error", path, err)
	}

	// configPath reads this package variable, so the test points it at the temporary file.
	previous := cfgFile
	cfgFile = path
	t.Cleanup(func() { cfgFile = previous })

	cmd := switchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{tailnetID})
	err := cmd.Execute()
	return out.String(), err
}

func TestSwitchCmd(t *testing.T) {
	t.Run("states in the summary that the command changes no state", func(t *testing.T) {
		const want = "Print the namespace name for a tailnet (changes no state)"
		if got := switchCmd().Short; got != want {
			t.Errorf("Short = %q, want %q", got, want)
		}
	})

	t.Run("prints the namespace name of the tailnet", func(t *testing.T) {
		out, err := runSwitch(t, configuredTailnet)
		if err != nil {
			t.Fatalf("switch %s = %v, want no error", configuredTailnet, err)
		}
		want := namespaces.GetNamespaceName(configuredTailnet)
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to contain %q", out, want)
		}
	})

	t.Run("states that the command changes no state", func(t *testing.T) {
		out, _ := runSwitch(t, configuredTailnet)
		if !strings.Contains(out, "changes no state") {
			t.Errorf("output = %q, want it to contain %q", out, "changes no state")
		}
	})

	t.Run("prints the exec routing form", func(t *testing.T) {
		out, _ := runSwitch(t, configuredTailnet)
		want := "hydrascale exec " + configuredTailnet + " -- <command>"
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to contain %q", out, want)
		}
	})

	t.Run("prints the tailscale routing form", func(t *testing.T) {
		out, _ := runSwitch(t, configuredTailnet)
		want := "hydrascale tailscale " + configuredTailnet + " -- <arguments>"
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to contain %q", out, want)
		}
	})

	t.Run("holds no form of the word switch in the output", func(t *testing.T) {
		out, _ := runSwitch(t, configuredTailnet)
		if strings.Contains(strings.ToLower(out), "switch") {
			t.Errorf("output = %q, want it to hold no form of the word %q", out, "switch")
		}
	})

	t.Run("returns no error for a configured tailnet", func(t *testing.T) {
		if _, err := runSwitch(t, configuredTailnet); err != nil {
			t.Errorf("switch %s = %v, want no error", configuredTailnet, err)
		}
	})

	t.Run("returns an error for a tailnet that the configuration file does not hold", func(t *testing.T) {
		if _, err := runSwitch(t, "absent"); err == nil {
			t.Error("switch absent returned no error, want an error")
		}
	})
}
