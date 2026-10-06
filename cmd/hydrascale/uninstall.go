package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/reconciler"
	"hydrascale/internal/routing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func uninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Completely tear down Hydrascale (namespaces, routes, service, config)",
		Long: `uninstall stops all tailnets, deletes their namespaces and the routes,
iptables rules, and DNS entries they created, removes the systemd service and
/var/lib/hydrascale. With --purge it also removes the binary and /etc/hydrascale.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			purge, _ := cmd.Flags().GetBool("purge")
			keepNodes, _ := cmd.Flags().GetBool("keep-nodes")
			return runUninstall(yes, purge, keepNodes)
		},
	}
	cmd.Flags().Bool("yes", false, "Do not prompt for confirmation")
	cmd.Flags().Bool("purge", false, "Also remove the binary and /etc/hydrascale")
	cmd.Flags().Bool("keep-nodes", false, "Do not deregister (log out) tailnet nodes")
	return cmd
}

func runUninstall(yes, purge, keepNodes bool) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("uninstall must run as root; try: sudo hydrascale uninstall")
	}
	p := newPrompter(os.Stdin, os.Stdout)

	fmt.Println("This will remove Hydrascale from this system:")
	fmt.Println("  - stop + disable the systemd service")
	fmt.Println("  - tear down all tailnet namespaces, veths, iptables rules, host routes, DNS entries")
	fmt.Println("  - remove /var/lib/hydrascale and /var/log/hydrascale")
	if purge {
		fmt.Println("  - remove the hydrascale binary and /etc/hydrascale (--purge)")
	}
	if !yes && !p.yesNo("Proceed?", false) {
		fmt.Println("Aborted.")
		return nil
	}

	// 1. Log each node out while its tailscaled still runs. The service stop below stops
	//    every tailscaled, so a logout after it reached no daemon. The tailnets come from
	//    the namespaces on the host, so a configuration file that does not load changes
	//    nothing here. See issue #417.
	ids := hostTailnets()
	if !keepNodes {
		for _, id := range ids {
			logoutNamespace(id)
		}
	}

	// 2. Stop the service. Its shutdown stops each tailscaled, removes the local rule
	//    chains, and resets the forwarding that it set.
	if isServiceActive("hydrascale") {
		_ = exec.Command("systemctl", "stop", "hydrascale").Run()
	}
	_ = exec.Command("systemctl", "disable", "hydrascale").Run()

	// 3. Remove each namespace, veth pair, and nat rule, and both chain families. The
	//    teardown writes no configuration file and runs no tick, so it neither destroys
	//    the file of the operator nor writes the chains again. See issue #417.
	if err := uninstallReconciler().Uninstall(); err != nil {
		fmt.Printf("\nThe teardown failed, so /var/lib/hydrascale and the service unit stay:\n%v\n", err)
		return fmt.Errorf("uninstall incomplete; correct the failures above and run it again")
	}

	// 4. Remove state, service unit, and (with --purge) binary + config.
	var removed []string
	for _, d := range []string{"/var/lib/hydrascale", "/var/log/hydrascale"} {
		if err := os.RemoveAll(d); err == nil {
			removed = append(removed, d)
		}
	}
	if err := os.Remove("/etc/systemd/system/hydrascale.service"); err == nil {
		removed = append(removed, "/etc/systemd/system/hydrascale.service")
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}
	if purge {
		for _, f := range []string{"/etc/hydrascale", "/usr/local/bin/hydrascale"} {
			if err := os.RemoveAll(f); err == nil {
				removed = append(removed, f)
			}
		}
	}

	fmt.Println("\nRemoved:")
	for _, r := range removed {
		fmt.Printf("  - %s\n", r)
	}
	fmt.Println("Hydrascale uninstalled. (If any tailnet was mid-teardown, a reboot fully clears namespaces.)")
	return nil
}

// hostTailnets returns the tailnet identifier of each namespace of the daemon that the
// host holds.
func hostTailnets() []string {
	list, err := namespaces.NewRealManager().List()
	if err != nil {
		fmt.Printf("  list the namespaces: %v\n", err)
		return nil
	}
	var ids []string
	for _, ns := range list {
		if id := namespaces.GetTailnetFromNamespace(ns); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// uninstallReconciler returns a Reconciler for the teardown. It needs the infra subnet
// alone, so it reads that key even from a configuration file that fails validation, and
// it falls back to the default subnet when the file does not parse.
func uninstallReconciler() *reconciler.Reconciler {
	infraSubnet := config.DefaultConfig().InfraSubnet
	if cfg, err := loadConfig(); err == nil {
		if cfg.InfraSubnet != "" {
			infraSubnet = cfg.InfraSubnet
		}
	} else if data, readErr := os.ReadFile(configPath()); readErr == nil {
		var raw struct {
			InfraSubnet string `yaml:"infra_subnet"`
		}
		if yaml.Unmarshal(data, &raw) == nil && raw.InfraSubnet != "" {
			infraSubnet = raw.InfraSubnet
		}
	}
	return reconciler.New(configPath(), namespaces.NewRealManager(), daemon.NewRealManager(),
		routing.NewRealManager(), 10*time.Second, nil, infraSubnet)
}

// isServiceActive reports whether a systemd unit is active.
func isServiceActive(name string) bool {
	out, _ := exec.Command("systemctl", "is-active", name).Output()
	return strings.TrimSpace(string(out)) == "active"
}

// logoutNamespace best-effort deregisters a tailnet node from its namespace.
func logoutNamespace(id string) {
	ns := namespaces.GetNamespaceName(id)
	sock := daemon.SocketPath(id)
	_ = exec.Command("ip", "netns", "exec", ns, "tailscale", "--socket="+sock, "logout").Run()
}
