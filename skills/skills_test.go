package skills

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
)

// skillTests holds one entry for each skill directory. An entry names the content tests
// of that skill. The compiler checks each name, so a renamed test changes this table too.
// A new skill directory without an entry fails TestEachSkillHasATestEntry.
// The test reads the embedded set, not the working directory, because the gate runs this
// binary from another directory.
var skillTests = map[string][]func(*testing.T){
	"hydrascale-setup": {
		TestTheSetupSkillStatesSudo,
		TestTheSetupSkillAllowsEachReadOnlyCommand,
		TestTheSetupSkillStatesEachStatusValue,
		TestTheSetupSkillNamesEachHostChange,
		TestTheSetupSkillStatesTheConfigurationKeys,
		TestTheSetupSkillStatesTheUpgradeInASectionOfItsOwn,
		TestTheSetupSkillStatesTheTunnelOnAnyLocalPort,
		TestTheSetupSkillNamesNoSourceLine,
	},
	"hydrascale-troubleshoot": {
		TestTheTroubleshootSkillIsEmbedded,
		TestTheTroubleshootSkillHoldsSixDiagnoses,
		TestTheTroubleshootSkillAllowsNoHostChange,
		TestTheTroubleshootSkillNamesEachEvent,
		TestTheTroubleshootSkillDiagnosesIPv6,
		TestTheTroubleshootSkillDiagnosesADirectConnection,
		TestTheTroubleshootSkillDiagnosesDNS,
		TestTheTroubleshootSkillDiagnosesADisplacedJumpRule,
		TestTheTroubleshootSkillDiagnosesARejectedCredential,
		TestTheTroubleshootSkillDiagnosesARefusedPortOfTheHost,
		TestTheTroubleshootSkillStatesTheResultWithNoFault,
		TestTheTroubleshootSkillNamesNoSourceLine,
	},
	"tailnet-exec": {
		TestTheTailnetExecSkillStatesRootForEachForm,
		TestTheTailnetExecSkillStatesTheStandaloneStatus,
		TestTheTailnetExecSkillStatesTheAlias,
		TestTheTailnetExecSkillStatesTheAliasZone,
		TestTheTailnetExecSkillStatesWrapApply,
		TestTheTailnetExecSkillLinksTheCommandLinePage,
	},
}

// skillsWithNoTest returns each name of dirs that tests holds no test for, in the order of
// dirs. An entry with an empty list counts as no entry.
func skillsWithNoTest(dirs []string, tests map[string][]func(*testing.T)) []string {
	var missing []string
	for _, dir := range dirs {
		if len(tests[dir]) == 0 {
			missing = append(missing, dir)
		}
	}
	return missing
}

func TestEachSkillHasATestEntry(t *testing.T) {
	stub := func(*testing.T) {}

	t.Run("reports a skill directory that holds no test entry", func(t *testing.T) {
		got := skillsWithNoTest([]string{"demo", "new-skill"}, map[string][]func(*testing.T){"demo": {stub}})
		if strings.Join(got, ",") != "new-skill" {
			t.Errorf("skillsWithNoTest() = %q, want %q", got, []string{"new-skill"})
		}
	})

	t.Run("reports a skill directory whose test entry holds no test", func(t *testing.T) {
		got := skillsWithNoTest([]string{"demo"}, map[string][]func(*testing.T){"demo": nil})
		if strings.Join(got, ",") != "demo" {
			t.Errorf("skillsWithNoTest() = %q, want %q", got, []string{"demo"})
		}
	})

	t.Run("holds a test entry for each directory of the embedded set", func(t *testing.T) {
		entries, err := fs.ReadDir(files, ".")
		if err != nil {
			t.Fatalf("fs.ReadDir() = %v, want no error", err)
		}
		var dirs []string
		for _, entry := range entries {
			if entry.IsDir() {
				dirs = append(dirs, entry.Name())
			}
		}
		if len(dirs) == 0 {
			t.Fatal("the embedded set holds no directory, so the test reads the wrong set")
		}
		for _, name := range skillsWithNoTest(dirs, skillTests) {
			t.Errorf("skills/%s holds no entry in skillTests. Write a content test and add it to the table", name)
		}
		for name := range skillTests {
			if !slices.Contains(dirs, name) {
				t.Errorf("skillTests holds an entry for %q, and the embedded set holds no such directory", name)
			}
		}
	})
}

