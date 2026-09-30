package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes body to a file and returns the path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestResolveTailnetRef_returns_the_id_of_an_alias(t *testing.T) {
	cfg := &Config{Tailnets: []Tailnet{
		{ID: "Tk4738291056CNTRL", Alias: "alias1"},
		{ID: "Tq8162094375CNTRL", Alias: "alias2"},
	}}

	got, err := cfg.ResolveTailnetRef("alias2")
	if err != nil {
		t.Fatalf("ResolveTailnetRef: %v", err)
	}
	if got != "Tq8162094375CNTRL" {
		t.Errorf("ResolveTailnetRef(alias2) = %q, want %q", got, "Tq8162094375CNTRL")
	}
}

func TestResolveTailnetRef_returns_the_id_of_an_id(t *testing.T) {
	cfg := &Config{Tailnets: []Tailnet{{ID: "Tk4738291056CNTRL", Alias: "alias1"}}}

	got, err := cfg.ResolveTailnetRef("Tk4738291056CNTRL")
	if err != nil {
		t.Fatalf("ResolveTailnetRef: %v", err)
	}
	if got != "Tk4738291056CNTRL" {
		t.Errorf("ResolveTailnetRef = %q, want the ID", got)
	}
}

func TestResolveTailnetRef_prefers_an_id_over_an_alias(t *testing.T) {
	// The loader rejects this file, so the state reaches ResolveTailnetRef only through a
	// caller that builds the struct itself. The ID must still win, because a namespace
	// carries the ID and never the alias.
	cfg := &Config{Tailnets: []Tailnet{
		{ID: "one", Alias: "two"},
		{ID: "two", Alias: "three"},
	}}

	got, err := cfg.ResolveTailnetRef("two")
	if err != nil {
		t.Fatalf("ResolveTailnetRef: %v", err)
	}
	if got != "two" {
		t.Errorf("ResolveTailnetRef(two) = %q, want the tailnet whose ID is two", got)
	}
}

func TestResolveTailnetRef_rejects_a_name_that_no_tailnet_holds(t *testing.T) {
	cfg := &Config{Tailnets: []Tailnet{{ID: "Tk4738291056CNTRL", Alias: "alias1"}}}

	if _, err := cfg.ResolveTailnetRef("absent"); err == nil {
		t.Fatal("ResolveTailnetRef returned no error, want a rejection")
	}
}

func TestLoadConfig_accepts_an_alias(t *testing.T) {
	path := writeConfig(t, `version: 2
tailnets:
  - id: Tk4738291056CNTRL
    alias: alias1
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Tailnets[0].Alias != "alias1" {
		t.Errorf("alias = %q, want %q", cfg.Tailnets[0].Alias, "alias1")
	}
}

func TestLoadConfig_rejects_an_alias_that_starts_with_a_hyphen(t *testing.T) {
	path := writeConfig(t, `version: 2
tailnets:
  - id: Tk4738291056CNTRL
    alias: "-alias1"
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig returned no error, want a rejection of the alias")
	}
	if !strings.Contains(err.Error(), "alias") {
		t.Errorf("error = %q, want it to name the alias", err)
	}
}

func TestLoadConfig_accepts_a_hyphen_and_an_underscore_inside_an_alias(t *testing.T) {
	path := writeConfig(t, `version: 2
tailnets:
  - id: Tk4738291056CNTRL
    alias: alias_1-b
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Tailnets[0].Alias != "alias_1-b" {
		t.Errorf("alias = %q, want %q", cfg.Tailnets[0].Alias, "alias_1-b")
	}
}

func TestLoadConfig_rejects_two_tailnets_that_hold_one_alias(t *testing.T) {
	path := writeConfig(t, `version: 2
tailnets:
  - id: Tk4738291056CNTRL
    alias: alias1
  - id: Tq8162094375CNTRL
    alias: alias1
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig returned no error, want a rejection of the duplicate alias")
	}
	if !strings.Contains(err.Error(), "duplicate alias") {
		t.Errorf("error = %q, want it to state the duplicate alias", err)
	}
}

