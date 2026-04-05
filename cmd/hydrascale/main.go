package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"hydrascale/internal/api"
	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/dns"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/reconciler"
	"hydrascale/internal/routing"
	"hydrascale/internal/tui"
)

var cfgFile string

func main() {
	var rootCmd = &cobra.Command{
		Use:   "hydrascale",
		Short: "Hydrascale - Run multiple Tailscale tailnets simultaneously",
		Long: `Hydrascale is a Linux-only Go service that lets a single user run
multiple Tailscale tailnets simultaneously by using network namespaces for isolation.

Declare your desired state in a YAML config file and Hydrascale continuously
reconciles toward it. GitOps for tailnets.`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is "+config.DefaultConfigPath+")")

	rootCmd.AddCommand(addCmd())
	rootCmd.AddCommand(removeCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(switchCmd())
	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(diffCmd())
	rootCmd.AddCommand(applyCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(execCmd())
	rootCmd.AddCommand(pingCmd())
	rootCmd.AddCommand(sshCmd())
	rootCmd.AddCommand(tailscaleCmd())
	rootCmd.AddCommand(tuiCmd())
	rootCmd.AddCommand(wrapCmd())
	rootCmd.AddCommand(envCmd())
	rootCmd.AddCommand(installCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func configPath() string {
	if cfgFile != "" {
		return cfgFile
	}
	return config.DefaultConfigPath
}

func loadConfig() (*config.Config, error) {
	path := configPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return config.DefaultConfig(), nil
	}
	return config.LoadConfig(path)
}

func newReconciler() *reconciler.Reconciler {
	ns := namespaces.NewRealManager()
	dm := daemon.NewRealManager()
	rt := routing.NewRealManager()
	return reconciler.New(configPath(), ns, dm, rt, 10*time.Second)
}

// --- Declarative commands ---

func diffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "Show what would change without applying",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := newReconciler()
			desired, err := r.DesiredState()
			if err != nil {
				return err
			}
			actual, err := r.ActualState()
			if err != nil {
				return err
			}
			actions := r.Diff(desired, actual)

			if len(actions) == 0 {
				fmt.Println("No changes needed. Desired state matches actual state.")
				return nil
			}

			fmt.Printf("%d action(s) needed:\n", len(actions))
			for _, a := range actions {
				fmt.Printf("  %s\n", a)
			}
			return nil
		},
	}
}

func applyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply config changes (one-shot reconciliation)",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			if dryRun {
				// Dry run: show diff without applying
				r := newReconciler()
				desired, err := r.DesiredState()
				if err != nil {
					return err
				}
				actual, err := r.ActualState()
				if err != nil {
					return err
				}
				actions := r.Diff(desired, actual)

				if len(actions) == 0 {
					fmt.Println("No changes needed (dry run).")
					return nil
				}

				fmt.Printf("%d action(s) would be taken (dry run):\n", len(actions))
				for _, a := range actions {
					fmt.Printf("  %s\n", a)
				}
				return nil
			}

			r := newReconciler()
			r.ResetAllErrors()
			return r.Reconcile()
		},
	}
	cmd.Flags().Bool("dry-run", false, "Show planned actions without executing")
	return cmd
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show desired vs actual state for all tailnets",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Try live data from running daemon first
			client := api.NewClient(api.DefaultSocketPath)
			if client.IsAvailable() {
				resp, err := client.Status()
				if err == nil {
					printStatusTable(resp.Desired, resp.Actual, resp.ErrorStates, resp.LastErrors, resp.PausedStates)
					return nil
				}
				// Fall through to standalone mode if API call fails
			}

			// Standalone mode: read config and inspect live state directly
			r := newReconciler()
			desired, err := r.DesiredState()
			if err != nil {
				return err
			}
			actual, err := r.ActualState()
			if err != nil {
				return err
			}

			printStatusTable(desired, actual, r.ErrorStates(), r.LastErrors(), r.PausedStates())
			return nil
		},
	}
}

