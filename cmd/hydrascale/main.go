package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/dns"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/reconciler"
	"hydrascale/internal/routing"
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
	return &cobra.Command{
		Use:   "apply",
		Short: "Apply config changes (one-shot reconciliation)",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := newReconciler()
			// Reset all error states for a fresh apply
			r.ResetAllErrors()
			return r.Reconcile()
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show desired vs actual state for all tailnets",
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
			errors := r.ErrorStates()

			if len(desired) == 0 && len(actual) == 0 {
				fmt.Println("No tailnets configured.")
				return nil
			}

			fmt.Println("Tailnet Status:")
			fmt.Printf("  %-20s %-15s %-10s %-10s\n", "ID", "NAMESPACE", "DAEMON", "STATE")
			fmt.Printf("  %-20s %-15s %-10s %-10s\n", "----", "---------", "------", "-----")

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

				if errors[id] {
					state = "ERROR"
				}

				fmt.Printf("  %-20s %-15s %-10s %-10s\n", id, nsName, daemonStatus, state)
			}

			// Show extra tailnets not in config
			for id, s := range actual {
				if _, wanted := desired[id]; !wanted {
					fmt.Printf("  %-20s %-15s %-10s %-10s\n", id, s.NsName, "orphan", "removing")
				}
			}

			return nil
		},
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

			// Start DNS resolver
			bindAddr := cfg.Resolver.BindAddress
			if bindAddr == "" {
				bindAddr = dns.DefaultBindAddress
			}
			go func() {
				if err := dns.StartResolver(cfg.Resolver.Mode, bindAddr); err != nil {
					fmt.Fprintf(os.Stderr, "DNS resolver error: %v\n", err)
				}
			}()

			// Set up context with signal handling
			ctx, cancel := context.WithCancel(context.Background())
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				<-sigChan
				fmt.Println("\nShutting down Hydrascale...")
				cancel()
			}()

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

			if err := r.Loop(ctx); err != nil {
				return err
			}

			fmt.Println("Hydrascale stopped.")
			return nil
		},
	}
}
