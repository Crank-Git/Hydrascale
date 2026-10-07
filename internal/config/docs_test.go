package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/docscheck"
)

func TestTheConfigurationPageNamesEveryKey(t *testing.T) {
	// FR-site-18: the page states every key that LoadConfig reads. A key that the struct
	// gains and the page omits fails here, so the page cannot fall behind the code.
	page, err := os.ReadFile(filepath.Join("..", "..", "docs", "site", "reference", "configuration.md"))
	if err != nil {
		t.Fatalf("read the configuration page: %v", err)
	}
	for _, key := range docscheck.ConfigKeys() {
		if !strings.Contains(string(page), "`"+key+"`") {
			t.Errorf("the configuration page does not name the key `%s`", key)
		}
	}
}
