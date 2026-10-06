package reconciler

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"hydrascale/internal/access"
	"hydrascale/internal/config"
	"hydrascale/internal/namespaces"
)

// The modes of the IPv6 path. The operator decided both on 2026-10-05. See issue #406.
const (
	// ipv6ForceForwarding sets force_forwarding on the upstream devices and on each host side veth
	// device. The kernel holds the key from Linux 6.17, and the daemon needs no key of the
	// configuration file for it.
	ipv6ForceForwarding = "force_forwarding"
	// ipv6AllForwarding sets net.ipv6.conf.all.forwarding. The daemon uses it only when the
	// kernel holds no force_forwarding and the configuration file sets ipv6: true.
	ipv6AllForwarding = "all_forwarding"
)

// ipv6Plan holds the outcome of one IPv6 decision.
// mode is empty when the daemon opens no IPv6 path, and reason then states why.
type ipv6Plan struct {
	mode     string
	reason   string
	host     namespaces.IPv6Host
	compiled access.Compiled
}

// planIPv6 reads the host and returns the IPv6 decision and the compiled IPv6 chains.
// planIPv6 returns an empty plan for a Reconciler that drives no live host. It returns an
// error when a host read fails or when the compile fails.
func (r *Reconciler) planIPv6(in accessInput) (ipv6Plan, error) {
	r.mu.Lock()
	host6, writer6, forced := r.ipv6, r.access6, r.forcedUpstreams
	r.mu.Unlock()
	if host6 == nil || writer6 == nil {
		return ipv6Plan{}, nil
	}

	h, err := host6.ReadIPv6Host()
	if err != nil {
		return ipv6Plan{}, err
	}
	plan := ipv6Plan{host: h}

	switch {
	case len(h.Upstreams) == 0:
		plan.reason = "the host holds no IPv6 default route"
		return plan, nil
	case h.ForceForwarding:
		plan.mode = ipv6ForceForwarding
	case in.ipv6:
		plan.mode = ipv6AllForwarding
	default:
		plan.reason = "the kernel holds no force_forwarding, and the configuration file does not set ipv6: true"
		return plan, nil
	}

	topo := access.TopologyIPv6{Devices: in.devices, Ports: in.ports, HostPrefixes: h.Prefixes}
	// A host that forwards on every device already forwarded a packet from the upstream device
	// before the daemon started, so the guard would stop a path of the operator. The guard
	// also covers an earlier upstream device, because its force_forwarding stays until Shutdown.
	if plan.mode == ipv6ForceForwarding && !h.Forwarding {
		topo.GuardedUpstreams = union(forced, h.Upstreams)
	}

	tail, err := access.TailForMode(in.set.EffectiveMode())
	if err != nil {
		return ipv6Plan{}, err
	}
	plan.compiled, err = access.CompileIPv6(in.set, topo, tail)
	if err != nil {
		return ipv6Plan{}, fmt.Errorf("compile the IPv6 local rule set: %w", err)
	}
	return plan, nil
}

// applyIPv6 opens the IPv6 path of each namespace when the host and the configuration
// file allow it.
// in is the compile input of the tick, desired holds the declared tailnets, and actual
// holds the state of each namespace.
// applyIPv6 writes the IPv6 chains before it changes a forwarding key, because the chains
// hold the guard of the upstream device. It records ipv6.state when the state changes, and
// access.write_failed when a command fails.
func (r *Reconciler) applyIPv6(in accessInput, desired map[string]config.Tailnet, actual map[string]*TailnetState) {
	plan, err := r.planIPv6(in)
	if err != nil {
		r.emit("access.write_failed", "", "IPv6: "+err.Error())
		return
	}
	r.mu.Lock()
	host6, writer6 := r.ipv6, r.access6
	r.mu.Unlock()
	if host6 == nil || writer6 == nil {
		return
	}

	state := "off: " + plan.reason
	if plan.mode != "" {
		state = "on: " + plan.mode
	}
	r.mu.Lock()
	changed := r.ipv6State != state
	r.ipv6State = state
	r.mu.Unlock()
	if changed {
		r.emit("ipv6.state", "", state)
	}
	if plan.mode == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), accessTimeout)
	defer cancel()
	res, err := writer6.Apply(ctx, plan.compiled)
	if err != nil {
		r.emit("access.write_failed", "", "IPv6: "+err.Error())
		return
	}
	if res.Wrote {
		r.emit("access.written", "", fmt.Sprintf("IPv6: mode %s, %d guard rules, %d forward rules, %d out rules",
			in.set.EffectiveMode(), len(plan.compiled.Guard), len(plan.compiled.Forward), len(plan.compiled.Out)))
	}

	if !r.enableIPv6Forwarding(plan) {
		return
	}

	for id := range desired {
		ns, ok := actual[id]
		if !ok || !ns.NsExists {
			continue
		}
		written, err := host6.EnsureIPv6Path(ns.NsName, namespaces.VethIndex(ns.NsName), plan.mode == ipv6ForceForwarding)
		if err != nil {
			r.emit("access.write_failed", id, err.Error())
			continue
		}
		if len(written) > 0 {
			r.emit("access.path_repaired", id, fmt.Sprintf("wrote %d rules: %s", len(written), strings.Join(written, ", ")))
		}
	}
}

// enableIPv6Forwarding turns on the forwarding that the mode of the plan needs, and it
// returns false when a write failed.
func (r *Reconciler) enableIPv6Forwarding(plan ipv6Plan) bool {
	switch plan.mode {
	case ipv6ForceForwarding:
		changed, err := r.ipv6.EnableForceForwarding(plan.host.Upstreams)
		if err != nil {
			r.emit("access.write_failed", "", "IPv6: "+err.Error())
			return false
		}
		r.mu.Lock()
		r.forcedUpstreams = union(r.forcedUpstreams, plan.host.Upstreams)
		r.mu.Unlock()
		if len(changed) > 0 {
			r.emit("ipv6.forwarding", "", "set force_forwarding on "+strings.Join(changed, ", "))
		}
	case ipv6AllForwarding:
		if plan.host.Forwarding {
			return true
		}
		changed, err := r.ipv6.EnableAllForwarding()
		if err != nil {
			r.emit("access.write_failed", "", "IPv6: "+err.Error())
			return false
		}
		message := "set net.ipv6.conf.all.forwarding to 1"
		if len(changed) > 0 {
			message += ", and accept_ra from 1 to 2 on " + strings.Join(changed, ", ")
		}
		r.emit("ipv6.forwarding", "", message)
	}
	return true
}

// union returns the values of a, then each value of b that a does not hold.
func union(a, b []string) []string {
	out := append([]string{}, a...)
	for _, v := range b {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
