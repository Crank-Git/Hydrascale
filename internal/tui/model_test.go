package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"hydrascale/internal/api"
	"hydrascale/internal/config"
)

// minimalStatus returns a StatusResponse with a single tailnet for test use.
func minimalStatus(id string) *api.StatusResponse {
	return &api.StatusResponse{
		Desired:       map[string]config.Tailnet{id: {ID: id}},
		Actual:        nil,
		ErrorStates:   map[string]bool{},
		PausedStates:  map[string]bool{},
		FailureCounts: map[string]int{},
		LastErrors:    map[string]string{},
	}
}

func TestInitialModel_MapsInitialized(t *testing.T) {
	m := initialModel("/tmp/test.sock")
	if m.expanded == nil {
		t.Fatal("expanded map is nil — writing to it will panic")
	}
	if m.detailCache == nil {
		t.Fatal("detailCache map is nil — writing to it will panic")
	}
	if m.fetching == nil {
		t.Fatal("fetching map is nil — writing to it will panic")
	}
}

func TestHandleNormalKey_EnterExpands(t *testing.T) {
	m := initialModel("/tmp/test.sock")
	m.status = minimalStatus("personal")
	m.cursor = 0

	next, cmd := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if !nm.expanded["personal"] {
		t.Error("expected expanded[personal] to be true after first Enter")
	}
	if cmd == nil {
		t.Error("expected a fetchDetail command to be returned, got nil")
	}
}

func TestHandleNormalKey_EnterCollapses(t *testing.T) {
	m := initialModel("/tmp/test.sock")
	m.status = minimalStatus("personal")
	m.cursor = 0
	m.expanded["personal"] = true

	next, cmd := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.expanded["personal"] {
		t.Error("expected expanded[personal] to be false after collapsing Enter")
	}
	if cmd != nil {
		t.Error("expected nil command when collapsing, got non-nil")
	}
}

func TestDetailMsg_PopulatesCache_Success(t *testing.T) {
	m := initialModel("/tmp/test.sock")
	m.expanded["personal"] = true

	detail := &api.TailnetDetailResponse{
		TailscaleIPs: []string{"100.64.0.1"},
		PeerCount:    3,
		OnlinePeers:  2,
		FetchedAt:    time.Now(),
	}
	next, cmd := m.Update(detailMsg{id: "personal", detail: detail})
	nm := next.(model)

	if cmd != nil {
		t.Errorf("expected nil cmd, got non-nil")
	}
	cached := nm.detailCache["personal"]
	if cached == nil {
		t.Fatal("detailCache[personal] is nil after success detailMsg")
	}
	if cached.PeerCount != 3 {
		t.Errorf("PeerCount = %d, want 3", cached.PeerCount)
	}
	if cached.Error != "" {
		t.Errorf("Error should be empty on success, got %q", cached.Error)
	}
}

func TestDetailMsg_PopulatesCache_Error(t *testing.T) {
	m := initialModel("/tmp/test.sock")

	next, _ := m.Update(detailMsg{id: "havoc", err: errStr("daemon unreachable")})
	nm := next.(model)

	cached := nm.detailCache["havoc"]
	if cached == nil {
		t.Fatal("detailCache[havoc] is nil after error detailMsg")
	}
	if cached.Error == "" {
		t.Error("expected Error to be set in cache after error detailMsg")
	}
}

func TestDetailLineCount(t *testing.T) {
	m := initialModel("/tmp/test.sock")

	// Not expanded → 0
	if n := m.detailLineCount("personal"); n != 0 {
		t.Errorf("not expanded: want 0, got %d", n)
	}

	// Expanded but no cache yet → 1 (loading)
	m.expanded["personal"] = true
	if n := m.detailLineCount("personal"); n != 1 {
		t.Errorf("expanded, no cache: want 1, got %d", n)
	}

	// Expanded with error → 1
	m.detailCache["personal"] = &api.TailnetDetailResponse{Error: "unreachable"}
	if n := m.detailLineCount("personal"); n != 1 {
		t.Errorf("expanded, error: want 1, got %d", n)
	}

	// Expanded with populated detail → 6
	m.detailCache["personal"] = &api.TailnetDetailResponse{
		TailscaleIPs: []string{"100.64.0.1"},
		PeerCount:    2,
		FetchedAt:    time.Now(),
	}
	if n := m.detailLineCount("personal"); n != 6 {
		t.Errorf("expanded, populated: want 6, got %d", n)
	}
}

