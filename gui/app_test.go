package main

import "testing"

func TestCoerceMock(t *testing.T) {
	cases := []struct {
		mode string
		flag bool
		want string
	}{
		{"", false, ""},           // fresh install stays disconnected
		{"socket", false, "socket"}, // configured connection preserved
		{"mock", false, ""},         // persisted demo migrated away without the flag
		{"", true, "mock"},          // flag opts the developer into demo
		{"socket", true, "mock"},    // flag wins (developer override)
	}
	for _, c := range cases {
		if got := coerceMock(c.mode, c.flag); got != c.want {
			t.Errorf("coerceMock(%q, %v) = %q, want %q", c.mode, c.flag, got, c.want)
		}
	}
}

func TestApplyConnection_SelectsSource(t *testing.T) {
	// Default (no mode) must be the empty source, never mock/demo data.
	a := &App{settings: Settings{Mode: ""}}
	if err := a.applyConnection(); err != nil {
		t.Fatalf("applyConnection: %v", err)
	}
	if _, ok := a.src.(emptySource); !ok {
		t.Errorf("default mode should select emptySource, got %T", a.src)
	}

	// Mock only when explicitly requested (mode already gated to the env flag).
	a.settings = Settings{Mode: "mock"}
	if err := a.applyConnection(); err != nil {
		t.Fatalf("applyConnection: %v", err)
	}
	if _, ok := a.src.(*mockSource); !ok {
		t.Errorf("mock mode should select mockSource, got %T", a.src)
	}
}

func TestDefaultSettings_AutoRefresh(t *testing.T) {
	if got := defaultSettings().RefreshSec; got != 30 {
		t.Errorf("default RefreshSec = %d, want 30", got)
	}
}

func TestMockSource_RendersConnected(t *testing.T) {
	// The frontend keys off Connected; without it the developer demo would render
	// as the not-connected empty state.
	d, err := newMockSource().Dashboard()
	if err != nil {
		t.Fatalf("mock Dashboard: %v", err)
	}
	if !d.Connected || d.Stale {
		t.Errorf("mock dashboard should be connected and not stale, got connected=%v stale=%v", d.Connected, d.Stale)
	}
}

func TestEmptySource_NoDataNoActions(t *testing.T) {
	var s emptySource
	d, err := s.Dashboard()
	if err != nil {
		t.Fatalf("empty Dashboard should not error: %v", err)
	}
	if d.Connected || d.Stale || len(d.Tailnets) != 0 {
		t.Errorf("empty dashboard must be disconnected and empty, got %+v", d)
	}
	if det, _ := s.TailnetDetail("x"); det.Error == "" {
		t.Error("empty detail should carry a not-connected error")
	}
	if s.AddTailnet(AddTailnetRequest{ID: "x"}) == nil {
		t.Error("AddTailnet should be refused when not connected")
	}
	if s.RemoveTailnet("x") == nil {
		t.Error("RemoveTailnet should be refused when not connected")
	}
}
