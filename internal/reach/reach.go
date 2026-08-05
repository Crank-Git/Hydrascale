// Package reach measures the reachability of one namespace.
//
// The daemon reports `healthy` for a namespace whose `tailscaled` process runs. That
// statement says nothing about the forward path, therefore issue #172 found two tailnets
// that reported `healthy` while neither namespace reached anything. A field that the
// daemon derives from the local rules repeats the same defect under a new name, because a
// rule that is present is not a packet that arrives.
//
// A Prober therefore sends one packet. It runs `ping` inside the namespace and it reports
// what came back. The result carries the target and the time of the measurement, so the
// operator reads a measurement and never a rule count.
package reach

import (
	"context"
	"time"

	"hydrascale/internal/execx"
)

// The three states that a measurement reports.
//
// StateReachable states that one packet left the namespace and an answer came back inside
// the timeout. StateUnreachable states that no answer came back. StateNotProbed states
// that the daemon sent no packet, which is the state of a tailnet that the operator
// stopped and of a namespace that does not exist.
const (
	StateReachable   = "reachable"
	StateUnreachable = "unreachable"
	StateNotProbed   = "not_probed"
)

// Timeout bounds one probe. The specification bounds the reconciler tick at 1 second, and
// the reconciler probes every namespace at the same time, therefore the whole step costs
// one timeout and not one for each namespace.
const Timeout = 250 * time.Millisecond

// Result is one measurement of the reachability of one namespace.
// Target holds the address that the daemon sent the packet to, and it is empty for
// StateNotProbed. CheckedAt holds the time of the measurement. Detail holds the reason of
// StateUnreachable and of StateNotProbed, and it is empty for StateReachable.
type Result struct {
	State     string    `json:"state"`
	Target    string    `json:"target,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	Detail    string    `json:"detail,omitempty"`
}

// Prober measures the reachability of a namespace with one packet.
type Prober struct {
	// Runner runs every command that the Prober sends to the host. A test replaces
	// Runner with an execx.Recorder and asserts the exact argument list.
	Runner execx.Runner
}

// New returns a Prober that runs each command on the host.
func New() *Prober { return &Prober{Runner: execx.OSRunner{}} }

// Probe sends one packet from nsName to target and reports what came back.
func (p *Prober) Probe(ctx context.Context, nsName, target string) Result {
	return Result{}
}

// ForgetGateway drops the default gateway that the last lookup returned.
func (p *Prober) ForgetGateway() {}
