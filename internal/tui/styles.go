package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))  // cyan
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")) // light gray
	healthyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))             // green
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))            // red
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))            // orange
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))            // dim gray
	statusBar    = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252")).Padding(0, 1)

	// selectedRow highlights the cursor row in the tailnet table.
	selectedRow = lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("255"))

	// inputLabel styles form field labels.
	inputLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)

	// confirmStyle styles the confirmation prompt box.
	confirmStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("214")).
			Padding(1, 2)

	// successStyle briefly shows operation success.
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))

	// cursorStyle marks the selected row indicator.
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
)
