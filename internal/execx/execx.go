// Package execx runs one external command through an interface that a test replaces.
//
// The daemon runs `ip`, `iptables`, and `sysctl` on the host. Code in internal/ never
// calls os/exec directly. It holds a Runner. A test replaces the Runner with a Recorder
// and asserts the exact argument list of every command that the code ran.
package execx

import (
	"context"
	"os/exec"
	"syscall"
)

// Runner runs one external command.
type Runner interface {
	// Run executes name with args and returns the combined output.
	// Run returns an error when the command fails to start or exits non-zero.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// OSRunner runs a command on the host.
type OSRunner struct{}

// Run executes name with args on the host and returns the combined output.
// Run kills the command when ctx ends.
func (OSRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// Spec describes one long-lived child process that a Starter starts.
type Spec struct {
	Name string
	Args []string
	// Env is the complete environment of the child. An empty Env gives the child an
	// empty environment; it does not give the child the environment of the daemon.
	Env []string
	// SysProcAttr holds the process attributes of the child. The daemon sets Setpgid and
	// Pdeathsig on tailscaled, so that the operating system stops an orphaned child.
	SysProcAttr *syscall.SysProcAttr
}

// Handle is one long-lived child process that a Starter started.
type Handle interface {
	// Pid returns the process identifier of the child.
	Pid() int
	// Kill stops the child immediately.
	Kill() error
	// Wait waits for the child to exit and releases its record.
	Wait() error
}

// Starter starts a long-lived child process and returns before the child exits.
//
// Runner runs a command to completion and returns its output, so Runner cannot carry a
// child that outlives the call. The daemon starts tailscaled as such a child. Starter is
// the second method for that case, and a test replaces a Starter in the same manner as it
// replaces a Runner.
//
// Starter takes no context, because the child outlives the call by design.
type Starter interface {
	Start(spec Spec) (Handle, error)
}

// OSStarter starts a child process on the host.
type OSStarter struct{}

// Start starts the child on the host and returns a Handle to it.
func (OSStarter) Start(spec Spec) (Handle, error) {
	cmd := exec.Command(spec.Name, spec.Args...)
	cmd.Env = spec.Env
	cmd.SysProcAttr = spec.SysProcAttr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return osHandle{cmd: cmd}, nil
}

// osHandle is a Handle for a child process on the host.
type osHandle struct{ cmd *exec.Cmd }

func (h osHandle) Pid() int    { return h.cmd.Process.Pid }
func (h osHandle) Kill() error { return h.cmd.Process.Kill() }
func (h osHandle) Wait() error { return h.cmd.Wait() }
