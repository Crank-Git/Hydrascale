package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/daemon"
)

// mountinfoWithEtcOverlay is verbatim from docs/dns-investigation.md:192. It is one line
// of /proc/self/mountinfo on the test host while the overlay mount holds.
const mountinfoWithEtcOverlay = `24 30 0:22 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw
4045 3693 0:74 / /etc rw,relatime - overlay overlay rw,lowerdir=/etc,upperdir=/var/lib/hydrascale/state/jbones/etc-upper,workdir=/var/lib/hydrascale/state/jbones/etc-work,uuid=on,nouserxattr
`

// mountinfoWithoutEtcOverlay holds an overlay mount that is not on /etc, and an /etc
// mount that is not an overlay. Neither line satisfies the verification.
const mountinfoWithoutEtcOverlay = `24 30 0:22 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw
30 1 259:2 /etc /etc rw,relatime - ext4 /dev/nvme0n1p2 rw
41 30 0:52 / /var/lib/docker/overlay2/m rw,relatime - overlay overlay rw,lowerdir=/a,upperdir=/b,workdir=/c
`

// mountinfoWithOptionalField carries a propagation optional field before the separator,
// which moves the filesystem type away from a fixed index.
const mountinfoWithOptionalField = `4045 3693 0:74 / /etc rw,relatime shared:1 master:2 - overlay overlay rw,lowerdir=/etc
`

func testOptions(t *testing.T) (nsDaemonOptions, *[]string) {
	t.Helper()
	dir := t.TempDir()
	var started []string
	opts := nsDaemonOptions{
		upper:           filepath.Join(dir, "etc-upper"),
		work:            filepath.Join(dir, "etc-work"),
		unprotectedFile: filepath.Join(dir, "dns-unprotected"),
		execChild: func(args []string) error {
			started = args
			return nil
		},
	}
	return opts, &started
}

func TestNsDaemon_exits_non_zero_when_the_mount_verification_fails(t *testing.T) {
	opts, started := testOptions(t)
	opts.mountEtc = func(upper, work string) error {
		return errors.New("invalid argument")
	}

	err := runNsDaemon(opts, []string{"tailscaled", "--state=/tmp/s"})
	if err == nil {
		t.Fatal("runNsDaemon returned no error, want a non-nil error so the child exits non-zero")
	}
	if !strings.Contains(err.Error(), "invalid argument") {
		t.Errorf("error = %q, want it to hold the mount error text", err)
	}
	if !strings.Contains(err.Error(), "dns.allow_unprotected") {
		t.Errorf("error = %q, want it to name dns.allow_unprotected", err)
	}
	if *started != nil {
		t.Errorf("the child started with %v, want no child", *started)
	}

	rec, ok := daemon.ReadUnprotected(opts.unprotectedFile)
	if !ok {
		t.Fatalf("no record at %s, want the mount error text", opts.unprotectedFile)
	}
	if !strings.Contains(rec.Reason, "invalid argument") {
		t.Errorf("record reason = %q, want it to hold the mount error text", rec.Reason)
	}
	if rec.Allowed {
		t.Error("record allowed = true, want false")
	}
}

func TestNsDaemon_starts_the_child_when_the_overlay_mount_holds(t *testing.T) {
	opts, started := testOptions(t)
	opts.mountEtc = func(upper, work string) error { return nil }

	args := []string{"tailscaled", "--state=/tmp/s"}
	if err := runNsDaemon(opts, args); err != nil {
		t.Fatalf("runNsDaemon: %v", err)
	}
	if len(*started) != 2 || (*started)[0] != "tailscaled" {
		t.Errorf("the child started with %v, want %v", *started, args)
	}
	if _, ok := daemon.ReadUnprotected(opts.unprotectedFile); ok {
		t.Error("a record exists, want none when the overlay mount holds")
	}
}

func TestNsDaemon_starts_the_child_when_allow_unprotected_is_true(t *testing.T) {
	opts, started := testOptions(t)
	opts.allowUnprotected = true
	opts.mountEtc = func(upper, work string) error {
		return errors.New("no such device")
	}

	if err := runNsDaemon(opts, []string{"tailscaled"}); err != nil {
		t.Fatalf("runNsDaemon: %v", err)
	}
	if len(*started) != 1 {
		t.Errorf("the child started with %v, want one argument", *started)
	}
	rec, ok := daemon.ReadUnprotected(opts.unprotectedFile)
	if !ok {
		t.Fatalf("no record at %s, want the mount error text", opts.unprotectedFile)
	}
	if !strings.Contains(rec.Reason, "no such device") {
		t.Errorf("record reason = %q, want it to hold the mount error text", rec.Reason)
	}
	if !rec.Allowed {
		t.Error("record allowed = false, want true")
	}
}

func TestNsDaemon_starts_the_child_when_no_overlay_directory_is_given(t *testing.T) {
	opts, started := testOptions(t)
	opts.upper = ""
	opts.work = ""
	opts.mountEtc = func(upper, work string) error {
		t.Error("mountEtc ran, want no mount without an upper directory")
		return nil
	}

	if err := runNsDaemon(opts, []string{"tailscaled"}); err != nil {
		t.Fatalf("runNsDaemon: %v", err)
	}
	if len(*started) != 1 {
		t.Errorf("the child started with %v, want one argument", *started)
	}
}

func TestNsDaemon_reports_the_record_write_failure(t *testing.T) {
	opts, _ := testOptions(t)
	// A file cannot hold a directory, so the write fails.
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	opts.unprotectedFile = filepath.Join(blocked, "dns-unprotected")
	opts.allowUnprotected = true
	opts.mountEtc = func(upper, work string) error { return errors.New("no such device") }

	err := runNsDaemon(opts, []string{"tailscaled"})
	if err == nil {
		t.Fatal("runNsDaemon returned no error, want the record write failure")
	}
}

func TestHasEtcOverlay_finds_an_overlay_mount_on_etc(t *testing.T) {
	if !hasEtcOverlay(strings.NewReader(mountinfoWithEtcOverlay)) {
		t.Error("hasEtcOverlay = false, want true")
	}
}

func TestHasEtcOverlay_rejects_an_overlay_mount_on_another_mount_point(t *testing.T) {
	if hasEtcOverlay(strings.NewReader(mountinfoWithoutEtcOverlay)) {
		t.Error("hasEtcOverlay = true, want false")
	}
}

func TestHasEtcOverlay_reads_past_an_optional_field(t *testing.T) {
	if !hasEtcOverlay(strings.NewReader(mountinfoWithOptionalField)) {
		t.Error("hasEtcOverlay = false, want true")
	}
}

func TestHasEtcOverlay_rejects_an_empty_stream(t *testing.T) {
	if hasEtcOverlay(strings.NewReader("")) {
		t.Error("hasEtcOverlay = true, want false")
	}
}
