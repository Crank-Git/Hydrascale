package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ciStep is one step of the job test in .github/workflows/ci.yml.
type ciStep struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

// ciSteps returns the steps of the job test in .github/workflows/ci.yml.
func ciSteps(t *testing.T) []ciStep {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []ciStep `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse ci.yml: %v", err)
	}
	steps := workflow.Jobs["test"].Steps
	if len(steps) == 0 {
		t.Fatal("ci.yml holds no step in the job test, so the parse is broken")
	}
	return steps
}

// ciCommands returns the commands of a run block that a contributor types. A one-line
// block is one command. A block of more lines holds shell logic around its commands, so
// only the lines that start a go, node, or python3 command count.
func ciCommands(run string) []string {
	lines := strings.Split(strings.TrimSpace(run), "\n")
	if len(lines) == 1 {
		return lines
	}
	var commands []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for _, tool := range []string{"go ", "node ", "python3 "} {
			if strings.HasPrefix(line, tool) {
				commands = append(commands, line)
			}
		}
	}
	return commands
}

// readSkill returns the text of the contributor skill name in .claude/skills.
func readSkill(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", ".claude", "skills", name, "SKILL.md"))
	if err != nil {
		t.Fatalf("read the skill %s: %v", name, err)
	}
	return string(data)
}

func TestTheTestSkillNamesEveryCIStep(t *testing.T) {
	// FR-refresh-26: a step that CI gains and the skill omits fails here.
	skill := readSkill(t, "test")
	for _, step := range ciSteps(t) {
		if step.Run == "" {
			continue
		}
		if !strings.Contains(skill, step.Name) {
			t.Errorf("the test skill does not name the CI step %q", step.Name)
		}
		for _, command := range ciCommands(step.Run) {
			if !strings.Contains(skill, command) {
				t.Errorf("the test skill does not hold the command %q of the CI step %q", command, step.Name)
			}
		}
	}
}

func TestTheTestSkillStatesThatTheDaemonDoesNotBuildOnMacOS(t *testing.T) {
	// cmd/hydrascale and the daemon packages use Pdeathsig, which macOS does not hold.
	skill := readSkill(t, "test")
	if strings.Contains(skill, "They all work on macOS") {
		t.Error("the test skill states that every command works on macOS")
	}
	if !strings.Contains(skill, "GOOS=linux") {
		t.Error("the test skill does not state the GOOS=linux build for a macOS machine")
	}
}

func TestTheRunSkillNamesNoMissingTest(t *testing.T) {
	// FR-refresh-25: each -run pattern of the skill names a test function of the repository.
	skill := readSkill(t, "run")
	var source strings.Builder
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "vendor" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, "_test.go") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			source.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the repository: %v", err)
	}
	for _, match := range regexp.MustCompile(`-run (\w+)`).FindAllStringSubmatch(skill, -1) {
		if !strings.Contains(source.String(), "func "+match[1]) {
			t.Errorf("the run skill names the test %s, which no test file holds", match[1])
		}
	}
}

func TestTheRunSkillNamesTheTestHostAndTheE2ESuite(t *testing.T) {
	// FR-refresh-25: no SSH alias named phobos exists, so the skill names the user and the
	// address.
	skill := readSkill(t, "run")
	for _, want := range []string{"phobos@192.168.1.221", "cd e2e", "npm run test:e2e", "HYDRASCALE_E2E_BASE_URL"} {
		if !strings.Contains(skill, want) {
			t.Errorf("the run skill does not hold %q", want)
		}
	}
	if regexp.MustCompile(`(?m)\sphobos$`).MatchString(skill) {
		t.Error("the run skill names the bare host phobos, which does not resolve")
	}
}

func TestTheCheckHygieneSkillStatesTheScriptRuns(t *testing.T) {
	// FR-refresh-27: the script exists and the CI step runs it.
	skill := readSkill(t, "check-hygiene")
	for _, stale := range []string{"Epic 1 adds", "Until that script exists"} {
		if strings.Contains(skill, stale) {
			t.Errorf("the check-hygiene skill still holds %q", stale)
		}
	}
	if _, err := os.Stat(filepath.Join("..", "scripts", "check-hygiene.sh")); err != nil {
		t.Fatalf("the script does not exist: %v", err)
	}
	var stepName string
	for _, step := range ciSteps(t) {
		if strings.TrimSpace(step.Run) == "scripts/check-hygiene.sh" {
			stepName = step.Name
		}
	}
	if stepName == "" {
		t.Fatal("no CI step runs scripts/check-hygiene.sh")
	}
	if !strings.Contains(skill, stepName) {
		t.Errorf("the check-hygiene skill does not name the CI step %q", stepName)
	}
}

func TestTheVerifySkillChecksEachV15Rule(t *testing.T) {
	// FR-refresh-28: each rule that version 1.5 writes on the host has one read-only check.
	skill := readSkill(t, "verify-on-phobos")
	for _, want := range []string{
		"sudo iptables -S HYDRASCALE-OUT",
		"sudo ip6tables -S HYDRASCALE-FWD",
		"sudo ip6tables -S HYDRASCALE-OUT",
		"sudo ip6tables -t nat -S POSTROUTING",
		"sudo iptables -t nat -S PREROUTING",
		"sudo ip6tables -t nat -S PREROUTING",
		"ip rule show",
		"force_forwarding",
		`HOST="${2:-phobos@192.168.1.221}"`,
	} {
		if !strings.Contains(skill, want) {
			t.Errorf("the verify-on-phobos skill does not hold %q", want)
		}
	}
}

func TestTheSkillsPageStatesEachSkillAndTheInstallRules(t *testing.T) {
	// FR-refresh-35: the page names each skill that the binary holds, the flag --dir, and
	// the refusal to run as root.
	data, err := os.ReadFile(filepath.Join("..", "docs", "site", "reference", "skills.md"))
	if err != nil {
		t.Fatalf("read the skills page: %v", err)
	}
	// The page wraps its lines, so the test compares the text with single spaces.
	page := strings.Join(strings.Fields(string(data)), " ")
	entries, err := os.ReadDir(filepath.Join("..", "skills"))
	if err != nil {
		t.Fatalf("read the skills directory: %v", err)
	}
	var names int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		names++
		if !strings.Contains(page, "`"+entry.Name()+"`") {
			t.Errorf("the skills page does not name the skill %s", entry.Name())
		}
	}
	if names == 0 {
		t.Fatal("the skills directory holds no skill, so the read is broken")
	}
	for _, want := range []string{"`--dir <path>`", "refuses to run as root"} {
		if !strings.Contains(page, want) {
			t.Errorf("the skills page does not hold %q", want)
		}
	}
}
