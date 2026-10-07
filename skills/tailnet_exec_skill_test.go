package skills

import (
	"strings"
	"testing"
)

// tailnetExecSkill returns the content of the embedded tailnet-exec skill. The test reads
// the embedded set rather than a file, because the gate runs this binary from another
// directory.
func tailnetExecSkill(t *testing.T) string {
	t.Helper()
	set, err := All()
	if err != nil {
		t.Fatalf("All() = %v, want no error", err)
	}
	for _, skill := range set {
		if skill.Name == "tailnet-exec" {
			return string(skill.Content)
		}
	}
	t.Fatal("All() holds no skill named \"tailnet-exec\"")
	return ""
}

// requireText fails the test once for each phrase that content does not hold.
func requireText(t *testing.T, content string, phrases ...string) {
	t.Helper()
	for _, phrase := range phrases {
		if !strings.Contains(content, phrase) {
			t.Errorf("the tailnet-exec skill does not state %q", phrase)
		}
	}
}

// FR-refresh-11: each of the four forms runs ip netns exec, so each needs root.
func TestTheTailnetExecSkillStatesRootForEachForm(t *testing.T) {
	content := tailnetExecSkill(t)
	requireText(t, content,
		"`exec`, `ping`, `ssh`, and `tailscale` each need root",
		"`ip netns exec`",
		"sudo hydrascale exec ",
		"sudo hydrascale ping ",
		"sudo hydrascale ssh ",
		"sudo hydrascale tailscale ",
	)
}

// FR-refresh-10: status reads the host directly when the daemon does not run.
func TestTheTailnetExecSkillStatesTheStandaloneStatus(t *testing.T) {
	content := tailnetExecSkill(t)
	if strings.Contains(content, "does not run in that case") {
		t.Error("the tailnet-exec skill states that a failed status means that the daemon does not run")
	}
	requireText(t, content,
		"reads the configuration file and the host directly",
		"still takes a routing form",
	)
}

// FR-refresh-12 and FR-refresh-13: a form takes the alias, and it reads the
// configuration file to resolve the alias.
func TestTheTailnetExecSkillStatesTheAlias(t *testing.T) {
	content := tailnetExecSkill(t)
	requireText(t, content,
		"`tailnets[].alias`",
		"`ns-<ID>`",
		"`/etc/hydrascale`",
		"run the command with sudo",
	)
}

// FR-refresh-15: the alias zone gives each peer a short name.
func TestTheTailnetExecSkillStatesTheAliasZone(t *testing.T) {
	content := tailnetExecSkill(t)
	requireText(t, content,
		"`<peer>.<alias>.ts.internal`",
		"https://crank-git.github.io/Hydrascale/concepts/dns/",
	)
}

// FR-refresh-14: wrap prints a drop-in, and wrap --apply writes it on the host.
func TestTheTailnetExecSkillStatesWrapApply(t *testing.T) {
	content := tailnetExecSkill(t)
	requireText(t, content,
		"hydrascale wrap <service-name> <tailnet-id> --apply",
		"/etc/systemd/system/<service-name>.service.d/hydrascale.conf",
		"changes the host",
	)
}

// The skill links the command line page for the full forms.
func TestTheTailnetExecSkillLinksTheCommandLinePage(t *testing.T) {
	requireText(t, tailnetExecSkill(t), "https://crank-git.github.io/Hydrascale/reference/command-line/")
}
