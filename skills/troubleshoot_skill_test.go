package skills

import (
	"regexp"
	"strings"
	"testing"
)

// troubleshootSkillName is the name of the skill that this file tests.
const troubleshootSkillName = "hydrascale-troubleshoot"

// siteURL opens every link of a skill to the documentation site. See FR-refresh-34.
const siteURL = "https://crank-git.github.io/Hydrascale/"

// diagnosisHeading opens each diagnosis section of the skill.
const diagnosisHeading = "## Diagnosis: "

// troubleshootSkill returns the content of the embedded troubleshoot skill. The test reads
// the embedded set rather than a file, because the gate runs this binary from another
// directory.
func troubleshootSkill(t *testing.T) string {
	t.Helper()
	set, err := All()
	if err != nil {
		t.Fatalf("All() = %v, want no error", err)
	}
	for _, skill := range set {
		if skill.Name == troubleshootSkillName {
			return string(skill.Content)
		}
	}
	t.Fatalf("All() holds no skill named %q", troubleshootSkillName)
	return ""
}

// requireTroubleshootText fails the test once for each phrase that content does not hold.
func requireTroubleshootText(t *testing.T, content string, phrases ...string) {
	t.Helper()
	for _, phrase := range phrases {
		if !strings.Contains(content, phrase) {
			t.Errorf("the %s skill does not state %q", troubleshootSkillName, phrase)
		}
	}
}

// diagnoses returns each diagnosis section of content, keyed by its heading. A section
// ends at the next heading of level 2.
func diagnoses(content string) map[string]string {
	sections := make(map[string]string)
	parts := strings.Split(content, "\n## ")
	for _, part := range parts[1:] {
		section := "## " + part
		if !strings.HasPrefix(section, diagnosisHeading) {
			continue
		}
		heading, _, _ := strings.Cut(section, "\n")
		sections[heading] = section
	}
	return sections
}

// diagnosisSection returns the one diagnosis section whose heading holds topic.
func diagnosisSection(t *testing.T, content, topic string) string {
	t.Helper()
	for heading, section := range diagnoses(content) {
		if strings.Contains(heading, topic) {
			return section
		}
	}
	t.Fatalf("the %s skill holds no diagnosis heading that names %q", troubleshootSkillName, topic)
	return ""
}

// subsection returns the text of the level 3 heading name inside section, up to the next
// level 3 heading. The second result is false when section holds no such heading.
func subsection(section, name string) (string, bool) {
	_, after, found := strings.Cut(section, "\n### "+name+"\n")
	if !found {
		return "", false
	}
	body, _, _ := strings.Cut(after, "\n### ")
	return body, true
}

// FR-refresh-16: the embedded set holds the skill.
func TestTheTroubleshootSkillIsEmbedded(t *testing.T) {
	content := troubleshootSkill(t)
	if !strings.HasPrefix(content, "---\nname: "+troubleshootSkillName+"\n") {
		t.Errorf("the skill does not open with the name %q", troubleshootSkillName)
	}
}

// FR-refresh-23 and FR-refresh-24: each diagnosis holds a read-only check, a printed
// repair command, and a link to the site.
func TestTheTroubleshootSkillHoldsSixDiagnoses(t *testing.T) {
	sections := diagnoses(troubleshootSkill(t))
	if len(sections) != 6 {
		t.Fatalf("the skill holds %d diagnosis sections, want 6", len(sections))
	}
	for heading, section := range sections {
		check, ok := subsection(section, "Check")
		if !ok {
			t.Errorf("%q holds no subsection Check", heading)
		} else if !strings.Contains(check, "```") {
			t.Errorf("%q holds no command block in its subsection Check", heading)
		}
		repair, ok := subsection(section, "Repair")
		if !ok {
			t.Errorf("%q holds no subsection Repair", heading)
		} else {
			if !strings.Contains(repair, "```") {
				t.Errorf("%q holds no command block in its subsection Repair", heading)
			}
			if !strings.Contains(strings.ToLower(repair), "print") {
				t.Errorf("%q does not tell the agent to print the repair", heading)
			}
		}
		if !strings.Contains(section, siteURL) {
			t.Errorf("%q holds no link to %s", heading, siteURL)
		}
	}
}

