// Package ui holds the console: the static single-page application and its handler.
//
// go:embed places every file of internal/ui/static in the daemon binary, so the daemon
// reads no console file from disk and one binary serves the whole console. See
// FR-console-5 of docs/specs/features/06-console-foundation.md.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static
var staticFS embed.FS

// Files returns the embedded console file system, rooted at internal/ui/static.
// A test reads it to check that the console names no other host.
func Files() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// The directory is a compile-time constant of the embed directive, so this
		// cannot happen. A panic here means the directive changed.
		panic("ui: the embedded console holds no static directory: " + err.Error())
	}
	return sub
}

// Handler returns the handler that serves the console files.
// The handler serves no directory listing, because a listing shows the file set of the
// binary and the console needs no listing.
func Handler() http.Handler {
	files := http.FileServerFS(Files())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}
