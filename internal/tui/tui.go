package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"hydrascale/internal/api"
)

// Run starts the TUI, connecting to the API at the given socket path.
func Run(socketPath string) error {
	client := api.NewClient(socketPath)
	if !client.IsAvailable() {
		return fmt.Errorf("daemon not running (no socket at %s)\nStart with: sudo hydrascale serve", socketPath)
	}

	p := tea.NewProgram(initialModel(socketPath), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		return err
	}
	return nil
}
