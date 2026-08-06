package reconciler

import (
	"fmt"
	"strings"

	"hydrascale/internal/access"
)

// InfraSubnet returns the subnet that holds every veth pair.
// A caller needs the subnet to report the address of a namespace, because the address
// comes from the subnet and the namespace name.
func (r *Reconciler) InfraSubnet() string {
	return r.infraSubnet
}

// ApplyAccess drives the host toward the rule set that the configuration file holds, and
// it records the event access.applied with the count of rules.
// count is the number of rules in the rule set that the caller wrote.
// ApplyAccess returns the error of the reconcile, and it records no event when the
// reconcile fails, because a failed reconcile applies nothing.
func (r *Reconciler) ApplyAccess(count int) error {
	if err := r.Reconcile(); err != nil {
		return err
	}
	r.emit("access.applied", "", fmt.Sprintf("%d rules", count))
	return nil
}

// AccessJumpPosition returns the position of the jump rule of the daemon in the parent
// chain, as the last tick measured it.
// parent is FORWARD or INPUT.
// AccessJumpPosition returns 0 before the first tick, and for a parent chain that held no
// jump rule on the last tick.
func (r *Reconciler) AccessJumpPosition(parent string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.jumpPositions[parent]
}

// reportJumps records the event access.jump_displaced for each jump rule of the daemon
// that another rule displaced, and for each parent chain that held no jump rule.
// placements holds one entry for each jump rule, as the chain writer measured it on this
// tick.
// reportJumps records one event for one change of position, therefore a host that keeps
// the same chain adds no event on the next tick.
func (r *Reconciler) reportJumps(placements []access.Placement) {
	for _, placement := range placements {
		r.mu.Lock()
		last, seen := r.jumpPositions[placement.Parent]
		r.jumpPositions[placement.Parent] = placement.Position
		r.mu.Unlock()

		if placement.Position == 1 || (seen && last == placement.Position) {
			continue
		}
		if placement.Position == 0 {
			r.emit("access.jump_displaced", "", fmt.Sprintf(
				"%s held no jump rule of the daemon, therefore the daemon wrote it at position 1",
				placement.Parent))
			continue
		}
		r.emit("access.jump_displaced", "", fmt.Sprintf(
			"the jump rule of %s is at position %d, below %s",
			placement.Parent, placement.Position, strings.Join(placement.Above, ", ")))
	}
}
