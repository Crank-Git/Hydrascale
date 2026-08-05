package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"hydrascale/internal/daemon"
)

// mountinfoSeparator divides the optional fields from the filesystem type in one
// /proc/self/mountinfo line. proc_pid_mountinfo(5) documents the format.
const mountinfoSeparator = " - "

// nsDaemonCmd is an internal helper, not part of the user-facing CLI. It is
// invoked as `ip netns exec <ns> hydrascale __nsdaemon --etc-upper U --etc-work W -- <cmd...>`.
// Running inside the private mount namespace that `ip netns exec` set up, it
// mounts a per-namespace overlay on /etc so that anything the child writes to
// /etc (notably tailscaled's resolv.conf, which it replaces via rename) lands
// in the namespace-local upper dir instead of the host's shared /etc. Then it
// execs the child. See issue #28.
func nsDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "__nsdaemon",
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := nsDaemonOptions{
				mountEtc:  mountEtcOverlay,
				execChild: execChild,
			}
			opts.upper, _ = cmd.Flags().GetString("etc-upper")
			opts.work, _ = cmd.Flags().GetString("etc-work")
			opts.unprotectedFile, _ = cmd.Flags().GetString("unprotected-file")
			opts.allowUnprotected, _ = cmd.Flags().GetBool("allow-unprotected")
			dash := cmd.ArgsLenAtDash()
			if dash < 0 || dash >= len(args) {
				return fmt.Errorf("__nsdaemon requires '-- <command...>'")
			}
			return runNsDaemon(opts, args[dash:])
		},
	}
	cmd.Flags().String("etc-upper", "", "overlay upperdir for /etc")
	cmd.Flags().String("etc-work", "", "overlay workdir for /etc")
	cmd.Flags().String("unprotected-file", "", "file that records an overlay mount failure")
	cmd.Flags().Bool("allow-unprotected", false, "start the child when the overlay mount fails")
	return cmd
}

// nsDaemonOptions holds the inputs of runNsDaemon.
type nsDaemonOptions struct {
	upper            string
	work             string
	unprotectedFile  string
	allowUnprotected bool
	// mountEtc places the overlay mount on /etc and verifies it. A test replaces it.
	mountEtc func(upper, work string) error
	// execChild replaces the process image with the child. A test replaces it.
	execChild func(args []string) error
}

// runNsDaemon places the overlay mount on /etc, verifies it, then executes the child.
// A namespace without the overlay mount lets tailscaled write the host /etc/resolv.conf
// file, so runNsDaemon returns an error and starts no child when the mount does not
// hold. Only dns.allow_unprotected lets the child start in that state, and runNsDaemon
// records the mount error either way. See issue #76.
func runNsDaemon(o nsDaemonOptions, cmdArgs []string) error {
	if o.upper != "" && o.work != "" {
		if mountErr := o.mountEtc(o.upper, o.work); mountErr != nil {
			reason := fmt.Sprintf("overlay /etc failed: %v", mountErr)
			writeErr := writeUnprotectedFile(o.unprotectedFile, reason, o.allowUnprotected)
			if !o.allowUnprotected {
				return errors.Join(fmt.Errorf("%s; set dns.allow_unprotected to true to start the tailnet without the overlay mount", reason), writeErr)
			}
			if writeErr != nil {
				return writeErr
			}
			fmt.Fprintf(os.Stderr, "hydrascale __nsdaemon: %s; dns.allow_unprotected is true, so the tailnet starts without the overlay mount\n", reason)
		}
	}
	return o.execChild(cmdArgs)
}

// writeUnprotectedFile records the overlay mount failure where the daemon reads it.
// The standard error stream of the child reaches no journal, so the file is the only
// record that survives. See internal/daemon/daemon.go and issue #75.
func writeUnprotectedFile(path, reason string, allowed bool) error {
	if path == "" {
		return nil
	}
	data, err := json.Marshal(daemon.UnprotectedRecord{Reason: reason, Allowed: allowed})
	if err != nil {
		return fmt.Errorf("encode the DNS protection record: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// mountEtcOverlay places the overlay mount on /etc and verifies it.
// mountEtcOverlay returns an error when it cannot prepare the upper directory or the
// work directory, when the mount call fails, and when /proc/self/mountinfo then holds no
// overlay mount on /etc.
func mountEtcOverlay(upper, work string) error {
	// A stale upper resolv.conf hides the current one until tailscaled writes its own,
	// and OverlayFS needs an empty work directory.
	if err := os.RemoveAll(upper); err != nil {
		return fmt.Errorf("remove overlay upperdir: %w", err)
	}
	if err := os.RemoveAll(work); err != nil {
		return fmt.Errorf("remove overlay workdir: %w", err)
	}
	if err := os.MkdirAll(upper, 0755); err != nil {
		return fmt.Errorf("create overlay upperdir: %w", err)
	}
	if err := os.MkdirAll(work, 0755); err != nil {
		return fmt.Errorf("create overlay workdir: %w", err)
	}
	opts := fmt.Sprintf("lowerdir=/etc,upperdir=%s,workdir=%s", upper, work)
	if err := syscall.Mount("overlay", "/etc", "overlay", 0, opts); err != nil {
		return err
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return fmt.Errorf("read /proc/self/mountinfo: %w", err)
	}
	defer f.Close()
	if !hasEtcOverlay(f) {
		return fmt.Errorf("/proc/self/mountinfo holds no overlay mount on /etc")
	}
	return nil
}

// hasEtcOverlay reports whether the mountinfo stream holds an overlay mount on /etc.
// The optional fields of a line end at the separator " - ", so the filesystem type is
// the first field after the separator and it is not at a fixed index.
func hasEtcOverlay(r io.Reader) bool {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		index := strings.Index(line, mountinfoSeparator)
		if index < 0 {
			continue
		}
		head := strings.Fields(line[:index])
		tail := strings.Fields(line[index+len(mountinfoSeparator):])
		if len(head) < 5 || len(tail) < 1 {
			continue
		}
		if head[4] == "/etc" && tail[0] == "overlay" {
			return true
		}
	}
	return false
}

// execChild replaces the process image with the child command.
func execChild(args []string) error {
	bin, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("locate %q: %w", args[0], err)
	}
	return syscall.Exec(bin, args, os.Environ())
}
