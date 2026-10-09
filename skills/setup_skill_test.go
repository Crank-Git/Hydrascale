package skills

import (
	"regexp"
	"strings"
	"testing"
)

// setupSkill returns the content of the skill hydrascale-setup from the embedded set. The
// test reads the embedded set rather than the file, because the gate runs this binary from
// another directory.
func setupSkill(t *testing.T) string {
	t.Helper()
	set, err := All()
	if err != nil {
		t.Fatalf("All() = %v, want no error", err)
	}
	for _, skill := range set {
		if skill.Name == "hydrascale-setup" {
			return string(skill.Content)
		}
	}
	t.Fatal("All() holds no skill named hydrascale-setup")
	return ""
}

// allowedTools returns each entry of the front matter key allowed-tools.
func allowedTools(t *testing.T, content string) []string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		value, found := strings.CutPrefix(line, "allowed-tools:")
		if !found {
			continue
		}
		var tools []string
		for _, tool := range strings.Split(value, ",") {
			tools = append(tools, strings.TrimSpace(tool))
		}
		return tools
	}
	t.Fatal("the front matter holds no allowed-tools key")
	return nil
}

func assertContains(t *testing.T, content string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(content, want) {
			t.Errorf("the skill hydrascale-setup does not contain %q", want)
		}
	}
}

func TestTheSetupSkillStatesSudo(t *testing.T) {
	t.Run("states sudo for status, diff, list, and env", func(t *testing.T) {
		assertContains(t, setupSkill(t),
			"`sudo hydrascale status`",
			"`sudo hydrascale diff`",
			"`sudo hydrascale list`",
			"`sudo hydrascale env <id>`",
			"`hydrascale version`",
		)
	})
}

func TestTheSetupSkillAllowsEachReadOnlyCommand(t *testing.T) {
	t.Run("names each read-only command in allowed-tools in the form that the skill states", func(t *testing.T) {
		tools := allowedTools(t, setupSkill(t))
		for _, want := range []string{
			"Bash(sudo hydrascale status:*)",
			"Bash(sudo hydrascale diff:*)",
			"Bash(sudo hydrascale list:*)",
			"Bash(sudo hydrascale env:*)",
			"Bash(hydrascale status:*)",
			"Bash(hydrascale version:*)",
		} {
			found := false
			for _, tool := range tools {
				if tool == want {
					found = true
				}
			}
			if !found {
				t.Errorf("allowed-tools = %q, want it to hold %q", tools, want)
			}
		}
	})

	t.Run("names no command in allowed-tools that changes the host", func(t *testing.T) {
		for _, tool := range allowedTools(t, setupSkill(t)) {
			for _, form := range []string{"apply", "add", "remove", "init", "uninstall", "wrap", "tui", "serve", "install"} {
				if strings.Contains(tool, "hydrascale "+form) || strings.Contains(tool, "systemctl") {
					t.Errorf("allowed-tools holds %q, which changes the host", tool)
				}
			}
		}
	})
}

func TestTheSetupSkillStatesEachStatusValue(t *testing.T) {
	t.Run("states the ALIAS column and every value that status prints", func(t *testing.T) {
		assertContains(t, setupSkill(t),
			"`ALIAS`", "`-`",
			"`healthy`", "`down`", "`absent`", "`stopped`", "`orphan`",
			"`running`", "`degraded`", "`pending`", "`ERROR`", "`paused`", "`removing`",
		)
	})

	t.Run("states no initial value that status never prints", func(t *testing.T) {
		content := setupSkill(t)
		for _, value := range []string{"`unknown`", "`desired`"} {
			if strings.Contains(content, value) {
				t.Errorf("the skill hydrascale-setup contains %q, which status never prints", value)
			}
		}
	})
}

func TestTheSetupSkillNamesEachHostChange(t *testing.T) {
	t.Run("names every command that changes the host or a tailnet", func(t *testing.T) {
		assertContains(t, setupSkill(t),
			"`sudo hydrascale apply`",
			"`sudo hydrascale add <id>`",
			"`sudo hydrascale remove <id>`",
			"`sudo hydrascale init`",
			"`sudo hydrascale init --force`",
			"`sudo hydrascale uninstall`",
			"`--yes`", "`--purge`", "`--keep-nodes`",
			"`sudo hydrascale wrap <service> <id> --apply`",
			"`sudo hydrascale tui`",
		)
	})

	t.Run("names every console action that changes the host or a tailnet", func(t *testing.T) {
		assertContains(t, setupSkill(t),
			"**Connect**", "**Disconnect**", "**Remove**", "**Apply**", "**Push**",
		)
	})

	t.Run("states that Push changes the policy of every device in the tailnet", func(t *testing.T) {
		assertContains(t, setupSkill(t), "every device in the tailnet")
	})
}

func TestTheSetupSkillStatesTheConfigurationKeys(t *testing.T) {
	t.Run("states the keys that an operator sets most and links the configuration page", func(t *testing.T) {
		assertContains(t, setupSkill(t),
			"`tailnets[].alias`", "`resolver.resolve_aliases`", "`route_table`",
			"`ipv6`", "`host_dns.mode`", "`socket_group`", "`tailnets[].publish`",
			"https://crank-git.github.io/Hydrascale/reference/configuration/",
		)
	})

	t.Run("states that membership of socket_group gives the access of root", func(t *testing.T) {
		assertContains(t, setupSkill(t),
			"membership of `socket_group` gives the access of root",
			"https://crank-git.github.io/Hydrascale/operations/remote-access/",
		)
	})
}

func TestTheSetupSkillStatesTheUpgradeInASectionOfItsOwn(t *testing.T) {
	assertContains(t, setupSkill(t),
		"\n## Upgrade from version 0.9 or 0.10\n",
		"https://crank-git.github.io/Hydrascale/operations/upgrade/",
	)
}

func TestTheSetupSkillStatesTheTunnelOnAnyLocalPort(t *testing.T) {
	assertContains(t, setupSkill(t),
		"any free local port",
		"https://crank-git.github.io/Hydrascale/security/console/",
	)
}

func TestTheSetupSkillNamesNoSourceLine(t *testing.T) {
	content := setupSkill(t)
	t.Run("names no source line in the form file.go:NN", func(t *testing.T) {
		if line := regexp.MustCompile(`\.go:[0-9]+`).FindString(content); line != "" {
			t.Errorf("the skill hydrascale-setup names the source line %q", line)
		}
	})

	t.Run("names no relative path of the site", func(t *testing.T) {
		if strings.Contains(content, "docs/site/") {
			t.Error("the skill hydrascale-setup names a relative path under docs/site/, want an absolute URL")
		}
	})
}
