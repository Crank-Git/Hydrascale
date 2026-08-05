package dns

import (
	"os"
	"path/filepath"
	"testing"
)

// writeHostFile writes content to path and fails the test on an error.
func writeHostFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

const stubResolvConf = "nameserver 127.0.0.53\nsearch internal.example.com\noptions edns0\n"

func TestHostFileMonitor(t *testing.T) {
	t.Run("reports no change on the first check", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "resolv.conf")
		writeHostFile(t, path, stubResolvConf)

		m := NewHostFileMonitor(path)
		changed, _, current, err := m.Check()
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if changed {
			t.Error("changed = true, want false on the first check")
		}
		if current.Checksum == "" {
			t.Error("Checksum is empty, want the checksum of the file")
		}
	})

	t.Run("reports one change when the host file changes once", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "resolv.conf")
		writeHostFile(t, path, stubResolvConf)

		m := NewHostFileMonitor(path)
		if _, err := m.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}

		writeHostFile(t, path, "nameserver 100.100.100.100\nsearch internal.example.com\n")

		changed, previous, current, err := m.Check()
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if !changed {
			t.Fatal("changed = false, want true after the file changes")
		}
		if previous.FirstLine != "nameserver 127.0.0.53" {
			t.Errorf("previous.FirstLine = %q, want %q", previous.FirstLine, "nameserver 127.0.0.53")
		}
		if current.FirstLine != "nameserver 100.100.100.100" {
			t.Errorf("current.FirstLine = %q, want %q", current.FirstLine, "nameserver 100.100.100.100")
		}

		changed, _, _, err = m.Check()
		if err != nil {
			t.Fatalf("second Check: %v", err)
		}
		if changed {
			t.Error("changed = true on the second check, want false, because one change gives one report")
		}
	})

	t.Run("reports no change when the host file stays the same", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "resolv.conf")
		writeHostFile(t, path, stubResolvConf)

		m := NewHostFileMonitor(path)
		if _, err := m.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		for i := 0; i < 3; i++ {
			changed, _, _, err := m.Check()
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if changed {
				t.Fatalf("changed = true on check %d, want false", i+1)
			}
		}
	})

	t.Run("reports a change when the symbolic link points to another file", func(t *testing.T) {
		dir := t.TempDir()
		first := filepath.Join(dir, "stub-resolv.conf")
		second := filepath.Join(dir, "resolv-uplink.conf")
		link := filepath.Join(dir, "resolv.conf")
		writeHostFile(t, first, stubResolvConf)
		writeHostFile(t, second, stubResolvConf)
		if err := os.Symlink(first, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		m := NewHostFileMonitor(link)
		start, err := m.Start()
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		if start.LinkTarget != first {
			t.Errorf("LinkTarget = %q, want %q", start.LinkTarget, first)
		}

		if err := os.Remove(link); err != nil {
			t.Fatalf("remove link: %v", err)
		}
		if err := os.Symlink(second, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		changed, _, current, err := m.Check()
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if !changed {
			t.Error("changed = false, want true, because the link target path is part of the checksum")
		}
		if current.LinkTarget != second {
			t.Errorf("LinkTarget = %q, want %q", current.LinkTarget, second)
		}
	})

	t.Run("reports a change when the symbolic link target content changes", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "stub-resolv.conf")
		link := filepath.Join(dir, "resolv.conf")
		writeHostFile(t, target, stubResolvConf)
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		m := NewHostFileMonitor(link)
		if _, err := m.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		writeHostFile(t, target, "nameserver 100.100.100.100\n")

		changed, _, current, err := m.Check()
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if !changed {
			t.Error("changed = false, want true after the target content changes")
		}
		if current.FirstLine != "nameserver 100.100.100.100" {
			t.Errorf("FirstLine = %q, want %q", current.FirstLine, "nameserver 100.100.100.100")
		}
	})

	t.Run("reports the file as missing rather than as changed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "resolv.conf")

		m := NewHostFileMonitor(path)
		start, err := m.Start()
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		if !start.Missing {
			t.Error("Missing = false, want true for a file that does not exist")
		}
		if start.Checksum != "" {
			t.Errorf("Checksum = %q, want an empty checksum", start.Checksum)
		}

		changed, _, _, err := m.Check()
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if changed {
			t.Error("changed = true, want false, because a missing file is missing and not changed")
		}
	})

	t.Run("holds the first line only", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "resolv.conf")
		writeHostFile(t, path, stubResolvConf)

		state, err := ReadHostFileState(path)
		if err != nil {
			t.Fatalf("ReadHostFileState: %v", err)
		}
		if state.FirstLine != "nameserver 127.0.0.53" {
			t.Errorf("FirstLine = %q, want %q", state.FirstLine, "nameserver 127.0.0.53")
		}
	})

	t.Run("records the time of the last change", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "resolv.conf")
		writeHostFile(t, path, stubResolvConf)

		m := NewHostFileMonitor(path)
		if _, err := m.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if _, last := m.State(); !last.IsZero() {
			t.Errorf("last change = %v, want the zero time before a change", last)
		}

		writeHostFile(t, path, "nameserver 100.100.100.100\n")
		if _, _, _, err := m.Check(); err != nil {
			t.Fatalf("Check: %v", err)
		}

		state, last := m.State()
		if last.IsZero() {
			t.Error("last change is the zero time, want the time of the change")
		}
		if state.FirstLine != "nameserver 100.100.100.100" {
			t.Errorf("State FirstLine = %q, want %q", state.FirstLine, "nameserver 100.100.100.100")
		}
	})
}
