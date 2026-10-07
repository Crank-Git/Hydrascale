package docscheck

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestConfigKeysNamesEachKeyAsADottedPath(t *testing.T) {
	keys := ConfigKeys()
	for _, want := range []string{
		"version",
		"route_table",
		"host_dns",
		"host_dns.mode",
		"console.bind_address",
		"reconciler.interval",
		"tailnets",
		"tailnets[].id",
		"access.rules[].ports",
	} {
		if !slices.Contains(keys, want) {
			t.Errorf("ConfigKeys holds no %q: %q", want, keys)
		}
	}
	if !slices.IsSorted(keys) {
		t.Errorf("ConfigKeys is not sorted: %q", keys)
	}
}

func TestConfigKeysOmitsAFieldThatTheFileDoesNotRead(t *testing.T) {
	// ReconcilerConfig.Interval carries the tag yaml:"-", because LoadConfig fills it from
	// reconciler.interval.
	for _, key := range ConfigKeys() {
		if key == "reconciler.-" || key == "reconciler." || key == "" {
			t.Errorf("ConfigKeys holds the key %q, which the file does not read", key)
		}
	}
}

func TestEventTypesReadsEachLiteralAndEachEventConstant(t *testing.T) {
	got, err := EventTypes(filepath.Join("testdata", "events"))
	if err != nil {
		t.Fatalf("EventTypes: %v", err)
	}
	want := []string{"sample.constant", "sample.literal"}
	if !slices.Equal(got, want) {
		t.Errorf("EventTypes returns %q, want %q", got, want)
	}
}

func TestEventTypesFindsTheEventsOfEachPackageOfTheDaemon(t *testing.T) {
	got, err := EventTypes(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("EventTypes: %v", err)
	}
	for _, want := range []string{
		"access.jump_displaced",     // a literal in internal/reconciler
		"reconcile_complete",        // a literal in internal/reconciler
		"console.request",           // a constant in internal/api
		"policy.pushed",             // a constant in internal/api
		"dns.split_domain_conflict", // a constant in internal/hostaccess
	} {
		if !slices.Contains(got, want) {
			t.Errorf("EventTypes holds no %q: %q", want, got)
		}
	}
}

func TestEventTypesFailsOnAMissingDirectory(t *testing.T) {
	if _, err := EventTypes(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("EventTypes returns no error for a directory that does not exist")
	}
}
