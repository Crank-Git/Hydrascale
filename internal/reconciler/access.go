package reconciler

import "fmt"

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
