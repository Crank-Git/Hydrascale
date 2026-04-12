package tui

import (
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

// errStr is a helper to construct an error value inline.
type errStr string

func (e errStr) Error() string { return string(e) }
