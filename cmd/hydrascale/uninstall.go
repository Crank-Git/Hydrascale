package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/namespaces"

	"github.com/spf13/cobra"
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

	// 1. Stop the running service first so it doesn't fight the teardown.
	if isServiceActive("hydrascale") {
		_ = exec.Command("systemctl", "stop", "hydrascale").Run()
	}
	_ = exec.Command("systemctl", "disable", "hydrascale").Run()

	// 2. Deregister nodes (best-effort) before their namespaces go away.
	path := configPath()
	cfg, err := config.LoadConfig(path)
	if err != nil {
		cfg = config.DefaultConfig()
	}
	if !keepNodes {
		for _, tn := range cfg.Tailnets {
			logoutNamespace(tn.ID)
		}
	}

	// 3. Reconcile toward an empty desired state to tear down every namespace,
	//    veth, iptables rule, host route, and DNS entry — the same path `remove` uses.
	if len(cfg.Tailnets) > 0 {
		empty := *cfg
		empty.Tailnets = nil
		if saveErr := config.SaveConfig(path, &empty); saveErr == nil {
			if rErr := newReconciler().Reconcile(); rErr != nil {
				fmt.Printf("  teardown reconcile reported: %v\n", rErr)
			}
		}
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
