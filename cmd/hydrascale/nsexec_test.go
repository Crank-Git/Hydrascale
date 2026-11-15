package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// nsExecRecord holds the calls that runNsExec made to its replaced functions.
type nsExecRecord struct {
	binds   [][2]string
	started []string
	stderr  strings.Builder
}

func nsExecTestOptions(t *testing.T, resolvConf string) (nsExecOptions, *nsExecRecord) {
	t.Helper()
	rec := &nsExecRecord{}
	return nsExecOptions{
		resolvConf: resolvConf,
		privateNS:  func() (bool, error) { return true, nil },
		stat:       os.Stat,
		bind: func(source, target string) error {
			rec.binds = append(rec.binds, [2]string{source, target})
			return nil
		},
		stderr: &rec.stderr,
		execChild: func(args []string) error {
			rec.started = args
			return nil
		},
	}, rec
}

func writeResolvConf(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 100.100.100.100\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestRunNsExec_binds_the_resolv_conf_of_tailscaled_before_the_command(t *testing.T) {
	path := writeResolvConf(t)
	opts, rec := nsExecTestOptions(t, path)
	opts.execChild = func(args []string) error {
		if len(rec.binds) != 1 {
			t.Errorf("the command started before the bind mount")
		}
		rec.started = args
		return nil
	}

	if err := runNsExec(opts, []string{"getent", "hosts", "nt-micro"}); err != nil {
		t.Fatalf("runNsExec: %v", err)
	}

	want := [][2]string{{path, "/etc/resolv.conf"}}
	if !slices.Equal(rec.binds, want) {
		t.Errorf("binds = %v, want %v", rec.binds, want)
	}
	if !slices.Equal(rec.started, []string{"getent", "hosts", "nt-micro"}) {
		t.Errorf("started = %v", rec.started)
	}
	if rec.stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", rec.stderr.String())
	}
}

func TestRunNsExec_runs_the_command_and_warns_when_the_resolv_conf_is_absent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	opts, rec := nsExecTestOptions(t, path)

	if err := runNsExec(opts, []string{"true"}); err != nil {
		t.Fatalf("runNsExec: %v", err)
	}

	if len(rec.binds) != 0 {
		t.Errorf("binds = %v, want none", rec.binds)
	}
	if !slices.Equal(rec.started, []string{"true"}) {
		t.Errorf("started = %v, want [true]", rec.started)
	}
	if !strings.Contains(rec.stderr.String(), path) {
		t.Errorf("stderr = %q, want it to name %s", rec.stderr.String(), path)
	}
}

func TestRunNsExec_runs_no_command_when_the_mount_fails(t *testing.T) {
	opts, rec := nsExecTestOptions(t, writeResolvConf(t))
	opts.bind = func(string, string) error { return errors.New("operation not permitted") }

	err := runNsExec(opts, []string{"true"})
	if err == nil || !strings.Contains(err.Error(), "operation not permitted") {
		t.Fatalf("err = %v, want the mount failure", err)
	}
	if rec.started != nil {
		t.Errorf("started = %v, want no command", rec.started)
	}
}

func TestRunNsExec_runs_no_command_when_it_cannot_read_the_resolv_conf(t *testing.T) {
	opts, rec := nsExecTestOptions(t, "/x/resolv.conf")
	opts.stat = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }

	err := runNsExec(opts, []string{"true"})
	if err == nil || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("err = %v, want the read failure", err)
	}
	if rec.started != nil || len(rec.binds) != 0 {
		t.Errorf("started = %v, binds = %v, want neither", rec.started, rec.binds)
	}
}

func TestRunNsExec_refuses_the_mount_namespace_of_the_host(t *testing.T) {
	opts, rec := nsExecTestOptions(t, writeResolvConf(t))
	opts.privateNS = func() (bool, error) { return false, nil }

	err := runNsExec(opts, []string{"true"})
	if err == nil || !strings.Contains(err.Error(), "mount namespace of the host") {
		t.Fatalf("err = %v, want the refusal", err)
	}
	if rec.started != nil || len(rec.binds) != 0 {
		t.Errorf("started = %v, binds = %v, want neither", rec.started, rec.binds)
	}
}

func TestNsExecArgs_runs_the_command_through_the_helper(t *testing.T) {
	got := nsExecArgs("/usr/local/bin/hydrascale", "/var/lib/hydrascale/state/havoc/etc-upper/resolv.conf",
		[]string{"getent", "hosts", "--", "nt-micro"})
	want := []string{
		"/usr/local/bin/hydrascale", "__nsexec",
		"--resolv-conf", "/var/lib/hydrascale/state/havoc/etc-upper/resolv.conf",
		"--", "getent", "hosts", "--", "nt-micro",
	}
	if !slices.Equal(got, want) {
		t.Errorf("nsExecArgs = %v, want %v", got, want)
	}
}
