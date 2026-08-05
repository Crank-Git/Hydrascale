package ui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTheEmbeddedConsoleServesTheIndexPage(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / returns %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<title>") {
		t.Errorf("GET / returns a body that holds no title element: %s", rec.Body.String())
	}
}

func TestTheEmbeddedConsoleServesTheBrandTokens(t *testing.T) {
	// go:embed places every file of internal/ui/static in the binary, so the daemon
	// reads no console file from disk. See FR-console-5.
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/brand/tokens/colors.css", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /brand/tokens/colors.css returns %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "--") {
		t.Errorf("the token file holds no custom property: %s", rec.Body.String())
	}
}

func TestTheEmbeddedConsoleHoldsEveryStaticFile(t *testing.T) {
	// The count guards the embed directive. A directive that names one file rather than
	// the directory passes every other test in this file.
	count := 0
	err := fs.WalkDir(Files(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if count < 20 {
		t.Errorf("the embedded file system holds %d files, want 20 or more", count)
	}
}

// TestTheConsoleRequestsNoResourceFromAnotherHost reads every embedded file and refuses a
// reference to another host. CLAUDE.md states that the console makes no request to
// another host, and FR-console-6 holds it. The check skips the xmlns attribute of an SVG
// file, because that value names an XML namespace and it is not a request.
func TestTheConsoleRequestsNoResourceFromAnotherHost(t *testing.T) {
	markers := []string{"http://", "https://", "src=\"//", "href=\"//", "url(//"}

	err := fs.WalkDir(Files(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(Files(), path)
		if err != nil {
			return err
		}
		text := strings.ReplaceAll(string(data), `xmlns="http://www.w3.org/2000/svg"`, "")
		for _, marker := range markers {
			if strings.Contains(text, marker) {
				t.Errorf("%s holds the marker %q, which names another host", path, marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
}
