package main

import (
	"bytes"
	"strings"
	"testing"

	"hydrascale/internal/daemon"
	"hydrascale/internal/namespaces"
)

// envTailnet is the identifier that the env tests pass to the command.
const envTailnet = "personal"

// runEnv runs the env command for one tailnet identifier.
// runEnv returns the text that the command printed and the error that it returned.
func runEnv(t *testing.T, tailnetID string) (string, error) {
	t.Helper()

	cmd := envCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{tailnetID})
	err := cmd.Execute()
	return out.String(), err
}

func TestEnvCmd(t *testing.T) {
	t.Run("prints a function that runs the command in the namespace", func(t *testing.T) {
		out, err := runEnv(t, envTailnet)
		if err != nil {
			t.Fatalf("env %s = %v, want no error", envTailnet, err)
		}
		want := `hstn() { hydrascale exec ` + envTailnet + ` -- "$@"; }`
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to contain %q", out, want)
		}
	})

	t.Run("substitutes the identifier of the tailnet into the function", func(t *testing.T) {
		const other = "work"
		out, err := runEnv(t, other)
		if err != nil {
			t.Fatalf("env %s = %v, want no error", other, err)
		}
		want := `hstn() { hydrascale exec ` + other + ` -- "$@"; }`
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to contain %q", out, want)
		}
	})

	t.Run("keeps the three exports that a script reads", func(t *testing.T) {
		out, _ := runEnv(t, envTailnet)
		wants := []string{
			"export HYDRASCALE_TAILNET=" + envTailnet,
			"export HYDRASCALE_NAMESPACE=" + namespaces.GetNamespaceName(envTailnet),
			"export TAILSCALE_SOCKET=" + daemon.SocketPath(envTailnet),
		}
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Errorf("output = %q, want it to contain %q", out, want)
			}
		}
	})

	t.Run("states in the help that an environment variable moves no shell", func(t *testing.T) {
		const want = "An environment variable does not move the shell into the namespace."
		if got := envCmd().Long; !strings.Contains(got, want) {
			t.Errorf("Long = %q, want it to contain %q", got, want)
		}
	})

	t.Run("follows every eval example in the help with a routed command", func(t *testing.T) {
		// The test reads a line that starts with the shell builtin, so a sentence
		// that holds the word evaluate fails no assertion.
		lines := strings.Split(envCmd().Long, "\n")
		for i, line := range lines {
			if !strings.HasPrefix(strings.TrimSpace(line), "eval ") {
				continue
			}
			next := ""
			for _, candidate := range lines[i+1:] {
				if strings.TrimSpace(candidate) != "" {
					next = strings.TrimSpace(candidate)
					break
				}
			}
			if next == "" {
				continue
			}
			if !strings.HasPrefix(next, "hstn ") && !strings.HasPrefix(next, "hydrascale ") {
				t.Errorf("the line after the eval example is %q, want a line that routes the command", next)
			}
		}
	})
}
