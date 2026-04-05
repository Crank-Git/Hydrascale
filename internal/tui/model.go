package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"hydrascale/internal/api"
)

// colWidths defines fixed column widths for the tailnet table.
// Columns: ID, NAMESPACE, DAEMON, STATE, ERROR
var colWidths = []int{14, 18, 9, 9, 30}

// ---- message types ----

type panel int

const (
	panelStatus panel = iota
	panelEvents
)

type viewMode int

const (
	modeNormal  viewMode = iota
	modeAdding           // add-tailnet form
	modeConfirm          // confirm destructive action
	modeDNS              // DNS settings form
)

type tickMsg time.Time
type statusMsg struct {
	status *api.StatusResponse
	err    error
}
type eventsMsg struct {
	events *api.EventsResponse
	err    error
}
type reconcileMsg struct {
	err     error
	message string
}
type mutationMsg struct {
	err     error
	message string
}
type clearStatusMsg struct{}

// ---- sub-forms ----

type addForm struct {
	inputs  []textinput.Model // [id, authKey, exitNode]
	focused int
}

type dnsForm struct {
	inputs  []textinput.Model // [mode, bindAddress]
	focused int
}

type confirmAction struct {
	action    string // "remove" or "disconnect"
	tailnetID string
}

// ---- main model ----

type model struct {
	client      *api.Client
	status      *api.StatusResponse
	events      *api.EventsResponse
	activePanel panel
	err         error
	lastUpdate  time.Time
	width       int
	height      int

	// Row selection
	cursor int

	// Mode
	mode viewMode

	// Forms
	addForm addForm
	dnsForm dnsForm

	// Confirm dialog
	confirm *confirmAction

	// Transient status message
	statusMsg     string
	statusMsgTime time.Time
}

func newAddForm() addForm {
	id := textinput.New()
	id.Placeholder = "personal"
	id.Focus()
	id.CharLimit = 64

	authKey := textinput.New()
	authKey.Placeholder = "tskey-auth-..."
	authKey.CharLimit = 256

	exitNode := textinput.New()
	exitNode.Placeholder = "100.64.0.1 (optional)"
	exitNode.CharLimit = 64

	return addForm{
		inputs:  []textinput.Model{id, authKey, exitNode},
		focused: 0,
	}
}

func newDNSForm() dnsForm {
	mode := textinput.New()
	mode.Placeholder = "off / auto / custom"
	mode.Focus()
	mode.CharLimit = 32

	bind := textinput.New()
	bind.Placeholder = "100.100.100.100:53 (optional)"
	bind.CharLimit = 64

	return dnsForm{
		inputs:  []textinput.Model{mode, bind},
		focused: 0,
	}
}

func initialModel(socketPath string) model {
	return model{
		client:      api.NewClient(socketPath),
		activePanel: panelStatus,
		mode:        modeNormal,
	}
}

// ---- commands ----

func (m model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), fetchStatus(m.client), fetchEvents(m.client))
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchStatus(c *api.Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.Status()
		return statusMsg{status: resp, err: err}
	}
}

func fetchEvents(c *api.Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.Events()
		return eventsMsg{events: resp, err: err}
	}
}

func triggerReconcile(c *api.Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.Reconcile()
		msg := ""
		if resp != nil {
			msg = resp.Message
		}
		return reconcileMsg{err: err, message: msg}
	}
}

func doAddTailnet(c *api.Client, id, authKey, exitNode string) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.AddTailnet(id, authKey, exitNode)
		if err != nil {
			return mutationMsg{err: err}
		}
		return mutationMsg{message: resp.Message}
	}
}

func doRemoveTailnet(c *api.Client, id string) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.RemoveTailnet(id)
		if err != nil {
			return mutationMsg{err: err}
		}
		return mutationMsg{message: resp.Message}
	}
}

func doConnectTailnet(c *api.Client, id string) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.ConnectTailnet(id)
		if err != nil {
			return mutationMsg{err: err}
		}
		return mutationMsg{message: resp.Message}
	}
}

func doDisconnectTailnet(c *api.Client, id string) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.DisconnectTailnet(id)
		if err != nil {
			return mutationMsg{err: err}
		}
		return mutationMsg{message: resp.Message}
	}
}

