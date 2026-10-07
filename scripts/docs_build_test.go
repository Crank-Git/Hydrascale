package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
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
