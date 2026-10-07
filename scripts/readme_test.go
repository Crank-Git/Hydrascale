package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// siteURL is the root of the published site (FR-site-4). FR-readme-8 requires each link of
// the README to a site page to start with it.
const siteURL = "https://crank-git.github.io/Hydrascale/"

// readmeSections are the "## " headings that FR-readme-2 requires after the header, in
// order.
var readmeSections = []string{
	"What Hydrascale does",
	"Requirements",
	"Install",
	"Quick start",
	"The console",
	"Documentation",
	"Agent skills",
	"License",
}

// siteSections are the eight navigation sections of FR-site-10, in order. The
// Documentation table of the README holds one row for each (FR-readme-6).
var siteSections = []string{
	"Home", "Get started", "Guides", "Concepts",
	"Reference", "Operations", "Architecture", "Security",
}

func readReadme(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	return string(data)
}

// readmeSection returns the lines below the heading "## <name>", up to the next "## "
// heading.
func readmeSection(readme, name string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(readme, "\n") {
		if strings.HasPrefix(line, "## ") {
			in = strings.TrimPrefix(line, "## ") == name
			continue
		}
		if in {
			out = append(out, line)
		}
	}
	return out
}

func TestTheReadmeIsShort(t *testing.T) {
	lines := strings.Count(readReadme(t), "\n")
	if lines > 220 {
		t.Errorf("README.md holds %d lines; FR-readme-1 allows 220 or fewer", lines)
	}
}

func TestTheReadmeSectionsAreInOrder(t *testing.T) {
	var got []string
	inFence := false
	for _, line := range strings.Split(readReadme(t), "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "## ") {
			got = append(got, strings.TrimPrefix(line, "## "))
		}
	}
	if !slices.Equal(got, readmeSections) {
		t.Errorf("the README sections are %q; FR-readme-2 requires %q", got, readmeSections)
	}
}

var siteLink = regexp.MustCompile(regexp.QuoteMeta(siteURL) + `[^)\s"'>]*`)

func TestEachSiteLinkOfTheReadmeMapsToAPage(t *testing.T) {
	links := siteLink.FindAllString(readReadme(t), -1)
	if len(links) == 0 {
		t.Fatal("the README holds no link to the site")
	}
	for _, link := range links {
		path := strings.TrimPrefix(link, siteURL)
		path, _, _ = strings.Cut(path, "#")
		path = strings.Trim(path, "/")
		candidates := []string{filepath.Join(path, "index.md")}
		if path != "" {
			candidates = append(candidates, path+".md")
		}
		found := false
		for _, c := range candidates {
			if _, err := os.Stat(filepath.Join("..", "docs", "site", c)); err == nil {
				found = true
			}
		}
		if !found {
			t.Errorf("the README links to %s, but docs/site holds none of %q", link, candidates)
		}
	}
}

var (
	repoMarkdownLink = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	repoAttrLink     = regexp.MustCompile(`(?:href|src)="([^"]+)"`)
)

func TestEachRepositoryLinkOfTheReadmeExists(t *testing.T) {
	readme := readReadme(t)
	var targets []string
	for _, m := range repoMarkdownLink.FindAllStringSubmatch(readme, -1) {
		targets = append(targets, m[1])
	}
	for _, m := range repoAttrLink.FindAllStringSubmatch(readme, -1) {
		targets = append(targets, m[1])
	}
	for _, target := range targets {
		if strings.Contains(target, "://") || strings.HasPrefix(target, "#") ||
			strings.HasPrefix(target, "mailto:") {
			continue
		}
		file, _, _ := strings.Cut(target, "#")
		if _, err := os.Stat(filepath.Join("..", file)); err != nil {
			t.Errorf("the README links to %s, which does not exist (FR-readme-9)", target)
		}
		if strings.HasSuffix(file, ".png") && !strings.HasPrefix(file, "docs/site/images/") {
			t.Errorf("the screenshot %s is not under docs/site/images/ (FR-readme-11)", target)
		}
	}
}

var linkText = regexp.MustCompile(`\[([^\]]+)\]`)

func TestTheDocumentationTableHasOneRowPerSection(t *testing.T) {
	var rows [][]string
	for _, line := range readmeSection(readReadme(t), "Documentation") {
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows = append(rows, cells)
	}
	if len(rows) > 0 {
		rows = rows[1:] // The first row is the header.
	}
	var names []string
	for _, row := range rows {
		// The first cell links the section name to the site, and the second states what the
		// section holds.
		if len(row) < 2 || row[1] == "" {
			t.Errorf("the row %q states no content of the section (FR-readme-6)", row)
			continue
		}
		name := linkText.FindStringSubmatch(row[0])
		if name == nil || !strings.Contains(row[0], "]("+siteURL) {
			t.Errorf("the row %q holds no absolute link to the site (FR-readme-6)", row)
			continue
		}
		names = append(names, name[1])
	}
	if !slices.Equal(names, siteSections) {
		t.Errorf("the Documentation table holds %q; FR-readme-6 requires %q", names, siteSections)
	}
}

// The mode observe writes its lines through the iptables LOG target. The kernel writes
// them, so journald stores no systemd unit with them, and "journalctl -u hydrascale"
// shows none of them.
func TestNoDocumentReadsTheObserveLogByUnit(t *testing.T) {
	files := []string{filepath.Join("..", "README.md")}
	err := filepath.WalkDir(filepath.Join("..", "docs", "site"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatalf("walk docs/site: %v", err)
	}
	wrong := regexp.MustCompile(`journalctl -u hydrascale \| grep hydrascale-would-deny`)
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if wrong.Match(data) {
			t.Errorf("%s reads the observe log with journalctl -u; use journalctl -k", file)
		}
	}
}