func doUpdateDNS(c *api.Client, mode, bindAddress string) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.UpdateDNS(mode, bindAddress)
		if err != nil {
			return mutationMsg{err: err}
		}
		return mutationMsg{message: resp.Message}
	}
}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

// ---- update ----

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(tickCmd(), fetchStatus(m.client), fetchEvents(m.client))

	case statusMsg:
		if msg.err == nil {
			m.status = msg.status
			m.lastUpdate = time.Now()
			m.err = nil
			// clamp cursor
			if m.status != nil && m.cursor >= len(m.status.Desired) {
				m.cursor = max(0, len(m.status.Desired)-1)
			}
		} else {
			m.err = msg.err
		}
		return m, nil

	case eventsMsg:
		if msg.err == nil {
			m.events = msg.events
		}
		return m, nil

	case reconcileMsg:
		if msg.err == nil {
			m.statusMsg = "Reconcile triggered"
			if msg.message != "" {
				m.statusMsg = msg.message
			}
		} else {
			m.statusMsg = "Reconcile failed: " + msg.err.Error()
		}
		m.statusMsgTime = time.Now()
		return m, tea.Batch(fetchStatus(m.client), fetchEvents(m.client), clearStatusAfter(3*time.Second))

	case mutationMsg:
		if msg.err != nil {
			m.statusMsg = "Error: " + msg.err.Error()
		} else {
			m.statusMsg = "Done"
			if msg.message != "" {
				m.statusMsg = msg.message
			}
		}
		m.statusMsgTime = time.Now()
		return m, tea.Batch(fetchStatus(m.client), fetchEvents(m.client), clearStatusAfter(3*time.Second))

	case clearStatusMsg:
		m.statusMsg = ""
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {

	case modeAdding:
		return m.handleAddingKey(msg)

	case modeDNS:
		return m.handleDNSKey(msg)

	case modeConfirm:
		return m.handleConfirmKey(msg)

	default: // modeNormal
		return m.handleNormalKey(msg)
	}
}

func (m model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "r":
		return m, triggerReconcile(m.client)

	case "tab":
		if m.activePanel == panelStatus {
			m.activePanel = panelEvents
		} else {
			m.activePanel = panelStatus
		}

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.status != nil && m.cursor < len(m.status.Desired)-1 {
			m.cursor++
		}

	case "a":
		m.addForm = newAddForm()
		m.mode = modeAdding

	case "d":
		if id := m.selectedID(); id != "" {
			m.confirm = &confirmAction{action: "remove", tailnetID: id}
			m.mode = modeConfirm
		}

	case "c":
		if id := m.selectedID(); id != "" {
			m.statusMsg = "Connecting " + id + "..."
			m.statusMsgTime = time.Now()
			return m, doConnectTailnet(m.client, id)
		}

	case "x":
		if id := m.selectedID(); id != "" {
			m.confirm = &confirmAction{action: "disconnect", tailnetID: id}
			m.mode = modeConfirm
		}

	case "n":
		m.dnsForm = newDNSForm()
		m.mode = modeDNS
	}

	return m, nil
}

func (m model) handleAddingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		return m, nil

	case "tab", "down":
		m.addForm.inputs[m.addForm.focused].Blur()
		m.addForm.focused = (m.addForm.focused + 1) % len(m.addForm.inputs)
		m.addForm.inputs[m.addForm.focused].Focus()
		return m, nil

	case "shift+tab", "up":
		m.addForm.inputs[m.addForm.focused].Blur()
		m.addForm.focused = (m.addForm.focused - 1 + len(m.addForm.inputs)) % len(m.addForm.inputs)
		m.addForm.inputs[m.addForm.focused].Focus()
		return m, nil

	case "enter":
		id := strings.TrimSpace(m.addForm.inputs[0].Value())
		authKey := strings.TrimSpace(m.addForm.inputs[1].Value())
		exitNode := strings.TrimSpace(m.addForm.inputs[2].Value())
		if id == "" {
			return m, nil
		}
		m.mode = modeNormal
		return m, doAddTailnet(m.client, id, authKey, exitNode)
	}

	// Forward key to focused input
	var cmd tea.Cmd
	m.addForm.inputs[m.addForm.focused], cmd = m.addForm.inputs[m.addForm.focused].Update(msg)
	return m, cmd
}

