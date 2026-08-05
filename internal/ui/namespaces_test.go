package ui

import (
	"regexp"
	"strings"
	"testing"

	"hydrascale/internal/config"
)

// identifierPattern reads the identifier rule out of the console source.
var identifierPattern = regexp.MustCompile(`IDENTIFIER_PATTERN\s*=\s*/([^/]+)/`)

func TestTheAddFlowAppliesTheIdentifierRuleOfTheDaemon(t *testing.T) {
	// FR-console-32. The console repeats the rule, and this test proves that the copy
	// still answers as config.IsValidID answers. SA-3 and SA-14 record what happened when
	// a route and a loader disagreed.
	source := readStatic(t, "namespaces.js")
	match := identifierPattern.FindStringSubmatch(source)
	if match == nil {
		t.Fatal("namespaces.js declares no IDENTIFIER_PATTERN")
	}

	rule, err := regexp.Compile(match[1])
	if err != nil {
		t.Fatalf("the console pattern %q does not compile: %v", match[1], err)
	}

	ids := []string{
		"alpha", "corp-prod", "a.b_c-1", "9", "A",
		strings.Repeat("a", 63), strings.Repeat("a", 64),
		"My Net", "-lead", ".dot", "_under", "a/b", "../../tmp/x", "",
	}
	for _, id := range ids {
		if got, want := rule.MatchString(id), config.IsValidID(id); got != want {
			t.Errorf("the console accepts %q as %v and the daemon accepts it as %v", id, got, want)
		}
	}
}

func TestThePageLoadsTheNamespaceViewModule(t *testing.T) {
	index := readStatic(t, "index.html")
	if !strings.Contains(index, `src="/namespaces.js"`) {
		t.Error("index.html loads no /namespaces.js, so the namespace view never registers")
	}
	if source := readStatic(t, "namespaces.js"); !strings.Contains(source, `registerView("namespaces"`) {
		t.Error("namespaces.js calls registerView for no namespaces view")
	}
}

func TestTheRemovalDialogNamesTheLogoutCommandAndTheAuthorization(t *testing.T) {
	// FR-console-30. The removal leaves the node authorized on the control server, and the
	// dialog says so before the operator confirms.
	source := readStatic(t, "namespaces.js")
	for _, want := range []string{"tailscale logout", "stays authorized on the control server"} {
		if !strings.Contains(source, want) {
			t.Errorf("the removal dialog names no %q", want)
		}
	}
}

func TestTheNamespaceViewSendsEveryMutatingRequestThroughTheConsoleLayer(t *testing.T) {
	// FR-console-8. Every mutating route needs the console header, and app.js sets it. A
	// bare fetch in the view would send no header and the daemon would refuse it.
	source := readStatic(t, "namespaces.js")
	if strings.Contains(source, "fetch(") {
		t.Error("namespaces.js calls fetch, so it bypasses the request layer that sets the console header")
	}
	for _, route := range []string{"/api/tailnet/add", "/api/tailnet/remove", "/api/tailnet/connect", "/api/tailnet/disconnect"} {
		if !strings.Contains(source, route) {
			t.Errorf("the namespace view names no %s", route)
		}
	}
}

func TestTheNamespaceViewUsesTheAccentForOneThing(t *testing.T) {
	// FR-console-40. The affirmative action is the one accent use of this view. The
	// selected row shows the selection with a surface and a border, and no second accent.
	style := readStatic(t, "app.css")

	blocks := regexp.MustCompile(`(?s)([^{}]*)\{([^}]*)\}`).FindAllStringSubmatch(style, -1)
	var accented []string
	for _, block := range blocks {
		selector := strings.TrimSpace(block[1])
		if !strings.HasPrefix(selector, ".ns-") && !strings.Contains(selector, ".btn.primary") {
			continue
		}
		if strings.Contains(block[2], "var(--accent)") {
			accented = append(accented, selector)
		}
	}
	if len(accented) != 1 {
		t.Errorf("the namespace view paints %d selectors with the accent, want 1: %v", len(accented), accented)
	}
	if len(accented) == 1 && !strings.Contains(accented[0], ".btn.primary") {
		t.Errorf("the accent belongs to the affirmative action, and it paints %q", accented[0])
	}
}
