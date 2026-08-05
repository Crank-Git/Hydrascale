package daemon

import (
	"encoding/json"
	"testing"
)

func TestTheParsedStatusHoldsTheBackendStateAndTheLoginURL(t *testing.T) {
	// A namespace whose node is not logged in reports BackendState NeedsLogin and it
	// carries the URL that authorizes the node. The console shows both, therefore the
	// daemon reads both out of tailscale status --json.
	const raw = `{
	  "BackendState": "NeedsLogin",
	  "AuthURL": "https://controlplane.example.net/register/nodekey:0000",
	  "Self": {"HostName": "phobos"},
	  "Peer": {},
	  "MagicDNSSuffix": "alpha.ts.net"
	}`

	var status TailscaleStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		t.Fatalf("parse the status: %v", err)
	}
	if status.BackendState != "NeedsLogin" {
		t.Errorf("the status holds the backend state %q, want NeedsLogin", status.BackendState)
	}
	if want := "https://controlplane.example.net/register/nodekey:0000"; status.AuthURL != want {
		t.Errorf("the status holds the login URL %q, want %q", status.AuthURL, want)
	}
}
