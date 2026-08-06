package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"hydrascale/internal/api"
)

// TestTheAccentColourAppearsInOneStyleOnly asserts FR-docs-19. The accent marks the
// current selection, and nothing else in the terminal interface carries it.
func TestTheAccentColourAppearsInOneStyleOnly(t *testing.T) {
	accent := lipgloss.Color(brandAccent)

	carries := func(s lipgloss.Style) bool {
		return s.GetForeground() == accent ||
			s.GetBackground() == accent ||
			s.GetBorderTopForeground() == accent ||
			s.GetBorderBottomForeground() == accent ||
			s.GetBorderLeftForeground() == accent ||
			s.GetBorderRightForeground() == accent
	}

	var found []string
	for name, style := range styleRegistry {
		if carries(style) {
			found = append(found, name)
		}
	}

	if len(found) != 1 {
		t.Fatalf("styles that carry the accent %s = %v, want exactly [cursorStyle]", brandAccent, found)
	}
	if found[0] != "cursorStyle" {
		t.Errorf("the style that carries the accent = %q, want %q", found[0], "cursorStyle")
	}
}

// TestTheTerminalInterfaceSourceHoldsNoEmoji asserts FR-docs-22. The allowed characters
// are the four that docs/DESIGN.md names, the characters the interface draws, and the
// punctuation of a comment. Every other character outside ASCII fails the test, which
// rejects an emoji.
func TestTheTerminalInterfaceSourceHoldsNoEmoji(t *testing.T) {
	allowed := map[rune]bool{
		'●': true, // a state dot
		'▸': true, // the current selection
		'┄': true, // an inline detail line
		'✓': true,
		'─': true, // a horizontal rule
		'⚠': true, // a stale detail response
		'↑': true, // a key hint
		'↓': true, // a key hint
		'—': true, // punctuation of a comment
		'→': true, // punctuation of a comment
	}

	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob the package source: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("the glob found no source file of the package")
	}

	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, r := range string(data) {
			if r < 128 || allowed[r] {
				continue
			}
			t.Errorf("%s: byte %d holds the character %q (U+%04X), which the brand does not allow", name, i, r, r)
		}
	}
}

// TestTheTailnetListShowsAStateDotAndALowercaseWord asserts FR-docs-20.
func TestTheTailnetListShowsAStateDotAndALowercaseWord(t *testing.T) {
	m := initialModel("/tmp/test.sock")
	m.status = minimalStatus("personal")
	m.status.ErrorStates["personal"] = true
	m.width = 100
	m.height = 40

	view := stripANSI(m.View())

	if !strings.Contains(view, "● error") {
		t.Errorf("the tailnet list does not hold %q, got:\n%s", "● error", view)
	}
	// The column header ERROR is a mono label, which the brand allows in upper case. A
	// state word is never in upper case.
	if strings.Contains(view, "● ERROR") {
		t.Errorf("the tailnet list holds the upper case state word %q", "ERROR")
	}
}

// TestTheStateWordReadsWithoutTheColour asserts the edge case of a terminal that does not
// support 24-bit colour. The word carries the meaning, therefore the word survives the
// removal of every colour code.
func TestTheStateWordReadsWithoutTheColour(t *testing.T) {
	plain := stripANSI(stateCell(healthyStyle, "running"))
	if plain != "● running" {
		t.Errorf("the state cell without the colour = %q, want %q", plain, "● running")
	}
}

// TestTheFooterShowsTheLocalRuleModeAndTheRuleCount asserts FR-docs-21.
func TestTheFooterShowsTheLocalRuleModeAndTheRuleCount(t *testing.T) {
	m := initialModel("/tmp/test.sock")
	m.status = minimalStatus("personal")
	m.status.Access = &api.AccessStatus{Mode: "enforce", Rules: 11}
	m.width = 100
	m.height = 40

	view := stripANSI(m.View())
	if !strings.Contains(view, "access enforce") {
		t.Errorf("the footer does not hold %q, got:\n%s", "access enforce", view)
	}
	if !strings.Contains(view, "rules 11") {
		t.Errorf("the footer does not hold %q, got:\n%s", "rules 11", view)
	}
}

// TestTheFooterNamesTheUnknownRuleMode asserts that a status response without the access
// field shows a word rather than an invented mode or an invented count.
func TestTheFooterNamesTheUnknownRuleMode(t *testing.T) {
	m := initialModel("/tmp/test.sock")
	m.status = minimalStatus("personal")
	m.width = 100
	m.height = 40

	view := stripANSI(m.View())
	if !strings.Contains(view, "access unknown") {
		t.Errorf("the footer does not hold %q, got:\n%s", "access unknown", view)
	}
}

// stripANSI removes every select graphic rendition sequence, which is what a terminal
// without colour support drops.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
