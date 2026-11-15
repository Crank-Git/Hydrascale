package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/spf13/cobra"
)

// namespaceResolvConfTarget is the file that the command of `hydrascale exec` reads.
const namespaceResolvConfTarget = "/etc/resolv.conf"

// nsExecCmd is an internal helper, not part of the user-facing CLI. `hydrascale exec`
// runs it as `ip netns exec <ns> hydrascale __nsexec --resolv-conf F -- <cmd...>`.
// ip netns exec places /etc/netns/<ns>/resolv.conf over /etc/resolv.conf, and that file
// holds the host upstreams, which resolve no name of a peer. The tailscaled of the
// namespace writes the MagicDNS resolver into the upper directory of its private /etc
// overlay, and only the mount namespace of __nsdaemon holds that overlay. __nsexec
// therefore places the file of tailscaled over /etc/resolv.conf, then executes the
// command. See issue #482.
func nsExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "__nsexec",
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dash := cmd.ArgsLenAtDash()
			if dash < 0 || dash >= len(args) {
				return fmt.Errorf("__nsexec requires '-- <command...>'")
			}
			resolvConf, _ := cmd.Flags().GetString("resolv-conf")
			return runNsExec(nsExecOptions{
				resolvConf: resolvConf,
				privateNS:  privateMountNamespace,
				stat:       os.Stat,
				bind:       bindMount,
				stderr:     os.Stderr,
				execChild:  execChild,
			}, args[dash:])
		},
	}
	cmd.Flags().String("resolv-conf", "", "resolv.conf to place over /etc/resolv.conf")
	return cmd
}

// nsExecOptions holds the inputs of runNsExec. A test replaces each function.
type nsExecOptions struct {
	resolvConf string
	privateNS  func() (bool, error)
	stat       func(string) (os.FileInfo, error)
	bind       func(source, target string) error
	stderr     io.Writer
	execChild  func(args []string) error
}

// runNsExec places resolvConf over /etc/resolv.conf, then executes the command.
// When resolvConf is absent, runNsExec writes a warning and executes the command with the
// host upstreams, because `hydrascale exec` runs without the daemon. runNsExec returns an
// error and executes no command when it cannot read the file or when the mount fails.
// runNsExec also returns an error when the process shares the mount namespace of the
// host, because the mount would then replace the host /etc/resolv.conf.
func runNsExec(o nsExecOptions, cmdArgs []string) error {
	if o.resolvConf != "" {
		private, err := o.privateNS()
		if err != nil {
			return err
		}
		if !private {
			return fmt.Errorf("__nsexec shares the mount namespace of the host; run it through ip netns exec")
		}
		_, err = o.stat(o.resolvConf)
		switch {
		case errors.Is(err, os.ErrNotExist):
			fmt.Fprintf(o.stderr, "hydrascale exec: %s is absent; the command uses the host upstreams and resolves no peer name\n", o.resolvConf)
		case err != nil:
			return fmt.Errorf("read %s: %w", o.resolvConf, err)
		default:
			if err := o.bind(o.resolvConf, namespaceResolvConfTarget); err != nil {
				return fmt.Errorf("bind %s over %s: %w", o.resolvConf, namespaceResolvConfTarget, err)
			}
		}
	}
	return o.execChild(cmdArgs)
}

// bindMount places source over target. The mount does not reach the host, because
// ip netns exec makes the mounts of the namespace a slave of the host mounts.
func bindMount(source, target string) error {
	return syscall.Mount(source, target, "", syscall.MS_BIND, "")
}

// privateMountNamespace reports whether the process holds a mount namespace that is not
// the mount namespace of PID 1. ip netns exec gives the process a new mount namespace.
// privateMountNamespace returns an error when it cannot read either link.
func privateMountNamespace() (bool, error) {
	self, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		return false, fmt.Errorf("read the mount namespace of the process: %w", err)
	}
	host, err := os.Readlink("/proc/1/ns/mnt")
	if err != nil {
		return false, fmt.Errorf("read the mount namespace of PID 1: %w", err)
	}
	return self != host, nil
}

// nsExecArgs returns the command that runs cmdArgs through __nsexec. runInNamespace runs
// it inside the namespace. self is the path of the hydrascale binary.
func nsExecArgs(self, resolvConf string, cmdArgs []string) []string {
	args := []string{self, "__nsexec", "--resolv-conf", resolvConf, "--"}
	return append(args, cmdArgs...)
}
