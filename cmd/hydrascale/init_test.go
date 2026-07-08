package main

import "testing"

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
