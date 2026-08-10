package skills

import (
	"strings"
	"testing"
)

// The test names the skills that the set holds today. The test reads no file of the
// working directory, because the gate runs this binary from another directory.
var knownSkills = []string{"hydrascale-setup", "tailnet-exec"}

func TestAll(t *testing.T) {
	t.Run("returns one skill for each directory of the skill set", func(t *testing.T) {
		set, err := All()
		if err != nil {
			t.Fatalf("All() = %v, want no error", err)
		}
		if len(set) < len(knownSkills) {
			t.Errorf("len(All()) = %d, want %d or more", len(set), len(knownSkills))
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
		for _, want := range knownSkills {
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