func (m model) handleDNSKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		return m, nil

	case "tab", "down":
		m.dnsForm.inputs[m.dnsForm.focused].Blur()
		m.dnsForm.focused = (m.dnsForm.focused + 1) % len(m.dnsForm.inputs)
		m.dnsForm.inputs[m.dnsForm.focused].Focus()
		return m, nil

	case "shift+tab", "up":
		m.dnsForm.inputs[m.dnsForm.focused].Blur()
		m.dnsForm.focused = (m.dnsForm.focused - 1 + len(m.dnsForm.inputs)) % len(m.dnsForm.inputs)
		m.dnsForm.inputs[m.dnsForm.focused].Focus()
		return m, nil

	case "enter":
		mode := strings.TrimSpace(m.dnsForm.inputs[0].Value())
		bind := strings.TrimSpace(m.dnsForm.inputs[1].Value())
		if mode == "" {
			return m, nil
		}
		m.mode = modeNormal
		return m, doUpdateDNS(m.client, mode, bind)
	}

	var cmd tea.Cmd
	m.dnsForm.inputs[m.dnsForm.focused], cmd = m.dnsForm.inputs[m.dnsForm.focused].Update(msg)
	return m, cmd
}

func (m model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.confirm == nil {
			m.mode = modeNormal
			return m, nil
		}
		action := m.confirm.action
		id := m.confirm.tailnetID
		m.confirm = nil
		m.mode = modeNormal

		switch action {
		case "remove":
			return m, doRemoveTailnet(m.client, id)
		case "disconnect":
			return m, doDisconnectTailnet(m.client, id)
		}

	case "n", "N", "esc":
		m.confirm = nil
		m.mode = modeNormal
	}
	return m, nil
}

// selectedID returns the tailnet ID at the current cursor position (sorted order).
func (m model) selectedID() string {
	if m.status == nil || len(m.status.Desired) == 0 {
		return ""
	}
	keys := sortedTailnetIDs(m.status)
	if m.cursor < 0 || m.cursor >= len(keys) {
		return ""
	}
	return keys[m.cursor]
}