// TestHandleNormalKey_EnterDedup verifies that pressing Enter while a fetch is
// already in-flight (m.fetching[id] == true) does not fire a second fetchDetail command.
func TestHandleNormalKey_EnterDedup(t *testing.T) {
	m := initialModel("/tmp/test.sock")
	m.status = minimalStatus("personal")
	m.cursor = 0
	// Simulate an in-flight fetch: expanded but fetching already set.
	m.expanded["personal"] = true
	m.fetching["personal"] = true

	// Collapse (Enter while expanded)
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	// Now expand again while the old fetch is still in-flight.
	next2, cmd2 := nm.handleNormalKey(tea.KeyMsg{Type: tea.KeyEnter})
	nm2 := next2.(model)

	if !nm2.expanded["personal"] {
		t.Error("expected expanded[personal] = true after re-expand")
	}
	if cmd2 != nil {
		t.Error("expected nil cmd when fetching flag is set (dedup guard), got non-nil")
	}
}

// TestDetailMsg_ClearsFetchingFlag verifies that receiving a detailMsg clears
// the fetching[id] entry so a future re-expand can fire a new fetch.
func TestDetailMsg_ClearsFetchingFlag(t *testing.T) {
	m := initialModel("/tmp/test.sock")
	m.fetching["personal"] = true

	next, _ := m.Update(detailMsg{id: "personal", detail: &api.TailnetDetailResponse{
		TailscaleIPs: []string{"100.64.0.1"},
		FetchedAt:    time.Now(),
	}})
	nm := next.(model)

	if nm.fetching["personal"] {
		t.Error("expected fetching[personal] to be cleared after detailMsg")
	}
}

// TestView_SuppressExpansion verifies that when the terminal is too small to show
// the inline detail panel, the "terminal too small" note is rendered instead.
func TestView_SuppressExpansion(t *testing.T) {
	m := initialModel("/tmp/test.sock")
	m.status = minimalStatus("personal")
	m.cursor = 0
	m.expanded["personal"] = true
	m.detailCache["personal"] = &api.TailnetDetailResponse{
		TailscaleIPs: []string{"100.64.0.1"},
		FetchedAt:    time.Now(),
	}
	// width must be non-zero or View returns early ("Loading...") before suppressExpansion runs.
	m.width = 80
	// A height of 5 is far too small to render the detail rows alongside events.
	m.height = 5

	view := m.View()
	if !strings.Contains(view, "terminal too small") {
		t.Errorf("expected 'terminal too small' note in View output when height=%d, got:\n%s", m.height, view)
	}
}

// TestRenderDetailLines_StalenessBadge verifies that when FetchedAt is older
// than detailStaleAfter, the staleness warning (⚠) appears in the rendered lines.
func TestRenderDetailLines_StalenessBadge(t *testing.T) {
	m := initialModel("/tmp/test.sock")
	m.expanded["personal"] = true
	// FetchedAt more than 30 seconds ago → stale
	m.detailCache["personal"] = &api.TailnetDetailResponse{
		TailscaleIPs: []string{"100.64.0.1"},
		FetchedAt:    time.Now().Add(-(detailStaleAfter + 5*time.Second)),
	}

	lines := m.renderDetailLines("personal")
	var found bool
	for _, line := range lines {
		if strings.Contains(line, "⚠") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected staleness ⚠ badge in renderDetailLines output, got: %v", lines)
	}
}

// errStr is a helper to construct an error value inline.
type errStr string

func (e errStr) Error() string { return string(e) }
