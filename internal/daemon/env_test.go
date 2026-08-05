package daemon

import (
	"strings"
	"testing"
)

func TestMinimalChildEnv_carries_no_auth_key_variable(t *testing.T) {
	t.Setenv("HYDRASCALE_AUTHKEY_CORP", "tskey-auth-kTESTSECRET123")
	t.Setenv("PATH", "/usr/bin")

	env := minimalChildEnv()

	for _, entry := range env {
		if strings.HasPrefix(entry, "HYDRASCALE_") {
			t.Errorf("the child environment carries %q", entry)
		}
		if strings.Contains(entry, "tskey-auth-kTESTSECRET123") {
			t.Errorf("the child environment carries an auth key value: %q", entry)
		}
	}
	if len(env) == 0 {
		t.Fatal("the child environment is empty, and PATH must be present")
	}
	var hasPath bool
	for _, entry := range env {
		if entry == "PATH=/usr/bin" {
			hasPath = true
		}
	}
	if !hasPath {
		t.Errorf("the child environment carries no PATH, got %v", env)
	}
}
