package scripts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/docscheck"
)

func TestTheEventsPageNamesEveryEvent(t *testing.T) {
	// FR-site-19: the page states every event type that the daemon records. An event that
	// the code gains and the page omits fails here.
	events, err := docscheck.EventTypes("..")
	if err != nil {
		t.Fatalf("list the event types: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("the source holds no event type, so the scan of internal/ is broken")
	}
	page, err := os.ReadFile(filepath.Join("..", "docs", "site", "reference", "events.md"))
	if err != nil {
		t.Fatalf("read the events page: %v", err)
	}
	for _, event := range events {
		if !strings.Contains(string(page), "`"+event+"`") {
			t.Errorf("the events page does not name the event type `%s`", event)
		}
	}
}

func TestTheSearchFindsRouteTable(t *testing.T) {
	// The acceptance criterion of issue #432: a search for route_table returns the
	// configuration page. The theme builds its search from search_index.json.
	dir := docsTree(t)
	if code, out := runDocsBuild(t, dir); code != 0 {
		t.Fatalf("scripts/docs-build.sh exits %d, want 0\n%s", code, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "build", "site", "search", "search_index.json"))
	if err != nil {
		t.Fatalf("read the search index: %v", err)
	}
	var index struct {
		Docs []struct {
			Location string `json:"location"`
			Title    string `json:"title"`
			Text     string `json:"text"`
		} `json:"docs"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("parse the search index: %v", err)
	}
	for _, doc := range index.Docs {
		if strings.HasPrefix(doc.Location, "reference/configuration/") &&
			(strings.Contains(doc.Title, "route_table") || strings.Contains(doc.Text, "route_table")) {
			return
		}
	}
	t.Error("the search index holds no entry of reference/configuration/ that names route_table")
}
