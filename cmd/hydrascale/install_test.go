package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSystemdUnitMatchesContrib guards against the regression in issue #25:
// `hydrascale install` used to embed a duplicate systemd unit string that
// drifted from contrib/hydrascale.service, so fixes to the contrib file
// (removing incompatible sandboxing) silently failed to reach installed
// systems. Keep them byte-identical.
func TestSystemdUnitMatchesContrib(t *testing.T) {
	contribPath := filepath.Join("..", "..", "contrib", "hydrascale.service")
	contrib, err := os.ReadFile(contribPath)
	if err != nil {
		t.Fatalf("read %s: %v", contribPath, err)
	}
	if string(contrib) != systemdUnit {
		t.Fatalf("embedded systemd unit drifted from %s.\n"+
			"Update cmd/hydrascale/hydrascale.service to match contrib/hydrascale.service "+
			"(or vice versa) so `hydrascale install` writes the same content users get "+
			"when they copy the contrib file manually.", contribPath)
	}
}
