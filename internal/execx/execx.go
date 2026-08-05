// Package execx runs one external command through an interface that a test replaces.
//
// The daemon runs `ip`, `iptables`, and `sysctl` on the host. Code in internal/ never
// calls os/exec directly. It holds a Runner. A test replaces the Runner with a Recorder
// and asserts the exact argument list of every command that the code ran.
package execx

import (
	"context"
	"os/exec"
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
