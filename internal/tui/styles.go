package tui

import "github.com/charmbracelet/lipgloss"

// The brand palette. `docs/DESIGN.md` holds the token table, and
// `internal/ui/static/brand/tokens/colors.css` declares the same values for the console.
// A terminal reads no CSS file, therefore the values are copied here.
const (
	brandSurfaceRaised = "#1c1a18" // --s2
	brandTextBody      = "#eceae6" // --tx
	brandTextSecondary = "#8d867d" // --mu
	brandTextTertiary  = "#5b544c" // --dim
	brandAccent        = "#c8ff2e" // --lime
	brandStatusOK      = "#7ddc8f" // --ok
	brandStatusWarn    = "#f0a63c" // --warn
	brandStatusCrit    = "#ff5f52" // --crit
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(brandTextBody))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(brandTextBody))

	// bodyStyle carries a state word. The word stays in the body colour, because the dot
	// carries the state colour.
	bodyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(brandTextBody))

	healthyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(brandStatusOK))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(brandStatusCrit))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(brandStatusWarn))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(brandTextTertiary))
	statusBar    = lipgloss.NewStyle().
			Background(lipgloss.Color(brandSurfaceRaised)).
			Foreground(lipgloss.Color(brandTextSecondary)).
			Padding(0, 1)

	// selectedRow raises the cursor row of the tailnet table. It carries no accent,
	// because the brand allows one accent for one thing per view, and the cursor mark
	// holds it.
	selectedRow = lipgloss.NewStyle().
			Background(lipgloss.Color(brandSurfaceRaised)).
			Foreground(lipgloss.Color(brandTextBody))

	// inputLabel styles a form field label.
	inputLabel = lipgloss.NewStyle().Foreground(lipgloss.Color(brandTextBody)).Bold(true)

	// confirmStyle styles the confirmation prompt box. The border takes the tertiary text
	// colour, because a terminal draws no scrim behind the box and the structural edge
	// colour of the console does not separate the box from the page.
	confirmStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(brandTextTertiary)).
			Padding(1, 2)

	// successStyle shows the result of an operation for a short time.
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(brandStatusOK))

	// cursorStyle marks the current selection. It is the one style that carries the
	// accent.
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(brandAccent)).Bold(true)
)

// styleRegistry names every style of the terminal interface. The test
// TestTheAccentColourAppearsInOneStyleOnly reads it, therefore a new style belongs in it.
var styleRegistry = map[string]lipgloss.Style{
	"titleStyle":   titleStyle,
	"headerStyle":  headerStyle,
	"bodyStyle":    bodyStyle,
	"healthyStyle": healthyStyle,
	"errorStyle":   errorStyle,
	"warnStyle":    warnStyle,
	"dimStyle":     dimStyle,
	"statusBar":    statusBar,
	"selectedRow":  selectedRow,
	"inputLabel":   inputLabel,
	"confirmStyle": confirmStyle,
	"successStyle": successStyle,
	"cursorStyle":  cursorStyle,
}

// stateCell returns a state as a coloured dot and a lowercase word. The dot takes the
// state colour and the word takes the body colour, therefore a terminal that degrades the
// colour still shows the word.
func stateCell(dot lipgloss.Style, word string) string {
	return dot.Render("●") + " " + bodyStyle.Render(word)
}