func TestAll(t *testing.T) {
	t.Run("returns one skill for each directory of the skill set", func(t *testing.T) {
		set, err := All()
		if err != nil {
			t.Fatalf("All() = %v, want no error", err)
		}
		if len(set) != len(skillTests) {
			t.Errorf("len(All()) = %d, want %d", len(set), len(skillTests))
		}
	})

	t.Run("returns a name and a description for each skill", func(t *testing.T) {
		set, err := All()
		if err != nil {
			t.Fatalf("All() = %v, want no error", err)
		}
		for _, skill := range set {
			if skill.Name == "" {
				t.Errorf("skill %+v holds an empty name", skill)
			}
			if skill.Description == "" {
				t.Errorf("skill %q holds an empty description", skill.Name)
			}
		}
	})

	t.Run("returns each skill that the repository holds", func(t *testing.T) {
		set, err := All()
		if err != nil {
			t.Fatalf("All() = %v, want no error", err)
		}
		for want := range skillTests {
			found := false
			for _, skill := range set {
				if skill.Name == want {
					found = true
				}
			}
			if !found {
				t.Errorf("All() holds no skill named %q", want)
			}
		}
	})

	t.Run("returns the whole file as the content of the skill", func(t *testing.T) {
		set, err := All()
		if err != nil {
			t.Fatalf("All() = %v, want no error", err)
		}
		for _, skill := range set {
			want := "---\nname: " + skill.Name + "\n"
			if !strings.HasPrefix(string(skill.Content), want) {
				t.Errorf("the content of %q does not open with %q", skill.Name, want)
			}
			if !strings.Contains(string(skill.Content), skill.Description) {
				t.Errorf("the content of %q holds no description", skill.Name)
			}
			if !strings.Contains(string(skill.Content), "\n# ") {
				t.Errorf("the content of %q holds no heading, so it is not the whole file", skill.Name)
			}
		}
	})

	t.Run("returns the skills in the order of the name", func(t *testing.T) {
		set, err := All()
		if err != nil {
			t.Fatalf("All() = %v, want no error", err)
		}
		for i := 1; i < len(set); i++ {
			if set[i-1].Name >= set[i].Name {
				t.Errorf("All()[%d].Name = %q and All()[%d].Name = %q, want ascending order",
					i-1, set[i-1].Name, i, set[i].Name)
			}
		}
	})
}

func TestFrontMatter(t *testing.T) {
	t.Run("reads the name and the description", func(t *testing.T) {
		content := []byte("---\nname: demo\ndescription: A skill of the test.\n---\n\n# Demo\n")
		name, description, err := frontMatter(content)
		if err != nil {
			t.Fatalf("frontMatter() = %v, want no error", err)
		}
		if name != "demo" {
			t.Errorf("name = %q, want %q", name, "demo")
		}
		if description != "A skill of the test." {
			t.Errorf("description = %q, want %q", description, "A skill of the test.")
		}
	})

	t.Run("returns an error when the file opens with no delimiter", func(t *testing.T) {
		if _, _, err := frontMatter([]byte("# Demo\n")); err == nil {
			t.Error("frontMatter() returned no error, want an error")
		}
	})

	t.Run("returns an error when the front matter holds no end delimiter", func(t *testing.T) {
		if _, _, err := frontMatter([]byte("---\nname: demo\n")); err == nil {
			t.Error("frontMatter() returned no error, want an error")
		}
	})

	t.Run("returns an error that names the key when the name is absent", func(t *testing.T) {
		_, _, err := frontMatter([]byte("---\ndescription: A skill.\n---\n"))
		if err == nil {
			t.Fatal("frontMatter() returned no error, want an error")
		}
		if !strings.Contains(err.Error(), "name") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "name")
		}
	})

	t.Run("returns an error that names the key when the description is absent", func(t *testing.T) {
		_, _, err := frontMatter([]byte("---\nname: demo\n---\n"))
		if err == nil {
			t.Fatal("frontMatter() returned no error, want an error")
		}
		if !strings.Contains(err.Error(), "description") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "description")
		}
	})

	t.Run("keeps a colon that the description holds", func(t *testing.T) {
		content := []byte("---\nname: demo\ndescription: Use when: the host runs.\n---\n")
		_, description, err := frontMatter(content)
		if err != nil {
			t.Fatalf("frontMatter() = %v, want no error", err)
		}
		if description != "Use when: the host runs." {
			t.Errorf("description = %q, want %q", description, "Use when: the host runs.")
		}
	})
}
