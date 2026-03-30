package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/dns"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/routing"
	"hydrascale/internal/watcher"
)

var cfgFile string
var globalConfig *config.Config

func main() {
	var rootCmd = &cobra.Command{
		Use:   "hydrascale",
		Short: "Hydrascale - Run multiple Tailscale tailnets simultaneously",
		Long: `Hydrascale is a Linux-only Go service that lets a single user run 
multiple Tailscale tailnets simultaneously by using network namespaces for isolation.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cfgFile != "" {
				var err error
				globalConfig, err = config.LoadConfig(cfgFile)
				if err != nil {
					return err
				}
			} else {
				// Load default config
				globalConfig = config.DefaultConfig()
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is /var/lib/hydrascale/config.yaml)")

	// Commands
	rootCmd.AddCommand(addCmd())
	rootCmd.AddCommand(removeCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(switchCmd())
	rootCmd.AddCommand(serveCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func addCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <id>",
		Short: "Add a new tailnet",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			tailnetID := args[0]
			fmt.Printf("Adding tailnet: %s\n", tailnetID)
			if err := addTailnet(tailnetID); err != nil {
				fmt.Fprintf(os.Stderr, "Error adding tailnet: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

func removeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a tailnet",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			tailnetID := args[0]
			fmt.Printf("Removing tailnet: %s\n", tailnetID)
			if err := removeTailnet(tailnetID); err != nil {
				fmt.Fprintf(os.Stderr, "Error removing tailnet: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all tailnets",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Listing tailnets:")
			tailnets := listTailnets()
			if len(tailnets) == 0 {
				fmt.Println("  No tailnets configured")
				return
			}
			for _, tn := range tailnets {
				fmt.Printf("  - %s\n", tn)
			}
		},
	}
}

func switchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <id>",
		Short: "Switch default namespace",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			tailnetID := args[0]
			fmt.Printf("Switching to tailnet: %s\n", tailnetID)
			if err := switchTailnet(tailnetID); err != nil {
				fmt.Fprintf(os.Stderr, "Error switching tailnet: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start Hydrascale in daemon mode",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Starting Hydrascale daemon...")
			if err := serveDaemon(); err != nil {
				fmt.Fprintf(os.Stderr, "Error starting daemon: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

// addTailnet implements the logic for adding a new tailnet
func addTailnet(tailnetID string) error {
	// 1. Create namespace
	if err := namespaces.CreateNamespace(tailnetID); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	namespaceName := namespaces.GetNamespaceName(tailnetID)

	// 2. Launch tailscaled daemon in the namespace
	if err := daemon.StartDaemon(tailnetID, namespaceName); err != nil {
		// Clean up namespace on failure
		namespaces.DeleteNamespace(namespaceName)
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// 3. Sync routes (will be implemented in routing engine)
	if err := syncRoutesForTailnet(tailnetID, namespaceName); err != nil {
		fmt.Printf("Warning: failed to sync routes: %v\n", err)
	}

	// 4. Update DNS resolver (will be implemented in dns resolver)
	if err := updateDNSResolver(tailnetID, namespaceName); err != nil {
		fmt.Printf("Warning: failed to update DNS resolver: %v\n", err)
	}

	// 5. Add to config
	globalConfig.Tailnets = append(globalConfig.Tailnets, config.Tailnet{
		ID: tailnetID,
	})

	// 6. Save config
	if err := saveConfig(); err != nil {
		fmt.Printf("Warning: failed to save config: %v\n", err)
	}

	fmt.Printf("Tailnet %s added successfully\n", tailnetID)
	return nil
}

// removeTailnet implements the logic for removing a tailnet
func removeTailnet(tailnetID string) error {
	// 1. Find namespace name
	namespaceName := namespaces.GetNamespaceName(tailnetID)

	// 2. Stop tailscaled daemon
	if err := daemon.StopDaemon(); err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}

	// 3. Delete namespace
	if err := namespaces.DeleteNamespace(namespaceName); err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	// 4. Remove from config
	for i, tn := range globalConfig.Tailnets {
		if tn.ID == tailnetID {
			globalConfig.Tailnets = append(globalConfig.Tailnets[:i], globalConfig.Tailnets[i+1:]...)
			break
		}
	}

	// 5. Save config
	if err := saveConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Tailnet %s removed successfully\n", tailnetID)
	return nil
}

// listTailnets returns the list of configured tailnet IDs
func listTailnets() []string {
	var ids []string
	for _, tn := range globalConfig.Tailnets {
		ids = append(ids, tn.ID)
	}
	return ids
}

// switchTailnet implements switching default namespace
func switchTailnet(tailnetID string) error {
	// Check if tailnet exists
	found := false
	for _, tn := range globalConfig.Tailnets {
		if tn.ID == tailnetID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tailnet %s not found", tailnetID)
	}

	// For now, just indicate success - full implementation would set default namespace
	namespaceName := namespaces.GetNamespaceName(tailnetID)
	fmt.Printf("Switched to tailnet %s (namespace: %s)\n", tailnetID, namespaceName)
	return nil
}

// serveDaemon starts Hydrascale in daemon mode
func serveDaemon() error {
	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start DNS resolver
	dnsReady := make(chan error)
	go func() {
		if err := dns.StartResolver(globalConfig.Resolver.Mode); err != nil {
			dnsReady <- err
		}
		close(dnsReady)
	}()

	// Start watchers for each tailnet
	for _, tn := range globalConfig.Tailnets {
		namespaceName := namespaces.GetNamespaceName(tn.ID)
		go func(ns string, id string) {
			if err := watcher.WatchDaemon(ns, 5); err != nil {
				fmt.Printf("Watcher error for %s: %v\n", ns, err)
				// Attempt restart
				if err := daemon.StartDaemon(id, ns); err != nil {
					fmt.Printf("Failed to restart daemon for %s: %v\n", ns, err)
				}
			}
		}(namespaceName, tn.ID)
	}

	// Wait for DNS resolver to start
	if err := <-dnsReady; err != nil {
		return fmt.Errorf("failed to start DNS resolver: %w", err)
	}

	fmt.Println("Hydrascale daemon started. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\nShutting down Hydrascale...")

	// Stop all daemons
	for _, tn := range globalConfig.Tailnets {
		namespaceName := fmt.Sprintf("ns-%s", tn.ID)
		if err := daemon.StopDaemon(); err != nil {
			fmt.Printf("Error stopping daemon for %s: %v\n", tn.ID, err)
		}
		// Delete namespace
		if err := namespaces.DeleteNamespace(namespaceName); err != nil {
			fmt.Printf("Error deleting namespace for %s: %v\n", tn.ID, err)
		}
	}

	fmt.Println("Hydrascale stopped.")
	return nil
}

// syncRoutesForTailnet syncs routes for a specific tailnet
func syncRoutesForTailnet(tailnetID, namespaceName string) error {
	// Get routes from tailscaled
	routes, err := routing.PollStatus(namespaceName, 5)
	if err != nil {
		return fmt.Errorf("failed to get routes: %w", err)
	}

	// Sync routes using netlink
	if err := routing.SyncRoutes(namespaceName, routes); err != nil {
		return fmt.Errorf("failed to sync routes: %w", err)
	}

	return nil
}

// updateDNSResolver updates the DNS resolver with new tailnet info
func updateDNSResolver(tailnetID, namespaceName string) error {
	// This would integrate with the DNS forwarder to add the new namespace's DNS server
	// For now, we'll just return success as the DNS resolver will pick up changes dynamically
	return nil
}

// saveConfig saves the current config to file
func saveConfig() error {
	configPath := "/var/lib/hydrascale/config.yaml"
	if cfgFile != "" {
		configPath = cfgFile
	}

	data, err := yaml.Marshal(globalConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
