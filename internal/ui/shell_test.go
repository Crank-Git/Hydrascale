package ui

import (
	"fmt"
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

// consoleViews are the entries of the left navigation. FR-console-12 names five entries,
// and FR-policy-22 adds the policy view as the sixth.
var consoleViews = []string{"overview", "namespaces", "access", "policy", "activity", "settings"}

func TestTheNavigationHoldsEveryEntryOfTheShell(t *testing.T) {
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
		// A WOFF2 file is compressed, so its bytes read as arbitrary code points and
		// they are not console source. The glyph set of each face is not source either.
		if strings.HasSuffix(name, ".woff2") {
			return nil
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

func TestEveryConsoleSourceFileIsText(t *testing.T) {
	// git stores a file that holds one control character as a binary file. That file then
	// carries no diff, no comment on a line, and no blame, and every later change to the
	// console loses its review. A separator inside a template literal is the way this
	// happens, so the test names the line feed and the tab as the two control characters
	// that a source file holds.
	textFile := func(name string) bool {
		for _, suffix := range []string{".js", ".css", ".html", ".json", ".svg"} {
			if strings.HasSuffix(name, suffix) {
				return true
			}
		}
		return false
	}

	err := fs.WalkDir(Files(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !textFile(name) {
			return err
		}
		for at, r := range readStatic(t, name) {
			if r == '\n' || r == '\t' {
				continue
			}
			if r < 0x20 || r == 0x7F {
				t.Errorf("%s holds the control character U+%04X at byte %d, so git reads the file as binary", name, r, at)
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

func TestTheConsoleDeclaresAFaceForEveryFontFile(t *testing.T) {
	// FR-console-6. brand/tokens/fonts.css is the one file that names a font file, so a
	// view needs no change when a face arrives. Each src is a path of this origin,
	// because the console makes no request to another host.
	faces := readStatic(t, "brand/tokens/fonts.css")

	files, err := fs.ReadDir(Files(), "brand/fonts")
	if err != nil {
		t.Fatalf("read the font directory: %v", err)
	}
	declared := 0
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".woff2") {
			continue
		}
		declared++
		reference := "url('/brand/fonts/" + file.Name() + "')"
		if !strings.Contains(faces, reference) {
			t.Errorf("fonts.css declares no face that reads %s", reference)
		}
	}
	if declared == 0 {
		t.Fatal("the brand holds no font file")
	}
	if strings.Count(faces, "@font-face") != declared {
		t.Errorf("fonts.css holds %d @font-face rules, want %d", strings.Count(faces, "@font-face"), declared)
	}
	// typography.css names these two families, and a face that names a third family
	// reaches no element.
	for _, family := range []string{"'Space Grotesk'", "'Space Mono'"} {
		if !strings.Contains(faces, "font-family:"+family) {
			t.Errorf("fonts.css declares no face for the family %s", family)
		}
	}
	// font-display:swap shows a sentence in the fallback family until the face arrives,
	// so the console never shows an empty view.
	if strings.Count(faces, "font-display:swap") != declared {
		t.Errorf("fonts.css sets font-display:swap on %d faces, want %d", strings.Count(faces, "font-display:swap"), declared)
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
		if strings.Contains(block[2], "var(--ring-focus)") {
			continue
		}
		// An SVG element is not a CSS box, so box-shadow draws no ring on it and
		// var(--ring-focus) states a box-shadow value. Such a rule draws the ring with the
		// outline property, and it still takes its colour from a brand token.
		outlined := strings.Contains(block[2], "outline:") &&
			!strings.Contains(block[2], "outline:none") &&
			strings.Contains(block[2], "var(--")
		if !outlined {
			t.Errorf("the rule %q draws no ring from var(--ring-focus) and no ring from an outline", strings.TrimSpace(block[1]))
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
var viewModules = map[string]string{
	"overview": "overview.js",
	"access":   "access.js",
	"policy":   "policy.js",
	"activity": "activity.js",
	"settings": "settings.js",
}

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

// panelViewSources are the files of the DNS view, the activity view, and the settings
// view. panels.js holds the model and the serializer of all three.
var panelViewSources = []string{"panels.js", "activity.js", "settings.js"}

func TestTheConsoleNamesNoCredentialField(t *testing.T) {
	// The settings view shows the daemon configuration, and an auth key, an OAuth client
	// secret, and a Headscale API key never reach it. SA-1 was an auth key in the body of
	// GET /api/status. The console reads no such field anywhere, so no later change to a
	// route can put one on the screen.
	//
	// The add flow of namespaces.js is the one place that carries a credential, because an
	// operator gives an auth key when a tailnet joins. It sends the field and it never
	// renders it: internal/ui/jstest/namespaces.test.mjs holds "the add flow never holds an
	// auth key after the operator submits it" and "the add flow keeps the auth key after a
	// refused request, and it never states the key". The field name is therefore allowed in
	// that file alone, and a literal key is allowed nowhere.
	credentials := []string{"auth_key", "AuthKey", "tskey-", "client_secret", "api_key", "secrets"}
	sendsTheAddFlow := map[string]bool{"auth_key": true, "AuthKey": true, "authKey": true}

	err := fs.WalkDir(Files(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(name, ".js") {
			return err
		}
		source := readStatic(t, name)
		for _, word := range credentials {
			if name == "namespaces.js" && sendsTheAddFlow[word] {
				continue
			}
			if strings.Contains(source, word) {
				t.Errorf("%s names the field %s, so the console can show a credential", name, word)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
}

func TestTheProtectionStateIsAColouredDotAndALowercaseWord(t *testing.T) {
	// FR-console-41. The DNS view states the protected state of every namespace, and it
	// states it the way every other state in this console reads.
	source := readStatic(t, "panels.js")
	for _, word := range []string{"protected", "unprotected"} {
		if !strings.Contains(source, `word: "`+word+`"`) {
			t.Errorf("panels.js states no protection word %q", word)
		}
	}
	// Every state word of this console is lowercase, so no state reads as a title.
	for _, match := range regexp.MustCompile(`word: "([^"]*)"`).FindAllStringSubmatch(source, -1) {
		if match[1] != strings.ToLower(match[1]) {
			t.Errorf("panels.js states the state word %q in a capital letter", match[1])
		}
	}
	for _, tone := range []string{"ok", "crit", "warn"} {
		if !strings.Contains(source, `tone: "`+tone+`"`) {
			t.Errorf("panels.js states no state of the tone %s", tone)
		}
	}
	if !strings.Contains(source, `class="dot ${`) {
		t.Error("panels.js draws no dot from the tone of a state")
	}

	style := readStatic(t, "app.css")
	for _, rule := range []string{".nsrow", ".kv", ".alert"} {
		if !strings.Contains(style, rule) {
			t.Errorf("app.css holds no rule %s, so the DNS view and the settings view have no row", rule)
		}
	}
}

func TestTheThreeViewsRenderEveryValueInTheMonoTypeface(t *testing.T) {
	// FR-console-42. A machine value is never in the sans typeface, and a sentence is
	// never in the mono typeface. Every value of a description list carries the mono
	// class, and the one exception states in words that the daemon reported no value.
	source := readStatic(t, "panels.js")

	values := regexp.MustCompile(`<dd[^>]*>`).FindAllString(source, -1)
	if len(values) == 0 {
		t.Fatal("panels.js writes no description list, so this test guards nothing")
	}
	for _, value := range values {
		if strings.Contains(value, `class="mono"`) || strings.Contains(value, `class="unset"`) {
			continue
		}
		t.Errorf("panels.js writes the value %s outside the mono typeface", value)
	}
}

func TestTheThreeViewsBuildEveryPartOfTheirMarkupFromOneEscapeFunction(t *testing.T) {
	// The console has no authentication, so an unescaped value that the daemon reports is
	// script injection. panels.js is the one file that builds markup from a daemon value,
	// and the two document modules write no markup of their own.
	if !strings.Contains(readStatic(t, "panels.js"), "function esc(") {
		t.Error("panels.js holds no escape function")
	}

	assignment := regexp.MustCompile(`innerHTML\s*=\s*([^;]+);`)
	fromSerializer := regexp.MustCompile(`^[a-zA-Z]+Markup\(`)
	found := 0
	for _, name := range panelViewSources {
		for _, match := range assignment.FindAllStringSubmatch(readStatic(t, name), -1) {
			found++
			if !fromSerializer.MatchString(strings.TrimSpace(match[1])) {
				t.Errorf("%s writes the markup %s, which the serializer of panels.js did not escape", name, match[1])
			}
		}
	}
	if found == 0 {
		t.Error("no view module writes markup, so this test guards nothing")
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
	node, err := nodeForConsoleTests()
	if err != nil {
		t.Fatal(err)
	}
	if node == "" {
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

// nodeForConsoleTests looks for node on the path. It returns the path of node when node
// is present. It returns an empty path and no error when the caller skips, which is a
// developer machine that holds no node. It returns an error when the caller fails, which
// is a gate that holds no node.
//
// A gate runs every test, so a gate that holds no node loses the 70 tests of
// internal/ui/jstest and reports success all the same. Two environment variables mark a
// gate: GitHub Actions sets CI, and the gate script of the test host exports
// HYDRASCALE_GATE. See issue 181 and issue 191.
func nodeForConsoleTests() (string, error) {
	node, err := exec.LookPath("node")
	if err == nil {
		return node, nil
	}
	if os.Getenv("CI") == "" && os.Getenv("HYDRASCALE_GATE") == "" {
		return "", nil
	}
	return "", fmt.Errorf("node is not on the path of this gate, so the console JavaScript "+
		"tests of internal/ui/jstest do not run and their coverage disappears without a "+
		"report. Install node on the gate. See issue 181. LookPath: %w", err)
}

func TestTheConsoleJavaScriptTestsFailOnAGateThatHoldsNoNode(t *testing.T) {
	// A gate that loses node runs 0 of the 70 tests of internal/ui/jstest and still
	// reports success, so the coverage disappears and nothing states it. See issue 181.
	t.Setenv("CI", "true")
	t.Setenv("PATH", "")

	node, err := nodeForConsoleTests()
	if err == nil {
		t.Fatalf("the gate holds no node and the run does not fail: the path is %q", node)
	}
	if !strings.Contains(err.Error(), "node") {
		t.Errorf("the message does not name node: %v", err)
	}
}

func TestTheConsoleJavaScriptTestsFailOnATestHostGateThatHoldsNoNode(t *testing.T) {
	// The gate script of the test host sets no CI, so it exports HYDRASCALE_GATE instead.
	// Without the marker the gate loses the 70 tests and reports success. See issue 191.
	t.Setenv("CI", "")
	t.Setenv("HYDRASCALE_GATE", "1")
	t.Setenv("PATH", "")

	node, err := nodeForConsoleTests()
	if err == nil {
		t.Fatalf("the gate holds no node and the run does not fail: the path is %q", node)
	}
	if !strings.Contains(err.Error(), "node") {
		t.Errorf("the message does not name node: %v", err)
	}
}

func TestTheConsoleJavaScriptTestsSkipOnADeveloperMachineThatHoldsNoNode(t *testing.T) {
	// A developer machine keeps the skip, because node is not a tool that a developer
	// must install to change the daemon.
	t.Setenv("CI", "")
	t.Setenv("HYDRASCALE_GATE", "")
	t.Setenv("PATH", "")

	node, err := nodeForConsoleTests()
	if err != nil {
		t.Fatalf("the developer machine holds no node and the run fails: %v", err)
	}
	if node != "" {
		t.Errorf("the path holds no node and the function reports the path %q", node)
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