// sortedTailnetIDs returns the Desired map keys in sorted order.
func sortedTailnetIDs(s *api.StatusResponse) []string {
	keys := make([]string, 0, len(s.Desired))
	for k := range s.Desired {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---- view ----

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	lineWidth := min(m.width, 80)

	// Build the header section (fixed)
	var header strings.Builder
	title := titleStyle.Render("HYDRASCALE")
	var connStatus string
	if m.err != nil {
		connStatus = errorStyle.Render("● Disconnected")
	} else if m.status != nil {
		ago := time.Since(m.lastUpdate).Truncate(time.Second)
		connStatus = healthyStyle.Render("● Connected") + dimStyle.Render(fmt.Sprintf("  Last: %s ago", ago))
	} else {
		connStatus = warnStyle.Render("● Connecting...")
	}
	header.WriteString(fmt.Sprintf(" %s  %s\n", title, connStatus))
	header.WriteString(strings.Repeat("─", lineWidth) + "\n")

	// Tailnets section (fixed)
	var tailnets strings.Builder
	tailnets.WriteString(" " + headerStyle.Render("Tailnets") + "\n")
	tailnets.WriteString(renderRow([]string{
		dimStyle.Render("ID"),
		dimStyle.Render("NAMESPACE"),
		dimStyle.Render("DAEMON"),
		dimStyle.Render("STATE"),
		dimStyle.Render("ERROR"),
	}, colWidths, false, false) + "\n")

	tailnetRows := 0
	if m.status != nil && len(m.status.Desired) > 0 {
		keys := sortedTailnetIDs(m.status)
		tailnetRows = len(keys)
		for i, id := range keys {
			selected := i == m.cursor

			nsName := "ns-" + id
			daemonStr := dimStyle.Render("absent")
			stateStr := dimStyle.Render("pending")
			errStr := ""

			if actual, ok := m.status.Actual[id]; ok && actual != nil {
				if actual.DaemonHealthy {
					daemonStr = healthyStyle.Render("healthy")
					stateStr = healthyStyle.Render("running")
				} else if actual.NsExists {
					daemonStr = errorStyle.Render("down")
					stateStr = warnStyle.Render("degraded")
				}
			}

			if m.status.PausedStates[id] {
				stateStr = warnStyle.Render("paused")
				daemonStr = dimStyle.Render("stopped")
			} else if m.status.ErrorStates[id] {
				stateStr = errorStyle.Render("ERROR")
			}
			if lastErr, ok := m.status.LastErrors[id]; ok && lastErr != "" {
				errStr = errorStyle.Render(truncate(lastErr, 28))
			}

			if selected {
				idPlain := id
				tailnets.WriteString(renderRow([]string{idPlain, nsName, daemonStr, stateStr, errStr}, colWidths, true, selected) + "\n")
			} else {
				tailnets.WriteString(renderRow([]string{id, nsName, daemonStr, stateStr, errStr}, colWidths, false, false) + "\n")
			}
		}
	} else if m.status != nil {
		tailnets.WriteString("  " + dimStyle.Render("No tailnets configured") + "\n")
		tailnetRows = 1
	} else {
		tailnets.WriteString("  " + dimStyle.Render("No data yet...") + "\n")
		tailnetRows = 1
	}

	tailnets.WriteString("\n" + strings.Repeat("─", lineWidth) + "\n")

	// Footer (fixed — always visible)
	footerText := "a add  d delete  c connect  x disconnect  r reconcile  n dns  tab switch  ↑↓/jk select  q quit"
	footerLine := statusBar.Render(footerText)

	// Status message (fixed — 0 or 1 line)
	var statusLine string
	statusLines := 0
	if m.statusMsg != "" {
		statusLine = " " + successStyle.Render(m.statusMsg) + "\n"
		statusLines = 1
	}

	// Calculate available height for events section.
	// Fixed lines: header(2) + tailnet header(2) + tailnet rows + separator(2) + events header(1) + separator(2) + status(0-1) + footer(1)
	fixedLines := 2 + 2 + tailnetRows + 2 + 1 + 2 + statusLines + 1
	availableForEvents := m.height - fixedLines
	if availableForEvents < 1 {
		availableForEvents = 1
	}

	// Events section (flexible — fills remaining space)
	var events strings.Builder
	events.WriteString(" " + headerStyle.Render("Events") + dimStyle.Render(fmt.Sprintf(" (last %d)", availableForEvents)) + "\n")
	if m.events != nil && len(m.events.Events) > 0 {
		eventList := m.events.Events
		start := len(eventList) - availableForEvents
		if start < 0 {
			start = 0
		}
		shown := eventList[start:]
		for _, e := range shown {
			ts := e.Time.Format("15:04:05")
			line := fmt.Sprintf("  %s [%s]", dimStyle.Render(ts), e.Type)
			if e.TailnetID != "" {
				line += " " + e.TailnetID
			}
			if e.Message != "" {
				line += ": " + e.Message
			}
			events.WriteString(truncateVisible(line, lineWidth) + "\n")
		}
		// Pad remaining lines so footer stays at bottom
		for i := len(shown); i < availableForEvents; i++ {
			events.WriteString("\n")
		}
	} else {
		events.WriteString("  " + dimStyle.Render("No events yet...") + "\n")
		for i := 1; i < availableForEvents; i++ {
			events.WriteString("\n")
		}
	}

	events.WriteString("\n" + strings.Repeat("─", lineWidth) + "\n")

	// Assemble final view
	var b strings.Builder
	b.WriteString(header.String())
	b.WriteString(tailnets.String())
	b.WriteString(events.String())
	b.WriteString(statusLine)
	b.WriteString(footerLine)

	mainView := b.String()

	// Overlay modes — shown OVER the main view, not appended below
	switch m.mode {
	case modeAdding:
		return m.overlayAddForm(mainView)
	case modeDNS:
		return m.overlayDNSForm(mainView)
	case modeConfirm:
		return m.overlayConfirm(mainView)
	}

	return mainView
}

// renderRow renders a table row with fixed column widths, correctly accounting for
// ANSI escape codes in styled cells. Selected rows get a highlight background applied.
func renderRow(cols []string, widths []int, hasIndicator bool, selected bool) string {
	var parts []string
	for i, col := range cols {
		width := 0
		if i < len(widths) {
			width = widths[i]
		}
		visible := lipgloss.Width(col)
		padding := width - visible
		if padding < 0 {
			padding = 0
		}
		cell := col + strings.Repeat(" ", padding)
		parts = append(parts, cell)
	}

	indicator := "  "
	if hasIndicator && selected {
		indicator = cursorStyle.Render("▸ ")
	}

	row := indicator + strings.Join(parts, "  ")
	if selected {
		row = selectedRow.Render(row)
	}
	return row
}

// overlayOnView replaces the middle lines of the main view with a centered
// overlay box, keeping header and footer visible.
func (m model) overlayOnView(behind string, box string) string {
	lines := strings.Split(behind, "\n")
	boxLines := strings.Split(box, "\n")
	boxHeight := len(boxLines)

	// Place the overlay in the center of the view
	startLine := (len(lines) - boxHeight) / 2
	if startLine < 2 { // keep header visible
		startLine = 2
	}
	endLine := startLine + boxHeight
	if endLine > len(lines)-1 { // keep footer visible
		endLine = len(lines) - 1
		startLine = endLine - boxHeight
		if startLine < 2 {
			startLine = 2
		}
	}

	// Replace lines with the overlay box
	result := make([]string, 0, len(lines))
	result = append(result, lines[:startLine]...)
	result = append(result, boxLines...)
	if endLine < len(lines) {
		result = append(result, lines[endLine:]...)
	}

	return strings.Join(result, "\n")
}

// overlayAddForm renders the add-tailnet form over the main view.
func (m model) overlayAddForm(behind string) string {
	labels := []string{"Tailnet ID", "Auth Key", "Exit Node (optional)"}
	var formLines []string
	formLines = append(formLines, inputLabel.Render("Add Tailnet"))
	formLines = append(formLines, "")
	for i, inp := range m.addForm.inputs {
		label := labels[i]
		if i == m.addForm.focused {
			label = inputLabel.Render("> " + label)
		} else {
			label = dimStyle.Render("  " + label)
		}
		formLines = append(formLines, label)
		formLines = append(formLines, "  "+inp.View())
	}
	formLines = append(formLines, "")
	formLines = append(formLines, dimStyle.Render("tab next  enter submit  esc cancel"))

	box := confirmStyle.Render(strings.Join(formLines, "\n"))
	return m.overlayOnView(behind, box)
}

// overlayDNSForm renders the DNS settings form over the main view.
func (m model) overlayDNSForm(behind string) string {
	labels := []string{"Mode", "Bind Address (optional)"}
	var formLines []string
	formLines = append(formLines, inputLabel.Render("DNS Settings"))
	formLines = append(formLines, "")
	for i, inp := range m.dnsForm.inputs {
		label := labels[i]
		if i == m.dnsForm.focused {
			label = inputLabel.Render("> " + label)
		} else {
			label = dimStyle.Render("  " + label)
		}
		formLines = append(formLines, label)
		formLines = append(formLines, "  "+inp.View())
	}
	formLines = append(formLines, "")
	formLines = append(formLines, dimStyle.Render("tab next  enter submit  esc cancel"))

	box := confirmStyle.Render(strings.Join(formLines, "\n"))
	return m.overlayOnView(behind, box)
}

// overlayConfirm renders a yes/no confirmation prompt over the main view.
func (m model) overlayConfirm(behind string) string {
	if m.confirm == nil {
		return behind
	}
	var action string
	switch m.confirm.action {
	case "remove":
		action = "Remove"
	case "disconnect":
		action = "Disconnect"
	default:
		action = strings.Title(m.confirm.action)
	}
	prompt := warnStyle.Render(fmt.Sprintf("%s tailnet '%s'?", action, m.confirm.tailnetID))
	hint := dimStyle.Render("(y/n)")
	box := confirmStyle.Render(prompt + "  " + hint)
	return m.overlayOnView(behind, box)
}

// truncate truncates s to maxLen bytes (not rune-aware, kept for short error strings).
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen || maxLen <= 0 {
		return s
	}
	return s[:maxLen-3] + "..."
}

// truncateVisible truncates a string to a visible width, accounting for ANSI codes.
func truncateVisible(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	// Fallback: truncate by byte length (good enough for event lines)
	if len(s) > maxWidth {
		return s[:maxWidth-3] + "..."
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
