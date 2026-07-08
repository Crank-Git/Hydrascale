package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

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
			upper, _ := cmd.Flags().GetString("etc-upper")
			work, _ := cmd.Flags().GetString("etc-work")
			dash := cmd.ArgsLenAtDash()
			if dash < 0 || dash >= len(args) {
				return fmt.Errorf("__nsdaemon requires '-- <command...>'")
			}
			return runNsDaemon(upper, work, args[dash:])
		},
	}
	cmd.Flags().String("etc-upper", "", "overlay upperdir for /etc")
	cmd.Flags().String("etc-work", "", "overlay workdir for /etc")
	return cmd
}

// runNsDaemon mounts the per-namespace /etc overlay (best-effort) then execs the child.
func runNsDaemon(upper, work string, cmdArgs []string) error {
	if upper != "" && work != "" {
		// Fresh dirs each launch: overlay requires an empty workdir, and a stale
		// upper resolv.conf would shadow the current one until tailscaled rewrites it.
		_ = os.RemoveAll(upper)
		_ = os.RemoveAll(work)
		if err := os.MkdirAll(upper, 0755); err != nil {
			return fmt.Errorf("create overlay upperdir: %w", err)
		}
		if err := os.MkdirAll(work, 0755); err != nil {
			return fmt.Errorf("create overlay workdir: %w", err)
		}
		opts := fmt.Sprintf("lowerdir=/etc,upperdir=%s,workdir=%s", upper, work)
		if err := syscall.Mount("overlay", "/etc", "overlay", 0, opts); err != nil {
			// Don't fail the daemon over the shield; log and continue so the
			// tailnet still comes up (falls back to pre-#28 behaviour).
			fmt.Fprintf(os.Stderr, "hydrascale __nsdaemon: overlay /etc failed: %v (continuing without host-DNS shield)\n", err)
		}
	}
	bin, err := exec.LookPath(cmdArgs[0])
	if err != nil {
		return fmt.Errorf("locate %q: %w", cmdArgs[0], err)
	}
	return syscall.Exec(bin, cmdArgs, os.Environ())
}
