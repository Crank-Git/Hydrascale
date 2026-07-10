package main

import "testing"

// startTunnel must reject flag-like / whitespace-bearing input before it ever
// reaches exec.Command("ssh", ...), preventing argv flag smuggling
// (e.g. "-oProxyCommand=...") and malformed -L forward specs.
func TestStartTunnel_RejectsFlagSmuggling(t *testing.T) {
	badHosts := []string{
		"",
		"-oProxyCommand=touch /tmp/pwned",
		"-D",
		"host with space",
		"host\nwith-newline",
	}
	for _, h := range badHosts {
		if _, err := startTunnel(h, "/var/lib/hydrascale/api.sock"); err == nil {
			t.Errorf("expected rejection for ssh host %q, got nil", h)
		}
	}

	badSocks := []string{
		"relative/path",
		"/has:colon",
		"/has space",
		"/has\nnewline",
	}
	for _, s := range badSocks {
		if _, err := startTunnel("validhost", s); err == nil {
			t.Errorf("expected rejection for remote socket %q, got nil", s)
		}
	}
}
