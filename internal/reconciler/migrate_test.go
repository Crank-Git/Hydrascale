package reconciler

import (
	"os"
	"testing"

	"hydrascale/internal/config"
)

func TestMigrateAccess(t *testing.T) {
	t.Run("records the event access.migrated with the rules that it wrote", func(t *testing.T) {
		cfgPath := writeTestConfig(t, "alpha", "beta")
		r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())

		if err := r.MigrateAccess(); err != nil {
			t.Fatalf("MigrateAccess: %v", err)
		}
		if !hasEvent(r, "access.migrated", "alpha -> internet") {
			t.Errorf("the event list holds no access.migrated for alpha: %v", r.Events())
		}
		if !hasEvent(r, "access.migrated", "beta -> internet") {
			t.Errorf("the event list holds no access.migrated for beta: %v", r.Events())
		}
	})

	t.Run("records no event when the configuration file holds an access block", func(t *testing.T) {
		cfgPath := writeTestConfig(t, "alpha")
		r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
		if err := r.MigrateAccess(); err != nil {
			t.Fatalf("the first MigrateAccess: %v", err)
		}

		second := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
		if err := second.MigrateAccess(); err != nil {
			t.Fatalf("the second MigrateAccess: %v", err)
		}
		if hasEvent(second, "access.migrated", "") {
			t.Errorf("the second start recorded access.migrated: %v", second.Events())
		}
	})

	t.Run("writes the backup of the configuration file", func(t *testing.T) {
		cfgPath := writeTestConfig(t, "alpha")
		r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
		if err := r.MigrateAccess(); err != nil {
			t.Fatalf("MigrateAccess: %v", err)
		}
		if _, err := os.Stat(cfgPath + config.BackupSuffix); err != nil {
			t.Errorf("stat the backup: %v", err)
		}
	})
}