// FR-refresh-17: the allowed-tools field names no command that changes the host. A prefix
// rule allows every suffix, so `sudo hydrascale tailscale` would also allow `up` and
// `logout`, and `sudo journalctl` would also allow `--vacuum-time`.
func TestTheTroubleshootSkillAllowsNoHostChange(t *testing.T) {
	entries := allowedTools(t, troubleshootSkill(t))
	if len(entries) == 0 {
		t.Fatal("allowed-tools names no entry")
	}
	forbidden := []string{
		"apply", "add", "remove", "init", "uninstall", "wrap", "tailscale ", "exec",
		"connect", "systemctl", "modprobe", "curl", "netns",
		" -A", " -I", " -D", " -F", " -X", " -N", " -P", " -w",
		"sudo journalctl", "sudo sysctl", "sudo ip ", "sudo resolvectl",
	}
	for _, entry := range entries {
		if entry != "Read" && !strings.HasPrefix(entry, "Bash(") {
			t.Errorf("allowed-tools names %q, want Read or a Bash rule alone", entry)
			continue
		}
		for _, word := range forbidden {
			if strings.Contains(entry, word) {
				t.Errorf("allowed-tools names %q, which holds %q", entry, word)
			}
		}
	}
}

// FR-refresh-31 checks each event against the code. This test checks that the skill
// names the four events that its diagnoses read.
func TestTheTroubleshootSkillNamesEachEvent(t *testing.T) {
	requireTroubleshootText(t, troubleshootSkill(t),
		"`ipv6.state`",
		"`dns.unprotected`",
		"`dns.split_domain_conflict`",
		"`access.jump_displaced`",
	)
}

// FR-refresh-18: the IPv6 diagnosis.
func TestTheTroubleshootSkillDiagnosesIPv6(t *testing.T) {
	requireTroubleshootText(t, diagnosisSection(t, troubleshootSkill(t), "IPv6"),
		"`ipv6.state`",
		"`ipv6: true`",
		"upstream device",
		"NAT66",
		"sudo ip6tables -t nat -S POSTROUTING",
		siteURL+"concepts/networking/",
	)
}

// FR-refresh-19: the direct connection diagnosis.
func TestTheTroubleshootSkillDiagnosesADirectConnection(t *testing.T) {
	requireTroubleshootText(t, diagnosisSection(t, troubleshootSkill(t), "direct connection"),
		"41642",
		"41895",
		"sudo iptables -t nat -S PREROUTING",
		"sudo hydrascale tailscale <tailnet-id> -- netcheck",
		siteURL+"concepts/networking/",
	)
}

// FR-refresh-20: the DNS diagnosis.
func TestTheTroubleshootSkillDiagnosesDNS(t *testing.T) {
	requireTroubleshootText(t, diagnosisSection(t, troubleshootSkill(t), "DNS"),
		"`host_dns.mode`",
		"overlay mount",
		"`dns.unprotected`",
		"`dns.split_domain_conflict`",
		siteURL+"concepts/dns/",
	)
}

// FR-refresh-21: the displaced jump rule diagnosis.
func TestTheTroubleshootSkillDiagnosesADisplacedJumpRule(t *testing.T) {
	requireTroubleshootText(t, diagnosisSection(t, troubleshootSkill(t), "jump rule"),
		"`access.jump_displaced`",
		"`ts-forward`",
		"`DOCKER-USER`",
		"`DOCKER-FORWARD`",
		siteURL+"operations/troubleshooting/",
	)
}

// FR-refresh-22: the rejected credential diagnosis.
func TestTheTroubleshootSkillDiagnosesARejectedCredential(t *testing.T) {
	requireTroubleshootText(t, diagnosisSection(t, troubleshootSkill(t), "credential"),
		"`rejected`",
		"An absent credential is not a fault",
		siteURL+"guides/credentials/",
	)
}

// FR-publish-21: the refused port diagnosis reads the nat PREROUTING chain of the
// namespace, and it names the key and the local rule that repair the cause.
func TestTheTroubleshootSkillDiagnosesARefusedPortOfTheHost(t *testing.T) {
	requireTroubleshootText(t, diagnosisSection(t, troubleshootSkill(t), "refused"),
		"iptables -t nat -S PREROUTING",
		"tailscale0",
		"`tailnets[].publish`",
		"to: host",
		"127.0.0.1",
		siteURL+"operations/troubleshooting/#a-peer-gets-connection-refused-on-a-port-of-the-host",
	)
}

// Flow 1 of the feature: an agent that finds no fault states what it checked.
func TestTheTroubleshootSkillStatesTheResultWithNoFault(t *testing.T) {
	requireTroubleshootText(t, troubleshootSkill(t),
		"## No fault found",
		siteURL+"operations/troubleshooting/",
	)
}

// FR-refresh-33: a line number moves with each edit, so a skill names none.
func TestTheTroubleshootSkillNamesNoSourceLine(t *testing.T) {
	sourceLine := regexp.MustCompile(`\.go:\d+`)
	if match := sourceLine.FindString(troubleshootSkill(t)); match != "" {
		t.Errorf("the skill names the source line %q", match)
	}
}
