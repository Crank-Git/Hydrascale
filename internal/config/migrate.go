package config

import (
	"fmt"
	"os"
	"path/filepath"

	"hydrascale/internal/access"
)

// BackupSuffix is the suffix of the copy that the version 0.9 migration keeps.
const BackupSuffix = ".pre-v1.backup"

// BackupFile writes a copy of the file at path to backupPath.
// The copy takes the mode of the source file, because the configuration file can hold an
// auth key; see SA-23 and SA-24 of docs/security-audit.md.
// BackupFile writes a temporary file in the directory of backupPath and it renames that
// file over backupPath, so a reader never sees a partial copy and a previous copy at a
// wider mode never survives.
// BackupFile returns an error when it cannot read the source file, and it returns an
// error when it cannot write the copy. It changes no file in either case.
func BackupFile(path, backupPath string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", path, err)
	}

	dir := filepath.Dir(backupPath)
	tmp, err := os.CreateTemp(dir, ".hydrascale-backup-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set the mode of the temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, backupPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename %s: %w", backupPath, err)
	}
	return nil
}

// MigrateAccess writes the rule set that preserves the reachability of version 0.9 into
// the configuration file at path.
// cfg is the configuration that LoadConfig returned for path. MigrateAccess sets the
// access block of cfg when it migrates.
// MigrateAccess detects the migration by the presence of the access key, not by a version
// marker, so an empty access block that an operator wrote by hand suppresses it.
// MigrateAccess returns the rule set that it wrote. It returns nil when the configuration
// already holds an access block, and it changes no file in that case.
// MigrateAccess writes the copy at path + BackupSuffix before it changes the
// configuration file. It returns an error when the copy fails, when the rule set fails
// validation, or when the write of the configuration file fails.
func MigrateAccess(path string, cfg *Config) (*access.RuleSet, error) {
	if cfg.Access != nil {
		return nil, nil
	}

	ids := make([]string, 0, len(cfg.Tailnets))
	for _, tn := range cfg.Tailnets {
		ids = append(ids, tn.ID)
	}
	set := access.PreservingRuleSet(ids)
	if err := set.Validate(ids); err != nil {
		return nil, fmt.Errorf("the preserving rule set fails validation: %w", err)
	}

	if err := BackupFile(path, path+BackupSuffix); err != nil {
		return nil, fmt.Errorf("failed to back up the config: %w", err)
	}

	cfg.Access = &set
	if err := SaveConfig(path, cfg); err != nil {
		cfg.Access = nil
		return nil, fmt.Errorf("failed to write the access block: %w", err)
	}
	return &set, nil
}
