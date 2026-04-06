package hostaccess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInsertManagedBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")

	existing := "127.0.0.1  localhost\n"
	if err := os.WriteFile(path, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	v4 := map[string]string{"havoc-mars": "100.98.107.70"}
	v6 := map[string]string{"havoc-mars": "fd7a:115c:a1e0::1"}

	if err := UpdateHostsFile(path, v4, v6); err != nil {
		t.Fatalf("UpdateHostsFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(got)

	if !strings.Contains(content, hostsBeginMarker) {
		t.Error("missing begin marker")
	}
	if !strings.Contains(content, hostsEndMarker) {
		t.Error("missing end marker")
	}
	if !strings.Contains(content, "100.98.107.70  havoc-mars") {
		t.Error("missing v4 entry")
	}
	if !strings.Contains(content, "fd7a:115c:a1e0::1  havoc-mars") {
		t.Error("missing v6 entry")
	}
	if !strings.Contains(content, "127.0.0.1  localhost") {
		t.Error("original content lost")
	}
}

func TestUpdateExistingBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")

	initial := "127.0.0.1  localhost\n" +
		hostsBeginMarker + "\n" +
		"10.0.0.1  old-host\n" +
		hostsEndMarker + "\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	v4 := map[string]string{"new-host": "10.0.0.2"}
	if err := UpdateHostsFile(path, v4, nil); err != nil {
		t.Fatalf("UpdateHostsFile: %v", err)
	}

	got, _ := os.ReadFile(path)
	content := string(got)

	if strings.Contains(content, "10.0.0.1  old-host") {
		t.Error("old entry should be replaced")
	}
	if !strings.Contains(content, "10.0.0.2  new-host") {
		t.Error("new entry missing")
	}
	if !strings.Contains(content, "127.0.0.1  localhost") {
		t.Error("non-managed content lost")
	}
}

func TestRemoveManagedBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")

	initial := "127.0.0.1  localhost\n" +
		hostsBeginMarker + "\n" +
		"10.0.0.1  some-host\n" +
		hostsEndMarker + "\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	if err := UpdateHostsFile(path, nil, nil); err != nil {
		t.Fatalf("UpdateHostsFile: %v", err)
	}

	got, _ := os.ReadFile(path)
	content := string(got)

	if strings.Contains(content, hostsBeginMarker) {
		t.Error("begin marker should be removed")
	}
	if strings.Contains(content, hostsEndMarker) {
		t.Error("end marker should be removed")
	}
	if strings.Contains(content, "some-host") {
		t.Error("managed entries should be removed")
	}
	if !strings.Contains(content, "127.0.0.1  localhost") {
		t.Error("non-managed content lost")
	}
}

func TestPreserveNonManagedContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")

	initial := "# system entries\n" +
		"127.0.0.1  localhost\n" +
		"::1        localhost\n" +
		hostsBeginMarker + "\n" +
		"10.0.0.1  old-host\n" +
		hostsEndMarker + "\n" +
		"192.168.1.1  router\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	v4 := map[string]string{"new-host": "10.0.0.3"}
	if err := UpdateHostsFile(path, v4, nil); err != nil {
		t.Fatalf("UpdateHostsFile: %v", err)
	}

	got, _ := os.ReadFile(path)
	content := string(got)

	for _, want := range []string{
		"# system entries",
		"127.0.0.1  localhost",
		"::1        localhost",
		"192.168.1.1  router",
		"10.0.0.3  new-host",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing expected content: %q", want)
		}
	}
}

func TestSkipWriteWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")

	v4 := map[string]string{"havoc-mars": "100.98.107.70"}
	v6 := map[string]string{"havoc-mars": "fd7a:115c:a1e0::1"}

	// Write once to establish content
	if err := UpdateHostsFile(path, v4, v6); err != nil {
		t.Fatalf("first write: %v", err)
	}

	info1, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	mtime1 := info1.ModTime()

	// Write again with identical records
	if err := UpdateHostsFile(path, v4, v6); err != nil {
		t.Fatalf("second write: %v", err)
	}

	info2, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if info2.ModTime() != mtime1 {
		t.Error("file was rewritten when content was unchanged")
	}
}

func TestHostsFileNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent_hosts")

	v4 := map[string]string{"new-host": "10.0.0.1"}
	if err := UpdateHostsFile(path, v4, nil); err != nil {
		t.Fatalf("UpdateHostsFile on nonexistent file: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	content := string(got)

	if !strings.Contains(content, hostsBeginMarker) {
		t.Error("missing begin marker")
	}
	if !strings.Contains(content, "10.0.0.1  new-host") {
		t.Error("missing entry")
	}
}
