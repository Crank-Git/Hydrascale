package scripts

import (
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// movedDocsPaths are the paths, relative to the repository root, that issue #430 moved
// into docs/site (FR-site-26 to FR-site-29).
var movedDocsPaths = []string{
	"docs/manual",
	"docs/images",
	"docs/UPGRADING.md",
	"docs/security-audit.md",
}

func TestNoFileNamesAMovedDocsPath(t *testing.T) {
	// FR-site-30: a link to a moved file is dead. docs/specs/ records the old paths on
	// purpose, so the search excludes it.
	for _, p := range movedDocsPaths {
		if _, err := os.Stat(filepath.Join("..", p)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists, want it moved into docs/site", p)
		}
	}

	args := []string{"grep", "-n", "-F"}
	for _, p := range movedDocsPaths {
		args = append(args, "-e", p)
	}
	args = append(args, "--", ".", ":!docs/specs/")
	cmd := exec.Command("git", args...)
	cmd.Dir = ".."
	out, err := cmd.Output()
	var exit *exec.ExitError
	switch {
	case err == nil:
		t.Errorf("these lines name a moved docs path:\n%s", out)
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		// git grep exits 1 when no line matches.
	default:
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

// relativeLink matches the target of a Markdown link or of an HTML src or href
// attribute.
var relativeLink = regexp.MustCompile(`\]\(([^)\s]+)\)|(?:src|href)="([^"]+)"`)

func TestEachRelativeLinkOutsideTheSiteResolves(t *testing.T) {
	// FR-site-30: mkdocs build --strict proves the links inside docs/site only. A file
	// such as docs/README.md links to a moved file by a relative path, which the search
	// for the old paths does not find. docs/specs/ records the old paths on purpose.
	cmd := exec.Command("git", "ls-files", "*.md")
	cmd.Dir = ".."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("list the Markdown files: %v", err)
	}
	for _, file := range strings.Fields(string(out)) {
		if strings.HasPrefix(file, "docs/specs/") || strings.HasPrefix(file, "docs/site/") {
			continue
		}
		data, err := os.ReadFile(filepath.Join("..", file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, m := range relativeLink.FindAllStringSubmatch(string(data), -1) {
			target := m[1] + m[2]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			target, _, _ = strings.Cut(target, "#")
			resolved := path.Join(path.Dir(file), target)
			if _, err := os.Stat(filepath.Join("..", filepath.FromSlash(resolved))); err != nil {
				t.Errorf("%s links to %s, and %s does not exist", file, target, resolved)
			}
		}
	}
}

func TestTheNavListsEachMovedPage(t *testing.T) {
	// A page that the nav does not list builds, but no reader finds it from the menu.
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
	var pages []string
	var collect func(v any)
	collect = func(v any) {
		switch v := v.(type) {
		case string:
			pages = append(pages, v)
		case []any:
			for _, e := range v {
				collect(e)
			}
		case map[string]any:
			for _, e := range v {
				collect(e)
			}
		}
	}
	for _, entry := range config.Nav {
		collect(entry)
	}
	for _, want := range []string{
		"guides/index.md",
		"guides/first-run.md",
		"guides/access-editor.md",
		"guides/policy-editor.md",
		"guides/policy-visual-editor.md",
		"operations/upgrade.md",
		"security/audit.md",
	} {
		if !slices.Contains(pages, want) {
			t.Errorf("the navigation does not list %s", want)
		}
	}
}
