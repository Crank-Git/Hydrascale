package ui

import (
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"hydrascale/internal/config"
)

// readStatic reads one file of the embedded console and fails the test when it is absent.
func readStatic(t *testing.T, name string) string {
	t.Helper()
	data, err := fs.ReadFile(Files(), name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// consoleViews are the five entries of the left navigation. See FR-console-12.
var consoleViews = []string{"overview", "namespaces", "access", "activity", "settings"}

func TestTheNavigationHoldsTheFiveEntriesOfTheShell(t *testing.T) {
	index := readStatic(t, "index.html")
	for _, view := range consoleViews {
		marker := `data-view="` + view + `"`
		if !strings.Contains(index, marker) {
			t.Errorf("the navigation holds no entry %s: the page holds no %s", view, marker)
		}
	}

	links := regexp.MustCompile(`<a[^>]*class="nav-link"`).FindAllString(index, -1)
	if len(links) != len(consoleViews) {
		t.Errorf("the navigation holds %d entries, want %d", len(links), len(consoleViews))
	}
}

func TestTheNavigationShowsTheLogoAndTheDaemonVersion(t *testing.T) {
	// FR-console-13. The version is a machine value, so it renders in the mono typeface.
	index := readStatic(t, "index.html")

	if !strings.Contains(index, `src="/brand/logo-lime.svg"`) {
		t.Error("the navigation names no logo file under /brand/")
	}
	if _, err := fs.Stat(Files(), "brand/logo-lime.svg"); err != nil {
		t.Errorf("the console names a logo that the binary does not hold: %v", err)
	}

	version := regexp.MustCompile(`<span[^>]*id="daemon-version"[^>]*>`).FindString(index)
	if version == "" {
		t.Fatal("the navigation holds no element with the identifier daemon-version")
	}
	if !strings.Contains(version, "mono") {
		t.Errorf("the daemon version is not in the mono typeface: %s", version)
	}
	if !strings.Contains(readStatic(t, "app.js"), "server_version") {
		t.Error("the console reads no server_version field, so it invents the version")
	}
}

func TestEachViewShowsExactlyOneHeadingOfTheLargestSize(t *testing.T) {
	// FR-console-14. The shell holds the one h1 element and the router writes it, so a
	// view cannot add a second one.
	index := readStatic(t, "index.html")

	if got := strings.Count(index, "<h1"); got != 1 {
		t.Errorf("the console holds %d h1 elements, want 1", got)
	}
	if !strings.Contains(index, `id="view-heading"`) {
		t.Error("the shell holds no element with the identifier view-heading")
	}

	for _, view := range consoleViews {
		section := viewSection(t, index, view)
		if strings.Contains(section, "<h1") {
			t.Errorf("the %s view holds an h1 element, so the page holds two headings of the largest size", view)
		}
	}
}

// viewSection returns the markup of one view section of index.html.
func viewSection(t *testing.T, index, view string) string {
	t.Helper()
	start := strings.Index(index, `id="view-`+view+`"`)
	if start < 0 {
		t.Fatalf("the console holds no section for the %s view", view)
	}
	end := strings.Index(index[start:], "</section>")
	if end < 0 {
		t.Fatalf("the section of the %s view has no end", view)
	}
	return index[start : start+end]
}

func TestTheConsoleSourceHoldsNoEmoji(t *testing.T) {
	// FR-console-44. The ranges cover the emoji blocks and the variation selector that
	// turns a symbol into an emoji.
	inEmojiRange := func(r rune) bool {
		switch {
		case r >= 0x2600 && r <= 0x27BF: // miscellaneous symbols and dingbats
			return true
		case r >= 0x2B00 && r <= 0x2BFF: // miscellaneous symbols and arrows
			return true
		case r == 0xFE0F: // the variation selector that requests an emoji glyph
			return true
		case r >= 0x1F000 && r <= 0x1FAFF: // the emoji blocks
			return true
		default:
			return false
		}
	}

	err := fs.WalkDir(Files(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		for _, r := range readStatic(t, name) {
			if inEmojiRange(r) {
				t.Errorf("%s holds the emoji code point U+%04X", name, r)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
}

func TestTheConsoleLoadsEveryBrandTokenFile(t *testing.T) {
	// FR-console-39. The console holds no second palette and no second spacing scale, so
	// every token file of the brand reaches the page.
	source := readStatic(t, "index.html") + readStatic(t, "tokens.css")

	tokens, err := fs.ReadDir(Files(), "brand/tokens")
	if err != nil {
		t.Fatalf("read the token directory: %v", err)
	}
	if len(tokens) == 0 {
		t.Fatal("the brand holds no token file")
	}
	for _, token := range tokens {
		reference := "/brand/tokens/" + token.Name()
		if !strings.Contains(source, reference) {
			t.Errorf("the console loads no %s", reference)
		}
	}
}

func TestTheConsoleDefinesAPlainFallbackForEveryColorMixToken(t *testing.T) {
	// A browser with no color-mix support drops the value where the console reads it, so
	// tokens.css restates each one as a value that every browser parses.
	fallbacks := readStatic(t, "tokens.css")
	if !strings.Contains(fallbacks, "@supports not (color: color-mix(") {
		t.Error("tokens.css holds no @supports rule for a browser with no color-mix support")
	}

	declaration := regexp.MustCompile(`(--[a-z0-9-]+)\s*:\s*[^;]*color-mix\(`)
	found := 0
	err := fs.WalkDir(Files(), "brand/tokens", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		for _, match := range declaration.FindAllStringSubmatch(readStatic(t, name), -1) {
			found++
			property := match[1]
			restated := regexp.MustCompile(regexp.QuoteMeta(property) + `\s*:\s*[^;]+;`)
			ok := false
			for _, line := range restated.FindAllString(fallbacks, -1) {
				if !strings.Contains(line, "color-mix(") {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%s sets %s with color-mix and tokens.css states no plain fallback", name, property)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if found == 0 {
		t.Error("no token file uses color-mix, so this test guards nothing")
	}
}

func TestEveryFocusRuleUsesTheFocusRingToken(t *testing.T) {
	// Every control reaches focus by keyboard and shows the one ring that the brand
	// declares, so no rule removes the outline without drawing the ring.
	style := readStatic(t, "app.css")

	blocks := regexp.MustCompile(`(?s)([^{}]*:focus-visible[^{}]*)\{([^}]*)\}`).FindAllStringSubmatch(style, -1)
	if len(blocks) == 0 {
		t.Fatal("app.css holds no :focus-visible rule, so a control shows no focus ring")
	}
	for _, block := range blocks {
		if !strings.Contains(block[2], "var(--ring-focus)") {
			t.Errorf("the rule %q draws no ring from var(--ring-focus)", strings.TrimSpace(block[1]))
		}
	}

	for _, block := range regexp.MustCompile(`(?s)([^{}]*)\{([^}]*)\}`).FindAllStringSubmatch(style, -1) {
		if !strings.Contains(block[2], "outline:none") && !strings.Contains(block[2], "outline: none") {
			continue
		}
		if !strings.Contains(block[1], ":focus-visible") {
			t.Errorf("the rule %q removes the outline outside a :focus-visible rule", strings.TrimSpace(block[1]))
		}
	}
}

func TestTheConsoleStatesAnEmptyStateForEveryView(t *testing.T) {
	// FR-console-17. The console shows no invented data, so every view states what would
	// fill it. The Node test checks that each sentence belongs to the right view.
	app := readStatic(t, "app.js")
	for _, view := range consoleViews {
		if !strings.Contains(app, `id: "`+view+`"`) {
			t.Errorf("the view table names no %s view", view)
		}
	}
	if got := strings.Count(app, "empty:"); got != len(consoleViews) {
		t.Errorf("the view table states %d empty states, want %d", got, len(consoleViews))
	}
}

// viewModules names the view that each module file draws. The shell draws a placeholder
// for a view that no module claims, so this table grows as each view lands.
var viewModules = map[string]string{"overview": "overview.js"}

func TestEveryViewModuleReachesTheBrowserAndRegistersItself(t *testing.T) {
	// A module file has to be embedded, has to be loaded from the console origin, and has
	// to claim its view. A module that the page does not load leaves the view on the
	// placeholder that the shell draws.
	index := readStatic(t, "index.html")

	for _, view := range consoleViews {
		module, ok := viewModules[view]
		if !ok {
			continue
		}
		source := readStatic(t, module)
		if !strings.Contains(index, `src="/`+module+`"`) {
			t.Errorf("the page loads no /%s, so the %s view keeps its placeholder", module, view)
		}
		if !strings.Contains(source, `registerView("`+view+`"`) {
			t.Errorf("%s registers no draw function for the %s view", module, view)
		}
	}
}

func TestTheTopologyDrawsAnAllowedPathAsADottedCurveInTheAccentColour(t *testing.T) {
	// FR-console-20 and FR-console-23. The geometry of the curve belongs to app.css, so the
	// renderer writes a class and no inline stroke. The accent belongs to the paths of the
	// selected node and to nothing else in this view. See FR-console-40.
	style := readStatic(t, "app.css")

	for _, rule := range []string{
		"stroke-dasharray:2 6",
		"stroke-width:1.4",
		"stroke:var(--edge)",
		"stroke:var(--edge-active)",
	} {
		if !strings.Contains(style, rule) {
			t.Errorf("app.css states no %s, so the allowed path is not the dotted curve of the brand", rule)
		}
	}
	if !strings.Contains(style, ".edge.sel") {
		t.Error("app.css holds no rule for the paths of the selected node")
	}
	if !strings.Contains(style, ".edge.muted") {
		t.Error("app.css holds no rule that mutes the paths of every other node")
	}
}

func TestTheConsoleDrawsNoArrowheadAndNoNodeIcon(t *testing.T) {
	// FR-console-22. The renderer holds no marker element and no image, so no build of the
	// console can put an arrowhead or an icon on the topology.
	forbidden := []string{"<marker", "marker-end", "marker-start", "marker-mid", "<textPath"}

	err := fs.WalkDir(Files(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(name, "brand/") {
			return err
		}
		source := readStatic(t, name)
		for _, word := range forbidden {
			if strings.Contains(source, word) {
				t.Errorf("%s holds %s, so the topology can draw an arrowhead or an edge label", name, word)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
}

func TestTheConsoleJavaScriptTestsPass(t *testing.T) {
	// The console has no build step and no package manager. The Node test runner reads
	// the same app.js that the browser loads. internal/ui/package.json declares the
	// module type for Node and it names no dependency.
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on this host, so the console JavaScript tests do not run here")
	}

	// The test names each file, because Node 26 reads a directory argument as a module
	// to run rather than as a set of tests to find.
	files, err := filepath.Glob("jstest/*.test.mjs")
	if err != nil || len(files) == 0 {
		t.Fatalf("the package holds no console JavaScript test: %v", err)
	}

	out, err := exec.Command(node, append([]string{"--test"}, files...)...).CombinedOutput()
	t.Logf("node --test %s\n%s", strings.Join(files, " "), out)
	if err != nil {
		t.Fatalf("the console JavaScript tests fail: %v", err)
	}
}

func TestTheDevelopmentConfigurationDeclaresTwoTailnetsAndNoAuthKey(t *testing.T) {
	// The console must never show invented data. contrib/dev-config.yaml drives the
	// console against a daemon that cannot connect, which is the state that most needs a
	// designed empty view. See the section "Seed data" of docs/specs/spec.md.
	const file = "../../contrib/dev-config.yaml"

	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", path.Base(file), err)
	}
	if strings.Contains(string(raw), "auth_key") {
		t.Error("the development configuration names an auth key")
	}

	cfg, err := config.LoadConfig(file)
	if err != nil {
		t.Fatalf("load %s: %v", path.Base(file), err)
	}
	if len(cfg.Tailnets) != 2 {
		t.Errorf("the development configuration declares %d tailnets, want 2", len(cfg.Tailnets))
	}
	for _, tailnet := range cfg.Tailnets {
		if tailnet.AuthKey != "" {
			t.Errorf("the tailnet %s holds an auth key", tailnet.ID)
		}
	}
}
