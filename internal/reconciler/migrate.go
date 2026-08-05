package reconciler

import (
	"fmt"
	"strings"

	"hydrascale/internal/config"
)

// MigrateAccess writes the rule set that preserves the reachability of version 0.9 when
// the configuration file holds no access block.
// MigrateAccess runs at start, before the first reconcile.
// MigrateAccess records the event access.migrated with every rule that it wrote. It
// records no event and it changes no file when the configuration file already holds an
// access block.
// MigrateAccess returns an error when it cannot read the configuration file, and it
// returns an error when it cannot write the copy or the access block.
func (r *Reconciler) MigrateAccess() error {
	cfg, err := config.LoadConfig(r.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	set, err := config.MigrateAccess(r.configPath, cfg)
	if err != nil {
		return err
	}
	if set == nil {
		return nil
	}

	written := make([]string, 0, len(set.Rules))
	for _, rule := range set.Rules {
		written = append(written, rule.String())
	}
	r.emit("access.migrated", "", fmt.Sprintf("wrote %d rules: %s", len(written), strings.Join(written, ", ")))
	return nil
}
