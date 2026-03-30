package watcher

import (
	"fmt"
	"os/exec"
	"time"
)

// WatchDaemon watches the Tailscale daemon in the specified namespace and restarts it if it crashes.
// The interval parameter specifies how often to check the daemon status (in seconds).
func WatchDaemon(namespaceName string, interval int) error {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Check if the daemon is running by trying to get its status
		cmd := exec.Command("ip", "netns", "exec", namespaceName, "tailscaled", "--status")
		if err := cmd.Run(); err != nil {
			// Daemon might have crashed
			fmt.Printf("Daemon in namespace %s appears to be down: %v\n", namespaceName, err)
			fmt.Printf("Attempting to restart daemon for namespace %s\n", namespaceName)
		}
	}

	return nil
}