func printStatusTable(
	desired map[string]config.Tailnet,
	actual map[string]*reconciler.TailnetState,
	errorStates map[string]bool,
	lastErrors map[string]string,
	pausedStates map[string]bool,
) {
	if len(desired) == 0 && len(actual) == 0 {
		fmt.Println("No tailnets configured.")
		return
	}

	fmt.Println("Tailnet Status:")
	fmt.Printf("  %-20s %-15s %-10s %-12s %s\n", "ID", "NAMESPACE", "DAEMON", "STATE", "ERROR")
	fmt.Printf("  %-20s %-15s %-10s %-12s %s\n", "----", "---------", "------", "-----", "-----")

	for id := range desired {
		nsName := namespaces.GetNamespaceName(id)
		daemonStatus := "unknown"
		state := "desired"

		if s, ok := actual[id]; ok {
			if s.DaemonHealthy {
				daemonStatus = "healthy"
				state = "running"
			} else {
				daemonStatus = "down"
				state = "degraded"
			}
		} else {
			daemonStatus = "absent"
			state = "pending"
		}

		if errorStates[id] {
			state = "ERROR"
		}

		if pausedStates[id] {
			state = "paused"
			daemonStatus = "stopped"
		}

		errMsg := ""
		if le, ok := lastErrors[id]; ok && le != "" {
			errMsg = le
		}

		fmt.Printf("  %-20s %-15s %-10s %-12s %s\n", id, nsName, daemonStatus, state, errMsg)
	}

	// Show extra tailnets not in config
	for id, s := range actual {
		if _, wanted := desired[id]; !wanted {
			fmt.Printf("  %-20s %-15s %-10s %-12s\n", id, s.NsName, "orphan", "removing")
		}
	}
}

// --- Imperative commands (wrappers around config edit + reconcile) ---

func addCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <id>",
		Short: "Add a new tailnet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tailnetID := args[0]
			path := configPath()

			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Check for duplicates
			for _, tn := range cfg.Tailnets {
				if tn.ID == tailnetID {
					return fmt.Errorf("tailnet %s already exists", tailnetID)
				}
			}

			cfg.Tailnets = append(cfg.Tailnets, config.Tailnet{ID: tailnetID})
			if err := config.SaveConfig(path, cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("Added tailnet %s to config. Reconciling...\n", tailnetID)
			r := newReconciler()
			return r.Reconcile()
		},
	}
}

func removeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a tailnet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tailnetID := args[0]
			path := configPath()

			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			found := false
			for i, tn := range cfg.Tailnets {
				if tn.ID == tailnetID {
					cfg.Tailnets = append(cfg.Tailnets[:i], cfg.Tailnets[i+1:]...)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("tailnet %s not found", tailnetID)
			}

			if err := config.SaveConfig(path, cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("Removed tailnet %s from config. Reconciling...\n", tailnetID)
			r := newReconciler()
			return r.Reconcile()
		},
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured tailnets",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			if len(cfg.Tailnets) == 0 {
				fmt.Println("No tailnets configured.")
				return nil
			}

			fmt.Println("Configured tailnets:")
			for _, tn := range cfg.Tailnets {
				extra := ""
				if tn.ExitNode != "" {
					extra = fmt.Sprintf(" (exit: %s)", tn.ExitNode)
				}
				fmt.Printf("  - %s%s\n", tn.ID, extra)
			}
			return nil
		},
	}
}

func switchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <id>",
		Short: "Switch default namespace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tailnetID := args[0]
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			found := false
			for _, tn := range cfg.Tailnets {
				if tn.ID == tailnetID {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("tailnet %s not found", tailnetID)
			}

			nsName := namespaces.GetNamespaceName(tailnetID)
			fmt.Printf("Switched to tailnet %s (namespace: %s)\n", tailnetID, nsName)
			return nil
		},
	}
}

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start Hydrascale in daemon mode (continuous reconciliation)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Start DNS forwarder (retained for graceful shutdown)
			bindAddr := cfg.Resolver.BindAddress
			if bindAddr == "" {
				bindAddr = dns.DefaultBindAddress
			}
			forwarder, fwdErr := dns.NewForwarder(nil, 5*time.Second, bindAddr)
			if fwdErr != nil {
				fmt.Fprintf(os.Stderr, "DNS forwarder init warning: %v (starting without DNS)\n", fwdErr)
			} else {
				if err := forwarder.Start(); err != nil {
					fmt.Fprintf(os.Stderr, "DNS forwarder start error: %v\n", err)
				}
			}

			// Set up context with signal handling
			ctx, cancel := context.WithCancel(context.Background())

			// SIGINT/SIGTERM for shutdown
			stopChan := make(chan os.Signal, 1)
			signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

			// SIGHUP for config reload
			reloadChan := make(chan os.Signal, 1)
			signal.Notify(reloadChan, syscall.SIGHUP)

			interval := cfg.Reconciler.Interval
			if interval == 0 {
				interval = 10 * time.Second
			}

			fmt.Printf("Hydrascale daemon starting (reconcile every %s)...\n", interval)

			r := reconciler.New(
				configPath(),
				namespaces.NewRealManager(),
				daemon.NewRealManager(),
				routing.NewRealManager(),
				interval,
			)

			// Set up JSON event logging if configured
			if cfg.EventLog != "" {
				if err := r.SetEventLog(cfg.EventLog); err != nil {
					fmt.Fprintf(os.Stderr, "Event log warning: %v (continuing without file logging)\n", err)
				}
			}

			// Start API server
			apiServer := api.NewServer(api.DefaultSocketPath, r)
			go func() {
				if err := apiServer.Start(); err != nil {
					fmt.Fprintf(os.Stderr, "API server start error: %v\n", err)
				}
			}()

			// Handle SIGHUP in a goroutine
			go func() {
				for range reloadChan {
					fmt.Println("Config reload triggered by SIGHUP")
					if err := r.Reconcile(); err != nil {
						fmt.Fprintf(os.Stderr, "SIGHUP reconcile error: %v\n", err)
					}
				}
			}()

			// Handle stop signals
			go func() {
				<-stopChan
				fmt.Println("\nShutting down Hydrascale...")
				cancel()
			}()

			if err := r.Loop(ctx); err != nil {
				return err
			}

			// Graceful shutdown: stop all daemons
			fmt.Println("Stopping all tailnet daemons...")
			if err := r.Shutdown(); err != nil {
				fmt.Fprintf(os.Stderr, "Shutdown warning: %v\n", err)
			}

			// Stop API server
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := apiServer.Shutdown(shutdownCtx); err != nil {
				fmt.Fprintf(os.Stderr, "API server shutdown warning: %v\n", err)
			}

			// Stop DNS forwarder
			if forwarder != nil {
				forwarder.Stop()
			}

			// Close event log
			r.Close()

			fmt.Println("Hydrascale stopped.")
			return nil
		},
	}
}

// --- Namespace execution helpers ---

// runInNamespace runs an arbitrary command inside the network namespace that
// belongs to tailnetID.  When passthrough is true the child process inherits
// stdin/stdout/stderr from the current process.
func runInNamespace(tailnetID string, args []string, passthrough bool) error {
	nsName := namespaces.GetNamespaceName(tailnetID)
	cmdArgs := append([]string{"netns", "exec", nsName}, args...)
	c := exec.Command("ip", cmdArgs...)
	if passthrough {
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	}
	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

// runTailscaleInNamespace runs a tailscale sub-command inside the network
// namespace for tailnetID, pointing it at the correct per-tailnet socket.
func runTailscaleInNamespace(tailnetID string, tsArgs []string) error {
	socketPath := daemon.SocketPath(tailnetID)
	args := append([]string{"tailscale", "--socket=" + socketPath}, tsArgs...)
	return runInNamespace(tailnetID, args, true)
}

// --- Passthrough commands ---

func execCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exec <tailnet-id> -- <command...>",
		Short: "Run a command inside a tailnet's network namespace",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("exec requires a tailnet-id")
			}
			tailnetID := args[0]
			dashIdx := cmd.ArgsLenAtDash()
			if dashIdx < 0 {
				return fmt.Errorf("exec requires a -- separator before the command")
			}
			cmdArgs := args[dashIdx:]
			if len(cmdArgs) == 0 {
				return fmt.Errorf("exec requires a command after --")
			}
			return runInNamespace(tailnetID, cmdArgs, true)
		},
	}
}

func pingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping <tailnet-id> <target>",
		Short: "Ping a Tailscale peer from within a tailnet's namespace",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTailscaleInNamespace(args[0], append([]string{"ping"}, args[1:]...))
		},
	}
}

func sshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ssh <tailnet-id> <target>",
		Short: "SSH to a Tailscale peer via a tailnet's namespace",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTailscaleInNamespace(args[0], append([]string{"ssh"}, args[1:]...))
		},
	}
}

func tailscaleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tailscale <tailnet-id> -- <args...>",
		Short: "Run an arbitrary tailscale command inside a tailnet's namespace",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("tailscale requires a tailnet-id")
			}
			tailnetID := args[0]
			dashIdx := cmd.ArgsLenAtDash()
			if dashIdx < 0 {
				return fmt.Errorf("tailscale requires a -- separator before the arguments")
			}
			tsArgs := args[dashIdx:]
			return runTailscaleInNamespace(tailnetID, tsArgs)
		},
	}
}

func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the monitoring TUI (requires running daemon)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run(api.DefaultSocketPath)
		},
	}
}

func wrapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wrap <service-name> <tailnet-id>",
		Short: "Generate a systemd drop-in to run a service inside a tailnet namespace",
		Long: `Generate a systemd drop-in override that runs an existing service inside
a Hydrascale network namespace. The service will have access to the tailnet's
network and DNS.

Example:
  hydrascale wrap nginx personal
  hydrascale wrap my-app work --apply

This creates /etc/systemd/system/<service>.service.d/hydrascale.conf`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceName := args[0]
			tailnetID := args[1]
			apply, _ := cmd.Flags().GetBool("apply")

			nsName := namespaces.GetNamespaceName(tailnetID)
			socketPath := daemon.SocketPath(tailnetID)

			dropin := fmt.Sprintf(`[Unit]
After=hydrascale.service
Requires=hydrascale.service

[Service]
# Run this service inside the Hydrascale namespace for tailnet %q
ExecStart=
ExecStart=/usr/bin/ip netns exec %s ${ORIG_EXEC_START}
Environment=TAILSCALE_SOCKET=%s
Environment=HYDRASCALE_TAILNET=%s
Environment=HYDRASCALE_NAMESPACE=%s
`, tailnetID, nsName, socketPath, tailnetID, nsName)

			if !apply {
				fmt.Printf("# Drop-in for %s.service → tailnet %s (namespace %s)\n", serviceName, tailnetID, nsName)
				fmt.Printf("# Save to: /etc/systemd/system/%s.service.d/hydrascale.conf\n", serviceName)
				fmt.Printf("# Or re-run with --apply to install automatically.\n")
				fmt.Printf("#\n")
				fmt.Printf("# NOTE: After installing, update ExecStart= in the drop-in to match\n")
				fmt.Printf("# your service's actual ExecStart command prefixed with:\n")
				fmt.Printf("#   /usr/bin/ip netns exec %s <original-command>\n\n", nsName)
				fmt.Print(dropin)
				return nil
			}

			dropinDir := fmt.Sprintf("/etc/systemd/system/%s.service.d", serviceName)
			dropinPath := fmt.Sprintf("%s/hydrascale.conf", dropinDir)

			if err := os.MkdirAll(dropinDir, 0755); err != nil {
				return fmt.Errorf("failed to create drop-in directory: %w", err)
			}
			if err := os.WriteFile(dropinPath, []byte(dropin), 0644); err != nil {
				return fmt.Errorf("failed to write drop-in: %w", err)
			}

			fmt.Printf("Installed drop-in: %s\n", dropinPath)
			fmt.Println("NOTE: Edit the ExecStart= line to match your service's command.")
			fmt.Println("Then run: sudo systemctl daemon-reload && sudo systemctl restart", serviceName)
			return nil
		},
	}
	cmd.Flags().Bool("apply", false, "Install the drop-in file directly (requires root)")
	return cmd
}

func envCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "env <tailnet-id>",
		Short: "Print shell environment for running commands in a tailnet namespace",
		Long: `Print shell commands that configure the environment for a tailnet namespace.
Use with eval to set up your shell:

  eval $(hydrascale env personal)
  curl http://my-tailscale-host:8080

Or use with any command:
  hydrascale exec personal -- curl http://my-tailscale-host:8080`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tailnetID := args[0]
			nsName := namespaces.GetNamespaceName(tailnetID)
			socketPath := daemon.SocketPath(tailnetID)

			fmt.Printf("export HYDRASCALE_TAILNET=%s\n", tailnetID)
			fmt.Printf("export HYDRASCALE_NAMESPACE=%s\n", nsName)
			fmt.Printf("export TAILSCALE_SOCKET=%s\n", socketPath)
			fmt.Printf("# Run commands in this namespace with:\n")
			fmt.Printf("#   sudo ip netns exec %s <command>\n", nsName)
			fmt.Printf("# Or use: hydrascale exec %s -- <command>\n", tailnetID)
			return nil
		},
	}
}

func installCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Hydrascale as a systemd service",
		Long: `Set up Hydrascale for running as a system service:
  - Creates required directories (/etc/hydrascale, /var/lib/hydrascale, /var/log/hydrascale)
  - Copies the binary to /usr/local/bin/ (if not already there)
  - Installs the systemd unit file
  - Copies example config if none exists

Run with --dry-run to see what would be done without making changes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			steps := []struct {
				desc string
				fn   func() error
			}{
				{
					desc: "Create /etc/hydrascale",
					fn:   func() error { return os.MkdirAll("/etc/hydrascale", 0755) },
				},
				{
					desc: "Create /var/lib/hydrascale/state",
					fn:   func() error { return os.MkdirAll("/var/lib/hydrascale/state", 0750) },
				},
				{
					desc: "Create /var/log/hydrascale",
					fn:   func() error { return os.MkdirAll("/var/log/hydrascale", 0750) },
				},
				{
					desc: "Install binary to /usr/local/bin/hydrascale",
					fn: func() error {
						self, err := os.Executable()
						if err != nil {
							return fmt.Errorf("cannot find own binary: %w", err)
						}
						if self == "/usr/local/bin/hydrascale" {
							fmt.Println("  (already in place)")
							return nil
						}
						data, err := os.ReadFile(self)
						if err != nil {
							return err
						}
						return os.WriteFile("/usr/local/bin/hydrascale", data, 0755)
					},
				},
				{
					desc: "Install systemd unit",
					fn: func() error {
						unit := `[Unit]
Description=Hydrascale - Multi-Tailnet Manager
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/hydrascale serve --config /etc/hydrascale/config.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5s
WorkingDirectory=/var/lib/hydrascale
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN
CapabilityBoundingSet=CAP_NET_ADMIN CAP_SYS_ADMIN
NoNewPrivileges=yes
ProtectSystem=strict
ReadWritePaths=/var/lib/hydrascale /run/netns /etc/hydrascale /var/log/hydrascale /run/tailscale
ProtectHome=yes
StandardOutput=journal
StandardError=journal
SyslogIdentifier=hydrascale

[Install]
WantedBy=multi-user.target
`
						return os.WriteFile("/etc/systemd/system/hydrascale.service", []byte(unit), 0644)
					},
				},
				{
					desc: "Copy example config to /etc/hydrascale/config.yaml (if absent)",
					fn: func() error {
						dst := "/etc/hydrascale/config.yaml"
						if _, err := os.Stat(dst); err == nil {
							fmt.Println("  (config already exists, skipping)")
							return nil
						}
						// Write a minimal starter config
						starter := `# Hydrascale configuration
# See: hydrascale --help and contrib/example-config.yaml for full options.
version: 2

tailnets: []

resolver:
  mode: unified
  bind_address: "127.0.0.53:5354"

reconciler:
  interval: 10s
`
						return os.WriteFile(dst, []byte(starter), 0640)
					},
				},
			}

			for _, step := range steps {
				if dryRun {
					fmt.Printf("[dry-run] %s\n", step.desc)
				} else {
					fmt.Printf("%s... ", step.desc)
					if err := step.fn(); err != nil {
						fmt.Printf("FAILED: %v\n", err)
						return err
					}
					fmt.Println("OK")
				}
			}

			if !dryRun {
				fmt.Println()
				fmt.Println("Installation complete. Next steps:")
				fmt.Println("  1. Edit /etc/hydrascale/config.yaml with your tailnets")
				fmt.Println("  2. sudo systemctl daemon-reload")
				fmt.Println("  3. sudo systemctl enable --now hydrascale")
				fmt.Println("  4. sudo hydrascale tui  (to monitor)")
			}

			return nil
		},
	}
	cmd.Flags().Bool("dry-run", false, "Show what would be done without making changes")
	return cmd
}