func TestLoadConfig_rejects_an_alias_that_is_the_id_of_a_later_tailnet(t *testing.T) {
	// The alias of the first tailnet is the ID of the second one, which the loader reads
	// after it. The check must therefore run after the loop that reads every ID.
	path := writeConfig(t, `version: 2
tailnets:
  - id: Tk4738291056CNTRL
    alias: Tq8162094375CNTRL
  - id: Tq8162094375CNTRL
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig returned no error, want a rejection of the ambiguous alias")
	}
	if !strings.Contains(err.Error(), "is the ID of a tailnet") {
		t.Errorf("error = %q, want it to state the collision with an ID", err)
	}
}

func TestLoadConfig_rejects_a_dot_inside_an_alias(t *testing.T) {
	// An ID accepts a dot and an alias does not. The first character of this alias is
	// valid, so the check must read the whole name.
	path := writeConfig(t, `version: 2
tailnets:
  - id: Tk4738291056CNTRL
    alias: alias1.b
`)

	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig returned no error, want a rejection of the dot")
	}
}

func TestSaveConfig_refuses_an_alias_that_is_the_id_of_a_tailnet(t *testing.T) {
	// `hydrascale add <alias>` built this state and stored it. Every later command then
	// failed on the load, and the operator could not even remove the tailnet.
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &Config{Version: 2, Tailnets: []Tailnet{
		{ID: "Tk4738291056CNTRL", Alias: "alias1"},
		{ID: "alias1"},
	}}

	if err := SaveConfig(path, cfg); err == nil {
		t.Fatal("SaveConfig returned no error, want it to refuse the write")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("SaveConfig wrote %s, want no file", path)
	}
}

func TestSaveConfig_keeps_the_alias_across_a_round_trip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &Config{Version: 2, Tailnets: []Tailnet{{ID: "Tk4738291056CNTRL", Alias: "alias1"}}}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	back, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if back.Tailnets[0].Alias != "alias1" {
		t.Errorf("alias = %q, want %q after a round trip", back.Tailnets[0].Alias, "alias1")
	}
}

// aliasConfig returns a configuration whose tailnets hold the given aliases.
func aliasConfig(resolveAliases bool, aliases ...string) *Config {
	c := &Config{}
	c.Resolver.ResolveAliases = resolveAliases
	for i, a := range aliases {
		c.Tailnets = append(c.Tailnets, Tailnet{ID: string(rune('a'+i)) + "-tailnet", Alias: a})
	}
	return c
}

// resolve_aliases builds the domain <alias>.ts.internal, therefore an alias must be a DNS
// label. An underscore is a legal alias and it is no DNS label.
func TestValidateTailnetNames_rejects_an_alias_that_is_no_dns_label(t *testing.T) {
	err := aliasConfig(true, "my_tailnet").ValidateTailnetNames()
	if err == nil {
		t.Fatal("ValidateTailnetNames accepted the alias my_tailnet with resolve_aliases set")
	}
	if !strings.Contains(err.Error(), "ts.internal") {
		t.Errorf("the error does not name the domain: %v", err)
	}
}

// A file that leaves resolve_aliases out keeps every alias that it holds now.
func TestValidateTailnetNames_accepts_an_underscore_when_resolve_aliases_is_unset(t *testing.T) {
	if err := aliasConfig(false, "my_tailnet").ValidateTailnetNames(); err != nil {
		t.Errorf("ValidateTailnetNames rejected my_tailnet with resolve_aliases unset: %v", err)
	}
}

// A domain name folds case, therefore two aliases that differ by case alone name one zone.
func TestValidateTailnetNames_rejects_two_aliases_that_differ_by_case(t *testing.T) {
	if err := aliasConfig(true, "mmo", "MMO").ValidateTailnetNames(); err == nil {
		t.Fatal("ValidateTailnetNames accepted the aliases mmo and MMO with resolve_aliases set")
	}
	if err := aliasConfig(false, "mmo", "MMO").ValidateTailnetNames(); err != nil {
		t.Errorf("ValidateTailnetNames rejected mmo and MMO with resolve_aliases unset: %v", err)
	}
}

// An alias that is the ID of another tailnet, case folded, names that tailnet twice.
func TestValidateTailnetNames_rejects_an_alias_that_is_an_id_of_another_case(t *testing.T) {
	c := &Config{Tailnets: []Tailnet{{ID: "Corp"}, {ID: "home", Alias: "CORP"}}}
	c.Resolver.ResolveAliases = true
	if err := c.ValidateTailnetNames(); err == nil {
		t.Fatal("ValidateTailnetNames accepted the alias CORP against the ID Corp")
	}
}

func TestValidateTailnetNames_accepts_a_dns_label_alias(t *testing.T) {
	if err := aliasConfig(true, "mmo", "psm-eu", "t1").ValidateTailnetNames(); err != nil {
		t.Errorf("ValidateTailnetNames rejected a DNS label alias: %v", err)
	}
}
