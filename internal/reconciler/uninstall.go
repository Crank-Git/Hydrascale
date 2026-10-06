package reconciler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Uninstall removes each namespace that the host holds, the local rule chains of both
// address families, and the force_forwarding that the daemon set.
// Uninstall reads the tailnets from the namespaces on the host and not from the
// configuration file, and it writes no configuration file. A file that does not load
// therefore still gets a full teardown, and the file of the operator stays unchanged.
// Uninstall writes no chain, because it runs no tick.
// A step that fails does not stop the remaining steps. Uninstall returns the failures
// together. See issue #417.
func (r *Reconciler) Uninstall() error {
	var errs []error

	actual, err := r.ActualState()
	if err != nil {
		errs = append(errs, err)
	}
	ids := make([]string, 0, len(actual))
	for id := range actual {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		ns := actual[id]
		if err := r.dm.Stop(ns.NsName, id); err != nil {
			errs = append(errs, fmt.Errorf("stop tailscaled of %s: %w", id, err))
		}
		if err := r.executeAction(Action{Type: ActionDeleteNS, TailnetID: id, NsName: ns.NsName}); err != nil {
			errs = append(errs, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r.mu.Lock()
	writer, writer6, host6 := r.access, r.access6, r.ipv6
	r.mu.Unlock()

	// This process set no force_forwarding, so it resets the key on each upstream device
	// that the host holds now. The daemon sets the key on those devices only.
	if host6 != nil {
		h, err := host6.ReadIPv6Host()
		if err != nil {
			errs = append(errs, err)
		} else if h.ForceForwarding && len(h.Upstreams) > 0 {
			if err := host6.DisableForceForwarding(h.Upstreams); err != nil {
				errs = append(errs, fmt.Errorf("reset IPv6 forwarding on the upstream devices: %w", err))
			}
		}
	}
	if writer6 != nil {
		if err := writer6.Teardown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("remove the IPv6 local rule chains: %w", err))
		}
	}
	if writer != nil {
		if err := writer.Teardown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("remove the local rule chains: %w", err))
		}
	}
	if r.ha != nil {
		if err := r.ha.TeardownAll(); err != nil {
			errs = append(errs, fmt.Errorf("remove the host access state: %w", err))
		}
	}
	return errors.Join(errs...)
}
