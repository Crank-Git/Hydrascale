package scripts

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// brandFontURL matches the file name in each src:url(...) entry of brand.css. The
// quotes are optional, because a CSS url() accepts both forms.
var brandFontURL = regexp.MustCompile(`url\(\s*['"]?\.\./assets/brand/fonts/([^'")]+)['"]?\s*\)`)

func TestBrandCSSDeclaresEachFontFile(t *testing.T) {
	// scripts/docs-build.sh copies the whole font directory, and brand.css names each
	// file. A renamed font file therefore builds clean and shows the fallback typeface,
	// so this test compares the two lists.
	files, err := filepath.Glob(filepath.Join("..", "internal", "ui", "static", "brand", "fonts", "*.woff2"))
	if err != nil {
		t.Fatalf("list the brand font files: %v", err)
	}
	var want []string
	for _, f := range files {
		want = append(want, filepath.Base(f))
	}
	if len(want) == 0 {
		t.Fatal("internal/ui/static/brand/fonts holds no .woff2 file")
	}

	css, err := os.ReadFile(filepath.Join("..", "docs", "site", "stylesheets", "brand.css"))
	if err != nil {
		t.Fatalf("read brand.css: %v", err)
	}
	var got []string
	for _, m := range brandFontURL.FindAllSubmatch(css, -1) {
		got = append(got, string(m[1]))
	}

	slices.Sort(want)
	slices.Sort(got)
	got = slices.Compact(got)
	if !slices.Equal(got, want) {
		t.Errorf("brand.css declares the font files %q, and the brand font directory holds %q", got, want)
	}
}

// docsSources are the paths, relative to the repository root, that scripts/docs-build.sh
// reads. A test copies them into a temporary tree, so that a test page never reaches
// docs/site of the repository.
var docsSources = []string{
	"mkdocs.yml",
	"docs/site",
	"docs/theme",
	"scripts/docs-build.sh",
	"internal/ui/static/brand",
}

// mkdocsForDocsTests looks for mkdocs on the path. It returns the path of mkdocs when
// mkdocs is present. It returns an empty path and no error when the caller skips, which
// is a developer machine that holds no mkdocs. It returns an error when the caller fails,
// which is a gate that holds no mkdocs.
//
// A gate that holds no mkdocs runs no test of the site build and reports success all the
// same. GitHub Actions sets CI, and the gate script of the test host exports
// HYDRASCALE_GATE. This is the gate rule of TestTheConsoleJavaScriptTestsPass in
// internal/ui/shell_test.go.
func mkdocsForDocsTests() (string, error) {
	mkdocs, err := exec.LookPath("mkdocs")
	if err == nil {
		return mkdocs, nil
	}
	if os.Getenv("CI") == "" && os.Getenv("HYDRASCALE_GATE") == "" {
		return "", nil
	}
	return "", fmt.Errorf("mkdocs is not on the path of this gate, so the tests of "+
		"scripts/docs-build.sh do not run and their coverage disappears without a report. "+
		"Run pip install -r docs/site/requirements.txt on the gate. LookPath: %w", err)
}

// docsTree returns a temporary tree that holds a copy of each path of docsSources. The
// test skips on a developer machine that holds no mkdocs, and fails on a gate that holds
// no mkdocs.
func docsTree(t *testing.T) string {
	t.Helper()
	mkdocs, err := mkdocsForDocsTests()
	if err != nil {
		t.Fatal(err)
	}
	if mkdocs == "" {
		t.Skip("mkdocs is not on this host, so the documentation site does not build here")
	}
	dir := t.TempDir()
	for _, rel := range docsSources {
		src := filepath.Join("..", filepath.FromSlash(rel))
		if err := copyPath(src, filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("copy %s into the temporary tree: %v", rel, err)
		}
	}
	return dir
}

// copyPath copies the file or the directory src to dst and keeps the mode of each file.
func copyPath(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

// runDocsBuild runs scripts/docs-build.sh of the tree dir and returns the exit code and
// the combined output.
func runDocsBuild(t *testing.T, dir string) (int, string) {
	t.Helper()
	out, err := exec.Command(filepath.Join(dir, "scripts", "docs-build.sh")).CombinedOutput()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("run scripts/docs-build.sh: %v", err)
		}
		code = exit.ExitCode()
	}
	return code, string(out)
}

func TestTheDocsBuildSucceeds(t *testing.T) {
	dir := docsTree(t)
	code, out := runDocsBuild(t, dir)
	if code != 0 {
		t.Fatalf("scripts/docs-build.sh exits %d, want 0\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "build", "site", "index.html")); err != nil {
		t.Errorf("the build writes no build/site/index.html: %v", err)
	}
	for _, f := range []string{"logo-lime.svg", "fonts/BarlowSemiCondensed-Regular.woff2"} {
		path := filepath.Join(dir, "build", "site", "assets", "brand", filepath.FromSlash(f))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the build holds no brand file %s: %v", f, err)
		}
	}
}

