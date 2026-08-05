package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"hydrascale/internal/access"
)

// v09Config is a configuration file of version 0.9. It holds no access key.
const v09Config = `version: 2
tailnets:
  - id: "alpha"
  - id: "beta"
resolver:
  mode: "unified"
`

// writeConfigFile writes content to a file named config.yaml at mode and returns the path.
func writeConfigFile(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write the configuration file: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("set the mode of the configuration file: %v", err)
	}
	return path
}

// migrate loads path and runs the migration on it.
func migrate(t *testing.T, path string) *access.RuleSet {
	t.Helper()
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	set, err := MigrateAccess(path, cfg)
	if err != nil {
		t.Fatalf("MigrateAccess: %v", err)
	}
	return set
}

func TestMigrateAccess(t *testing.T) {
	t.Run("produces one rule per tailnet to internet when the file holds no access block", func(t *testing.T) {
		path := writeConfigFile(t, v09Config, 0600)
		set := migrate(t, path)
		if set == nil {
			t.Fatal("MigrateAccess returned no rule set, want the preserving rule set")
		}
		want := []access.Rule{
			{From: "alpha", To: access.Internet},
			{From: "beta", To: access.Internet},
		}
		if !reflect.DeepEqual(set.Rules, want) {
			t.Errorf("Rules = %+v, want %+v", set.Rules, want)
		}
	})

	t.Run("gives every rule that it writes an empty port list", func(t *testing.T) {
		path := writeConfigFile(t, v09Config, 0600)
		set := migrate(t, path)
		for i, rule := range set.Rules {
			if len(rule.Ports) != 0 {
				t.Errorf("Rules[%d].Ports = %v, want an empty list", i, rule.Ports)
			}
		}
	})

	t.Run("writes the backup before it changes the configuration file", func(t *testing.T) {
		path := writeConfigFile(t, v09Config, 0600)
		migrate(t, path)

		backup, err := os.ReadFile(path + BackupSuffix)
		if err != nil {
			t.Fatalf("read the backup: %v", err)
		}
		if string(backup) != v09Config {
			t.Errorf("the backup holds %q, want the bytes of the file before the change", backup)
		}

		changed, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the configuration file: %v", err)
		}
		if string(changed) == v09Config {
			t.Error("the configuration file is unchanged, want the generated access block")
		}
	})

	t.Run("gives the backup the mode of the configuration file", func(t *testing.T) {
		for _, mode := range []os.FileMode{0600, 0640} {
			path := writeConfigFile(t, v09Config, mode)
			migrate(t, path)
			info, err := os.Stat(path + BackupSuffix)
			if err != nil {
				t.Fatalf("stat the backup: %v", err)
			}
			if info.Mode().Perm() != mode {
				t.Errorf("the backup has mode %04o, want %04o", info.Mode().Perm(), mode)
			}
		}
	})

	t.Run("runs once, so a second start changes no file", func(t *testing.T) {
		path := writeConfigFile(t, v09Config, 0600)
		migrate(t, path)

		first, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the configuration file: %v", err)
		}
		if err := os.Remove(path + BackupSuffix); err != nil {
			t.Fatalf("remove the backup: %v", err)
		}

		set := migrate(t, path)
		if set != nil {
			t.Errorf("the second start returned %+v, want no rule set", set)
		}
		if _, err := os.Stat(path + BackupSuffix); !os.IsNotExist(err) {
			t.Error("the second start wrote a backup, want no backup")
		}
		second, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the configuration file: %v", err)
		}
		if string(second) != string(first) {
			t.Errorf("the second start changed the configuration file to %q", second)
		}
	})

	t.Run("makes no change when the file holds an empty access block", func(t *testing.T) {
		empty := `version: 2
tailnets:
  - id: "alpha"
access: {}
`
		path := writeConfigFile(t, empty, 0600)
		set := migrate(t, path)
		if set != nil {
			t.Errorf("MigrateAccess returned %+v, want no rule set", set)
		}
		if _, err := os.Stat(path + BackupSuffix); !os.IsNotExist(err) {
			t.Error("the migration wrote a backup, want no backup")
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the configuration file: %v", err)
		}
		if string(after) != empty {
			t.Errorf("the configuration file holds %q, want the file unchanged", after)
		}
	})

	t.Run("writes a configuration file that loads again and produces the same rule set", func(t *testing.T) {
		path := writeConfigFile(t, v09Config, 0600)
		set := migrate(t, path)

		reloaded, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("LoadConfig after the migration: %v", err)
		}
		if reloaded.Access == nil {
			t.Fatal("the reloaded configuration holds no access block")
		}
		if !reflect.DeepEqual(*reloaded.Access, *set) {
			t.Errorf("the reloaded rule set is %+v, want %+v", *reloaded.Access, *set)
		}
	})

	t.Run("keeps the tailnets of the configuration file", func(t *testing.T) {
		path := writeConfigFile(t, v09Config, 0600)
		migrate(t, path)

		reloaded, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("LoadConfig after the migration: %v", err)
		}
		if len(reloaded.Tailnets) != 2 {
			t.Fatalf("len(Tailnets) = %d, want 2", len(reloaded.Tailnets))
		}
	})
}

func TestBackupFile(t *testing.T) {
	t.Run("copies the bytes of the source file", func(t *testing.T) {
		path := writeConfigFile(t, v09Config, 0600)
		if err := BackupFile(path, path+BackupSuffix); err != nil {
			t.Fatalf("BackupFile: %v", err)
		}
		got, err := os.ReadFile(path + BackupSuffix)
		if err != nil {
			t.Fatalf("read the backup: %v", err)
		}
		if string(got) != v09Config {
			t.Errorf("the backup holds %q, want %q", got, v09Config)
		}
	})

	t.Run("does not widen the mode of a source file that holds a secret", func(t *testing.T) {
		path := writeConfigFile(t, v09Config, 0600)
		if err := BackupFile(path, path+BackupSuffix); err != nil {
			t.Fatalf("BackupFile: %v", err)
		}
		info, err := os.Stat(path + BackupSuffix)
		if err != nil {
			t.Fatalf("stat the backup: %v", err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("the backup has mode %04o, want 0600", info.Mode().Perm())
		}
	})

	t.Run("replaces a backup that a previous run left at a wider mode", func(t *testing.T) {
		path := writeConfigFile(t, v09Config, 0600)
		backup := path + BackupSuffix
		if err := os.WriteFile(backup, []byte("old"), 0644); err != nil {
			t.Fatalf("write the previous backup: %v", err)
		}
		if err := os.Chmod(backup, 0644); err != nil {
			t.Fatalf("set the mode of the previous backup: %v", err)
		}
		if err := BackupFile(path, backup); err != nil {
			t.Fatalf("BackupFile: %v", err)
		}
		info, err := os.Stat(backup)
		if err != nil {
			t.Fatalf("stat the backup: %v", err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("the backup has mode %04o, want 0600", info.Mode().Perm())
		}
	})

	t.Run("returns an error when the source file is absent", func(t *testing.T) {
		dir := t.TempDir()
		err := BackupFile(filepath.Join(dir, "absent.yaml"), filepath.Join(dir, "absent.yaml"+BackupSuffix))
		if err == nil {
			t.Fatal("BackupFile returned no error, want an error")
		}
	})
}
