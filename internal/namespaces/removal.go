package namespaces

import (
	"fmt"
	"path/filepath"
	"strings"

	"hydrascale/internal/config"
)

// RemovalPlan states what the daemon does on this host when it removes one tailnet.
//
// The console dialog of FR-console-29 names every command before the operator confirms,
// therefore the daemon states them and the console repeats no rule of its own.
//
// Namespace holds the network namespace name. HostVeth holds the host side device of the
// veth pair. StateDir holds the directory that carries the node private key. RuleCount
// holds the number of iptables rules that the removal deletes, and Commands holds every
// step in the order that the removal runs it.
//
// The daemon runs the `iptables` steps and the `ip` steps as commands. It removes the
// files itself with the Go standard library, and the plan states those two steps in shell
// form, because the operator reads what changes on the host.
type RemovalPlan struct {
	Namespace string
	HostVeth  string
	StateDir  string
	RuleCount int
	Commands  []string
}

// vethTeardownRules returns the iptables rules that TeardownVeth deletes for nsName.
// vethTeardownRules returns an error when infraSubnet holds no veth addresses.
func vethTeardownRules(nsName string, infraSubnet string) ([][]string, error) {
	hostVeth, _ := VethNames(nsName)
	_, nsIP, _, _, err := VethIPs(infraSubnet, VethIndex(nsName))
	if err != nil {
		return nil, fmt.Errorf("veth IPs for %s: %w", nsName, err)
	}
	// The two FORWARD deletes stay, because a host that ran version 0.9 still holds the
	// rules that this version does not write. RemoveLegacyForwardRules removes them for a
	// namespace that keeps running.
	return [][]string{
		{"-D", "FORWARD", "-i", hostVeth, "-j", "ACCEPT"},
		{"-D", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		{"-t", "nat", "-D", "POSTROUTING", "-s", nsIP, "-j", "MASQUERADE"},
	}, nil
}

// PlanRemoval returns the plan for the tailnet with the identifier id.
//
// stateBase is the directory that holds one state directory per tailnet, which is
// daemon.DefaultStateDir on the host. infraSubnet is the subnet that carries the veth
// addresses.
//
// PlanRemoval returns an error when id is not an identifier that the configuration loader
// accepts, and when the identifier leaves stateBase. PlanRemoval runs no command.
func PlanRemoval(id string, infraSubnet string, stateBase string) (RemovalPlan, error) {
	stateDir, ok := config.SafeStateDir(stateBase, id)
	if !ok {
		return RemovalPlan{}, fmt.Errorf("id %q is not a tailnet identifier that the daemon accepts", id)
	}

	nsName := GetNamespaceName(id)
	hostVeth, _ := VethNames(nsName)

	rules, err := vethTeardownRules(nsName, infraSubnet)
	if err != nil {
		return RemovalPlan{}, err
	}

	commands := make([]string, 0, len(rules)+4)
	for _, rule := range rules {
		commands = append(commands, "iptables "+strings.Join(rule, " "))
	}
	commands = append(commands,
		"ip link del "+hostVeth,
		"ip netns del "+nsName,
		"rm -f "+filepath.Join("/etc/netns", nsName, "resolv.conf"),
		"rm -rf "+stateDir,
	)

	return RemovalPlan{
		Namespace: nsName,
		HostVeth:  hostVeth,
		StateDir:  stateDir,
		RuleCount: len(rules),
		Commands:  commands,
	}, nil
}

// teardownVethRules deletes every rule that vethTeardownRules names and returns the failed
// deletes together.
func (m *RealManager) teardownVethRules(nsName string, infraSubnet string) []error {
	rules, err := vethTeardownRules(nsName, infraSubnet)
	if err != nil {
		return []error{err}
	}
	errs := make([]error, 0, len(rules))
	for _, rule := range rules {
		errs = append(errs, m.deleteRule("iptables", rule...))
	}
	return errs
}