func TestTheDocsBuildFailsOnABrokenLink(t *testing.T) {
	// FR-site-8: mkdocs build --strict reports a link to a missing file as a warning,
	// and the script must turn that warning into a failure.
	dir := docsTree(t)
	writeFile(t, dir, "docs/site/broken.md", "# Broken\n\nRead [the missing page](missing.md).\n", 0o644)

	code, out := runDocsBuild(t, dir)
	if code == 0 {
		t.Fatalf("scripts/docs-build.sh exits 0 on a link to a missing file, want non-zero\n%s", out)
	}
	if !strings.Contains(out, "missing.md") {
		t.Errorf("the output does not name the missing file\n%s", out)
	}
}

func TestTheBuiltSiteRequestsNoOtherHost(t *testing.T) {
	// FR-site-38: the theme requests fonts and repository data from other hosts unless
	// the configuration stops it, and a page can add a request of its own. The script
	// must fail on each.
	cases := []struct {
		name string
		page string
		want string
	}{
		{
			name: "a script element that names another host",
			page: "<script src=\"https://cdn.example.com/x.js\"></script>\n",
			want: "https://cdn.example.com/x.js",
		},
		{
			name: "a link element with a URL relative to the protocol",
			page: "<link rel=\"stylesheet\" href=\"//cdn.example.com/x.css\">\n",
			want: "//cdn.example.com/x.css",
		},
		{
			name: "a reference to the Google Fonts host",
			page: "The theme can load fonts.googleapis.com.\n",
			want: "fonts.googleapis.com",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := docsTree(t)
			writeFile(t, dir, "docs/site/request.md", "# Request\n\n"+c.page, 0o644)

			code, out := runDocsBuild(t, dir)
			if code == 0 {
				t.Fatalf("scripts/docs-build.sh exits 0, want non-zero\n%s", out)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("the output does not name %s\n%s", c.want, out)
			}
		})
	}
}

func TestTheHeaderShowsTheLimeMarkTheWordmarkAndARepositoryLink(t *testing.T) {
	// FR-site-37 and FR-site-38. The header element of the theme that carries
	// data-md-component="source" requests api.github.com from JavaScript, so
	// docs/theme/partials/header.html shows a plain link instead.
	dir := docsTree(t)
	if code, out := runDocsBuild(t, dir); code != 0 {
		t.Fatalf("scripts/docs-build.sh exits %d, want 0\n%s", code, out)
	}
	page, err := os.ReadFile(filepath.Join(dir, "build", "site", "index.html"))
	if err != nil {
		t.Fatalf("read the built home page: %v", err)
	}
	html := string(page)
	for _, want := range []*regexp.Regexp{
		regexp.MustCompile(`<img src="assets/brand/logo-lime\.svg"`),
		regexp.MustCompile(`class="md-ellipsis">\s*hydrascale\s*</span>`),
		regexp.MustCompile(`<a href="https://github\.com/Crank-Git/Hydrascale"`),
	} {
		if !want.MatchString(html) {
			t.Errorf("the built home page holds no match for %s", want)
		}
	}
	if strings.Contains(html, `data-md-component="source"`) {
		t.Error(`the built home page holds data-md-component="source", which requests api.github.com`)
	}
}

func TestTheNavHoldsTheEightSections(t *testing.T) {
	// A later issue changes a section entry from one page into a list of pages, so the
	// test reads the key of each entry and not its value.
	data, err := os.ReadFile(filepath.Join("..", "mkdocs.yml"))
	if err != nil {
		t.Fatalf("read mkdocs.yml: %v", err)
	}
	var config struct {
		Nav []map[string]any `yaml:"nav"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse mkdocs.yml: %v", err)
	}
	var got []string
	for _, entry := range config.Nav {
		for key := range entry {
			got = append(got, key)
		}
	}
	want := []string{"Home", "Get started", "Guides", "Concepts", "Reference", "Operations", "Architecture", "Security"}
	if !slices.Equal(got, want) {
		t.Errorf("the navigation holds the sections %q, want %q", got, want)
	}
}

func TestTheDocsTestsFailOnAGateThatHoldsNoMkdocs(t *testing.T) {
	// GitHub Actions sets CI, and the gate script of the test host sets HYDRASCALE_GATE.
	for _, marker := range []string{"CI", "HYDRASCALE_GATE"} {
		t.Run(marker, func(t *testing.T) {
			t.Setenv("CI", "")
			t.Setenv("HYDRASCALE_GATE", "")
			t.Setenv(marker, "1")
			t.Setenv("PATH", "")
			mkdocs, err := mkdocsForDocsTests()
			if err == nil {
				t.Fatalf("the gate holds no mkdocs and the run does not fail: the path is %q", mkdocs)
			}
			if !strings.Contains(err.Error(), "pip install -r docs/site/requirements.txt") {
				t.Errorf("the message does not name the install command: %v", err)
			}
		})
	}
}

func TestTheDocsTestsSkipOnADeveloperMachineThatHoldsNoMkdocs(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("HYDRASCALE_GATE", "")
	t.Setenv("PATH", "")
	mkdocs, err := mkdocsForDocsTests()
	if err != nil {
		t.Fatalf("the developer machine holds no mkdocs and the run fails: %v", err)
	}
	if mkdocs != "" {
		t.Errorf("the path holds no mkdocs and the function reports the path %q", mkdocs)
	}
}
