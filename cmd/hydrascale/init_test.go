package main

import (
	"strings"
	"testing"

	"hydrascale/internal/config"
)

func TestPromptGUIAccess(t *testing.T) {
	const (
		consequence = "a member of this group can send a command to the daemon"
		equivalence = "equivalent to root access on this host"
		question    = "Enable non-root access via a unix group?"
	)

	// The operator answers "n", so the prompt returns before it creates a group.
	run := func(t *testing.T) string {
		t.Helper()
		var out strings.Builder
		promptGUIAccess(newPrompter(strings.NewReader("n\n"), &out), &config.Config{})
		return out.String()
	}

	t.Run("states that group membership is equivalent to root access", func(t *testing.T) {
		got := run(t)
		for _, want := range []string{consequence, equivalence} {
			if !strings.Contains(got, want) {
				t.Errorf("output = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("states the root equivalence before the socket group prompt", func(t *testing.T) {
		got := run(t)
		warning := strings.Index(got, equivalence)
		prompt := strings.Index(got, question)
		if warning < 0 || prompt < 0 {
			t.Fatalf("output = %q, want it to hold the warning and the prompt", got)
		}
		if warning > prompt {
			t.Errorf("the warning is at byte %d and the prompt is at byte %d, want the warning first", warning, prompt)
		}
	})
}

func TestCheckHostAcceptDNS(t *testing.T) {
	t.Run("returns an error when the host tailscaled accepts DNS", func(t *testing.T) {
		warning, err := checkHostAcceptDNS(true, false)
		if err == nil {
			t.Fatal("checkHostAcceptDNS(true, false) returned no error")
		}
		if warning != "" {
			t.Errorf("warning = %q, want empty", warning)
		}
	})

	t.Run("names the fix command in the failure message", func(t *testing.T) {
		_, err := checkHostAcceptDNS(true, false)
		if err == nil {
			t.Fatal("checkHostAcceptDNS(true, false) returned no error")
		}
		if !strings.Contains(err.Error(), "sudo tailscale set --accept-dns=false") {
			t.Errorf("message = %q, want it to contain %q", err.Error(), "sudo tailscale set --accept-dns=false")
		}
	})

	t.Run("returns a warning and no error when force is set", func(t *testing.T) {
		warning, err := checkHostAcceptDNS(true, true)
		if err != nil {
			t.Fatalf("checkHostAcceptDNS(true, true) = %v, want no error", err)
		}
		if !strings.Contains(warning, "sudo tailscale set --accept-dns=false") {
			t.Errorf("warning = %q, want it to contain %q", warning, "sudo tailscale set --accept-dns=false")
		}
	})

	t.Run("returns no error when the host runs no tailscaled", func(t *testing.T) {
		warning, err := checkHostAcceptDNS(false, false)
		if err != nil {
			t.Fatalf("checkHostAcceptDNS(false, false) = %v, want no error", err)
		}
		if warning != "" {
			t.Errorf("warning = %q, want empty", warning)
		}
	})
}

func TestIsLoggedIn(t *testing.T) {
	loggedOut := "Logged out.\nLog in at: https://login.tailscale.com/a/abc123\n"
	loggedIn := "100.72.247.43  mars-1  johhnybones28@  linux  -\n"
	cases := map[string]bool{
		loggedOut:      false,
		loggedIn:       true,
		"":             false,
		"NeedsLogin\n": false,
		"   \n":        false,
	}
	for in, want := range cases {
		if got := isLoggedIn(in); got != want {
			t.Errorf("isLoggedIn(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLoginURL(t *testing.T) {
	status := "Logged out.\n\nTo authenticate, visit:\n\n\thttps://login.tailscale.com/a/deadbeef\n\n"
	if got := loginURL(status); got != "https://login.tailscale.com/a/deadbeef" {
		t.Errorf("loginURL = %q", got)
	}
	if got := loginURL("100.72.247.43 mars-1 johhnybones28@ linux -"); got != "" {
		t.Errorf("loginURL(logged-in) = %q, want empty", got)
	}
}
