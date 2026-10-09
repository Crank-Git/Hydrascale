package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestEachJournalSearchOfTheDiagnosisPagesIsBounded(t *testing.T) {
	// On the test host, a search of the whole journal of the unit took 49 to 60 seconds on
	// 2026-10-07. A search with --since "-2h" took 5 seconds.
	search := regexp.MustCompile(`journalctl -u hydrascale[^|]*\|`)
	bound := regexp.MustCompile(`--since |-n \d+`)
	for _, page := range []string{"operations/troubleshooting.md", "concepts/networking.md"} {
		file := filepath.Join("..", "docs", "site", page)
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if search.MatchString(line) && !bound.MatchString(line) {
				t.Errorf("%s:%d searches the whole journal of the unit; add --since", file, i+1)
			}
		}
	}
}

func TestTheTroubleshootingPageQuotesEachIPv6Reason(t *testing.T) {
	// The operator searches the log for the reason, so the page quotes the code verbatim.
	code, err := os.ReadFile(filepath.Join("..", "internal", "reconciler", "ipv6.go"))
	if err != nil {
		t.Fatalf("read ipv6.go: %v", err)
	}
	reasons := regexp.MustCompile(`plan\.reason = "([^"]+)"`).FindAllSubmatch(code, -1)
	if len(reasons) == 0 {
		t.Fatal("ipv6.go holds no reason, so the scan is broken")
	}
	page, err := os.ReadFile(filepath.Join("..", "docs", "site", "operations", "troubleshooting.md"))
	if err != nil {
		t.Fatalf("read the troubleshooting page: %v", err)
	}
	for _, reason := range reasons {
		if !strings.Contains(string(page), "`"+string(reason[1])+"`") {
			t.Errorf("the troubleshooting page does not quote the IPv6 reason %q", reason[1])
		}
	}
}

func TestTheCommandLinePageNamesEachCommandThatResolvesAnAlias(t *testing.T) {
	// resolveRef in cmd/hydrascale/main.go reads the configuration file for these commands.
	data, err := os.ReadFile(filepath.Join("..", "docs", "site", "reference", "command-line.md"))
	if err != nil {
		t.Fatalf("read the command line page: %v", err)
	}
	page := string(data)
	start := strings.Index(page, "## A tailnet alias")
	if start < 0 {
		t.Fatal("the command line page holds no section A tailnet alias")
	}
	section := page[start:]
	if end := strings.Index(section[1:], "\n## "); end >= 0 {
		section = section[:end+1]
	}
	for _, command := range []string{"exec", "ping", "ssh", "tailscale", "wrap", "env"} {
		if !strings.Contains(section, "`"+command+"`") {
			t.Errorf("the section A tailnet alias does not name the command %s", command)
		}
	}
}

func TestTheHostAccessGuideAndTheTroubleshootingPageStateThePublishedPort(t *testing.T) {
	// FR-publish-19 and FR-publish-20: each page names the key and the local rule that a
	// published port needs, because the configuration load refuses the key without the rule.
	pages := map[string][]string{
		"guides/host-access.md":         {"## Published ports", "`tailnets[].publish`", "to: host"},
		"operations/troubleshooting.md": {"`tailnets[].publish`", "to: host", "iptables -t nat -S PREROUTING"},
	}
	for page, phrases := range pages {
		file := filepath.Join("..", "docs", "site", page)
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, phrase := range phrases {
			if !strings.Contains(string(data), phrase) {
				t.Errorf("%s does not state %q", file, phrase)
			}
		}
	}
}
