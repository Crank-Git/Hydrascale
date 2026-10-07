package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// sitePage is one page that FR-site-11 to FR-site-16 require. The page holds each
// string of want, and it holds no string of forbid.
type sitePage struct {
	fr      string
	section string
	page    string
	want    []string
	forbid  []string
}

var sitePages = []sitePage{
	{"FR-site-11", "Home", "index.md", []string{"images/console-overview.png", "get-started/"}, nil},
	{"FR-site-12", "Get started", "get-started/index.md", nil, nil},
	{"FR-site-12", "Get started", "get-started/requirements.md", []string{"# Requirements"}, nil},
	{"FR-site-12", "Get started", "get-started/install.md", []string{"## A released binary", "## A build from source"}, nil},
	{"FR-site-12", "Get started", "get-started/quick-start.md", []string{"# Quick start", "hydrascale init"}, nil},
	// The heading "Getting started" is an -ing noun, which the writing standard rejects.
	{"FR-site-13", "Guides", "guides/index.md", nil, []string{"Getting started"}},
	{"FR-site-14", "Guides", "guides/credentials.md", []string{"secrets.yaml", "HYDRASCALE_TS_CLIENT_ID_"}, nil},
	{"FR-site-14", "Guides", "guides/host-access.md", []string{"host_access", "route_table"}, nil},
	{"FR-site-14", "Guides", "guides/headscale.md", []string{"control_url", "policy.mode"}, nil},
	{"FR-site-15", "Concepts", "concepts/index.md", nil, nil},
	{"FR-site-15", "Concepts", "concepts/local-rules.md", []string{"## The two modes", "observe", "enforce"}, nil},
	{"FR-site-15", "Concepts", "concepts/upstream-policy.md", []string{"If-Match", "HTTP 412"}, nil},
	{"FR-site-15", "Concepts", "concepts/dns.md", []string{"overlay mount", "host_dns"}, nil},
	{"FR-site-15", "Concepts", "concepts/networking.md", nil, nil},
}

// navSections returns the pages of each navigation section, by section name.
func navSections(t *testing.T) map[string][]string {
	t.Helper()
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
	sections := map[string][]string{}
	var collect func(section string, v any)
	collect = func(section string, v any) {
		switch v := v.(type) {
		case string:
			sections[section] = append(sections[section], v)
		case []any:
			for _, e := range v {
				collect(section, e)
			}
		case map[string]any:
			for _, e := range v {
				collect(section, e)
			}
		}
	}
	for _, entry := range config.Nav {
		for name, v := range entry {
			collect(name, v)
		}
	}
	return sections
}

func TestTheSitePagesExist(t *testing.T) {
	sections := navSections(t)
	for _, p := range sitePages {
		data, err := os.ReadFile(filepath.Join("..", "docs", "site", filepath.FromSlash(p.page)))
		if err != nil {
			t.Errorf("%s: read %s: %v", p.fr, p.page, err)
			continue
		}
		text := string(data)
		for _, w := range p.want {
			if !strings.Contains(text, w) {
				t.Errorf("%s: %s does not hold %q", p.fr, p.page, w)
			}
		}
		for _, f := range p.forbid {
			if strings.Contains(text, f) {
				t.Errorf("%s: %s holds %q", p.fr, p.page, f)
			}
		}
		if !slices.Contains(sections[p.section], p.page) {
			t.Errorf("%s: the navigation section %s does not list %s", p.fr, p.section, p.page)
		}
	}
}

func TestTheNetworkingPageStatesEachTopic(t *testing.T) {
	// FR-site-16 names six topics. A heading for each topic puts it in the table of
	// contents of the page, so the operator finds it without a scroll.
	data, err := os.ReadFile(filepath.Join("..", "docs", "site", "concepts", "networking.md"))
	if err != nil {
		t.Fatalf("read the networking page: %v", err)
	}
	headings := regexp.MustCompile(`(?m)^#{2,3} (.+)$`).FindAllStringSubmatch(string(data), -1)
	var got []string
	for _, h := range headings {
		got = append(got, h[1])
	}
	for _, want := range []string{
		"IP forwarding",
		"Veth pairs",
		"NAT and masquerade",
		"Direct connections and the listen port",
		"IPv6",
		"Docker",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("the networking page holds no heading %q; its headings are %q", want, got)
		}
	}
}
