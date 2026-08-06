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
	"fmt"
	"strings"
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

// DefaultTarget is the address that a namespace sends the packet to when the configuration
// file declares no probe_target.
//
// The address is public, therefore each namespace sends one packet to a third party on
// each tick. The measurement needs a target that the compiled rule set permits, and the
// default rule set permits a public destination alone: `HYDRASCALE-FWD` denies every
// destination in 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16, and
// 127.0.0.0/8. A target on the local network therefore reports `unreachable` on a host
// whose forward path works. Issue #172 measured the outage with this same address. An
// operator who accepts no packet to a third party declares `probe_target` with an address
// inside a tailnet.
const DefaultTarget = "1.1.1.1"

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

// runner returns the command runner. A Prober with no Runner runs on the host.
func (p *Prober) runner() execx.Runner {
	if p.Runner == nil {
		return execx.OSRunner{}
	}
	return p.Runner
}

// Probe sends one packet from nsName to target and reports what came back.
// An empty target selects DefaultTarget. The target sits beyond the host, therefore the
// packet crosses the FORWARD chain, the chain HYDRASCALE-FWD, and the masquerade rule of
// the forward path.
// ctx bounds the whole call. The caller gives ctx a deadline; Probe adds Timeout when ctx
// carries none.
// Probe returns no error. Every outcome is a Result, because a failed command is the
// measurement and not a fault of the caller.
func (p *Prober) Probe(ctx context.Context, nsName, target string) Result {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, Timeout)
		defer cancel()
	}
	if target == "" {
		target = DefaultTarget
	}

	out, err := p.runner().Run(ctx, "ip", "netns", "exec", nsName, "ping", "-n", "-c", "1", "-W", "1", target)
	if err != nil {
		return Result{
			State:     StateUnreachable,
			Target:    target,
			CheckedAt: time.Now(),
			Detail:    detail(ctx, out, err),
		}
	}
	return Result{State: StateReachable, Target: target, CheckedAt: time.Now()}
}

// detail returns the reason of a failed probe as one line, for the status response.
// A packet that gets no answer makes `ping` wait for its own deadline of 1 second, which
// is longer than Timeout, therefore the daemon stops the command and the operating system
// reports `signal: killed`. detail states the timeout instead, because the name of the
// signal says nothing to the operator.
func detail(ctx context.Context, out []byte, err error) string {
	if ctx.Err() != nil {
		return fmt.Sprintf("no answer inside %v", Timeout)
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return err.Error()
	}
	lines := strings.Split(text, "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
