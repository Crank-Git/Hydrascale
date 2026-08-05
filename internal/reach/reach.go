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
	"sync"
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

	mu sync.Mutex
	// gateway holds the default gateway of the host that the last lookup returned. The
	// Prober drops the value after a failed probe, so a host that changes its gateway
	// gets a new lookup on the next tick.
	gateway string
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
// An empty target makes the Prober use the default gateway of the host, which keeps the
// packet on the local network and sends nothing to a third party. The gateway sits beyond
// the host, therefore the packet crosses the FORWARD chain and the masquerade rule of the
// forward path.
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

	address, err := p.resolve(ctx, target)
	if err != nil {
		return Result{State: StateNotProbed, CheckedAt: time.Now(), Detail: err.Error()}
	}

	out, err := p.runner().Run(ctx, "ip", "netns", "exec", nsName, "ping", "-n", "-c", "1", "-W", "1", address)
	if err != nil {
		return Result{
			State:     StateUnreachable,
			Target:    address,
			CheckedAt: time.Now(),
			Detail:    detail(out, err),
		}
	}
	return Result{State: StateReachable, Target: address, CheckedAt: time.Now()}
}

// resolve returns the address that Probe sends the packet to.
// resolve returns target when the operator declared one. For an empty target it returns
// the default gateway of the host, and it keeps that value for the next call.
func (p *Prober) resolve(ctx context.Context, target string) (string, error) {
	if target != "" {
		return target, nil
	}

	p.mu.Lock()
	cached := p.gateway
	p.mu.Unlock()
	if cached != "" {
		return cached, nil
	}

	out, err := p.runner().Run(ctx, "ip", "-4", "route", "show", "default")
	if err != nil {
		return "", fmt.Errorf("read the default route of the host: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	gateway := parseGateway(string(out))
	if gateway == "" {
		return "", fmt.Errorf("the host holds no default route, and the configuration file declares no probe_target")
	}

	p.mu.Lock()
	p.gateway = gateway
	p.mu.Unlock()
	return gateway, nil
}

// ForgetGateway drops the default gateway that the last lookup returned. The reconciler
// calls it after a failed probe, so a host that changes its gateway measures the new one.
func (p *Prober) ForgetGateway() {
	p.mu.Lock()
	p.gateway = ""
	p.mu.Unlock()
}

// parseGateway returns the address that follows `via` in the first default route.
// `ip -4 route show default` prints `default via 192.168.1.1 dev eth0 proto dhcp`. A host
// with no default route prints nothing, and a host with a link route prints no `via`.
func parseGateway(out string) string {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "via" && i+1 < len(fields) {
				return fields[i+1]
			}
		}
	}
	return ""
}

// detail returns the reason of a failed probe as one line, for the status response.
func detail(out []byte, err error) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return err.Error()
	}
	lines := strings.Split(text, "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
