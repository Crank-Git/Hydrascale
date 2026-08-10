package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/skills"
)

// runSkills runs the command `hydrascale skills` with the arguments that the test names.
// runSkills returns the text that the command printed and the error that it returned.
func runSkills(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := skillsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// asOperator points the effective user identifier at a value other than 0, so the command
// runs as the account of the operator rather than as root.
func asOperator(t *testing.T) {
	t.Helper()
	previous := skillsGeteuid
	skillsGeteuid = func() int { return 1000 }
	t.Cleanup(func() { skillsGeteuid = previous })
}

// asRoot points the effective user identifier at 0.
func asRoot(t *testing.T) {
	t.Helper()
	previous := skillsGeteuid
	skillsGeteuid = func() int { return 0 }
	t.Cleanup(func() { skillsGeteuid = previous })
}

// skillNames returns the name of each embedded skill.
func skillNames(t *testing.T) []string {
	t.Helper()
	set, err := skills.All()
	if err != nil {
		t.Fatalf("skills.All() = %v, want no error", err)
	}
	names := make([]string, 0, len(set))
	for _, skill := range set {
		names = append(names, skill.Name)
	}
	if len(names) == 0 {
		t.Fatal("skills.All() returned no skill, want at least one")
	}
	return names
}

func TestSkillsInstall(t *testing.T) {
	t.Run("writes each skill under the skill directory of HOME", func(t *testing.T) {
		asOperator(t)
		home := t.TempDir()
		t.Setenv("HOME", home)

		out, err := runSkills(t, "install")
		if err != nil {
			t.Fatalf("skills install = %v, want no error", err)
		}
		for _, name := range skillNames(t) {
			path := filepath.Join(home, ".claude", "skills", name, "SKILL.md")
			if _, statErr := os.Stat(path); statErr != nil {
				t.Errorf("os.Stat(%q) = %v, want no error. output = %q", path, statErr, out)
			}
		}
	})

	t.Run("writes the whole embedded file", func(t *testing.T) {
		asOperator(t)
		home := t.TempDir()
		t.Setenv("HOME", home)

		if _, err := runSkills(t, "install"); err != nil {
			t.Fatalf("skills install = %v, want no error", err)
		}
		set, err := skills.All()
		if err != nil {
			t.Fatalf("skills.All() = %v, want no error", err)
		}
		for _, skill := range set {
			path := filepath.Join(home, ".claude", "skills", skill.Name, "SKILL.md")
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("os.ReadFile(%q) = %v, want no error", path, readErr)
			}
			if string(got) != string(skill.Content) {
				t.Errorf("the file %s differs from the embedded skill %q", path, skill.Name)
			}
		}
	})

	t.Run("creates the skill directory at mode 0755 when it does not exist", func(t *testing.T) {
		asOperator(t)
		home := t.TempDir()
		t.Setenv("HOME", home)

		if _, err := runSkills(t, "install"); err != nil {
			t.Fatalf("skills install = %v, want no error", err)
		}
		base := filepath.Join(home, ".claude", "skills")
		info, err := os.Stat(base)
		if err != nil {
			t.Fatalf("os.Stat(%q) = %v, want no error", base, err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("the mode of %s = %o, want %o", base, info.Mode().Perm(), 0o755)
		}
	})

	t.Run("prints one line for each file that it wrote", func(t *testing.T) {
		asOperator(t)
		home := t.TempDir()
		t.Setenv("HOME", home)

		out, err := runSkills(t, "install")
		if err != nil {
			t.Fatalf("skills install = %v, want no error", err)
		}
		names := skillNames(t)
		lines := 0
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if strings.HasPrefix(line, "wrote ") {
				lines++
			}
		}
		if lines != len(names) {
			t.Errorf("the output holds %d lines that open with %q, want %d. output = %q",
				lines, "wrote ", len(names), out)
		}
		for _, name := range names {
			path := filepath.Join(home, ".claude", "skills", name, "SKILL.md")
			if !strings.Contains(out, "wrote "+path) {
				t.Errorf("output = %q, want it to contain %q", out, "wrote "+path)
			}
		}
	})

	t.Run("returns an error when the effective user identifier is zero", func(t *testing.T) {
		asRoot(t)
		t.Setenv("HOME", t.TempDir())

		if _, err := runSkills(t, "install"); err == nil {
			t.Fatal("skills install returned no error, want an error")
		}
	})

	t.Run("names the account that owns the skill in that error", func(t *testing.T) {
		asRoot(t)
		t.Setenv("HOME", t.TempDir())
		t.Setenv("SUDO_USER", "chris")

		_, err := runSkills(t, "install")
		if err == nil {
			t.Fatal("skills install returned no error, want an error")
		}
		if !strings.Contains(err.Error(), "chris") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "chris")
		}
		if !strings.Contains(err.Error(), "operator") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "operator")
		}
	})

	t.Run("names the operator in that error when the environment holds no SUDO_USER", func(t *testing.T) {
		asRoot(t)
		t.Setenv("HOME", t.TempDir())
		t.Setenv("SUDO_USER", "")

		_, err := runSkills(t, "install")
		if err == nil {
			t.Fatal("skills install returned no error, want an error")
		}
		if !strings.Contains(err.Error(), "operator") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "operator")
		}
	})

	t.Run("writes no file when the effective user identifier is zero", func(t *testing.T) {
		asRoot(t)
		home := t.TempDir()
		t.Setenv("HOME", home)

		if _, err := runSkills(t, "install"); err == nil {
			t.Fatal("skills install returned no error, want an error")
		}
		base := filepath.Join(home, ".claude")
		if _, err := os.Stat(base); err == nil {
			t.Errorf("os.Stat(%q) returned no error, want the directory to be absent", base)
		}
	})

	t.Run("returns an error that names HOME when the environment holds no HOME", func(t *testing.T) {
		asOperator(t)
		t.Setenv("HOME", "")

		_, err := runSkills(t, "install")
		if err == nil {
			t.Fatal("skills install returned no error, want an error")
		}
		if !strings.Contains(err.Error(), "HOME") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "HOME")
		}
	})

	t.Run("keeps a file that exists, prints the path, and returns no error", func(t *testing.T) {
		asOperator(t)
		home := t.TempDir()
		t.Setenv("HOME", home)

		name := skillNames(t)[0]
		path := filepath.Join(home, ".claude", "skills", name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("os.MkdirAll = %v, want no error", err)
		}
		const kept = "the file of the operator\n"
		if err := os.WriteFile(path, []byte(kept), 0o644); err != nil {
			t.Fatalf("os.WriteFile(%q) = %v, want no error", path, err)
		}

		out, err := runSkills(t, "install")
		if err != nil {
			t.Fatalf("skills install = %v, want no error", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("os.ReadFile(%q) = %v, want no error", path, err)
		}
		if string(got) != kept {
			t.Errorf("the file %s = %q, want %q", path, string(got), kept)
		}
		if !strings.Contains(out, path) {
			t.Errorf("output = %q, want it to contain %q", out, path)
		}
	})

	t.Run("writes a file that exists when force is set", func(t *testing.T) {
		asOperator(t)
		home := t.TempDir()
		t.Setenv("HOME", home)

		name := skillNames(t)[0]
		path := filepath.Join(home, ".claude", "skills", name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("os.MkdirAll = %v, want no error", err)
		}
		if err := os.WriteFile(path, []byte("the file of the operator\n"), 0o644); err != nil {
			t.Fatalf("os.WriteFile(%q) = %v, want no error", path, err)
		}

		if _, err := runSkills(t, "install", "--force"); err != nil {
			t.Fatalf("skills install --force = %v, want no error", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("os.ReadFile(%q) = %v, want no error", path, err)
		}
		if string(got) == "the file of the operator\n" {
			t.Errorf("the file %s holds the old content, want the embedded skill", path)
		}
	})

	t.Run("prints each path and writes no file when dry run is set", func(t *testing.T) {
		asOperator(t)
		home := t.TempDir()
		t.Setenv("HOME", home)

		out, err := runSkills(t, "install", "--dry-run")
		if err != nil {
			t.Fatalf("skills install --dry-run = %v, want no error", err)
		}
		for _, name := range skillNames(t) {
			path := filepath.Join(home, ".claude", "skills", name, "SKILL.md")
			if !strings.Contains(out, path) {
				t.Errorf("output = %q, want it to contain %q", out, path)
			}
			if _, statErr := os.Stat(path); statErr == nil {
				t.Errorf("os.Stat(%q) returned no error, want the file to be absent", path)
			}
		}
		base := filepath.Join(home, ".claude")
		if _, statErr := os.Stat(base); statErr == nil {
			t.Errorf("os.Stat(%q) returned no error, want the directory to be absent", base)
		}
	})

	t.Run("writes to the directory that dir names", func(t *testing.T) {
		asOperator(t)
		home := t.TempDir()
		t.Setenv("HOME", home)
		target := filepath.Join(t.TempDir(), "elsewhere")

		if _, err := runSkills(t, "install", "--dir", target); err != nil {
			t.Fatalf("skills install --dir = %v, want no error", err)
		}
		for _, name := range skillNames(t) {
			path := filepath.Join(target, name, "SKILL.md")
			if _, statErr := os.Stat(path); statErr != nil {
				t.Errorf("os.Stat(%q) = %v, want no error", path, statErr)
			}
		}
		underHome := filepath.Join(home, ".claude")
		if _, statErr := os.Stat(underHome); statErr == nil {
			t.Errorf("os.Stat(%q) returned no error, want the directory to be absent", underHome)
		}
	})

	t.Run("writes to the directory that dir names when the environment holds no HOME", func(t *testing.T) {
		asOperator(t)
		t.Setenv("HOME", "")
		target := filepath.Join(t.TempDir(), "elsewhere")

		if _, err := runSkills(t, "install", "--dir", target); err != nil {
			t.Fatalf("skills install --dir = %v, want no error", err)
		}
		for _, name := range skillNames(t) {
			path := filepath.Join(target, name, "SKILL.md")
			if _, statErr := os.Stat(path); statErr != nil {
				t.Errorf("os.Stat(%q) = %v, want no error", path, statErr)
			}
		}
	})
}

func TestSkillsList(t *testing.T) {
	t.Run("prints the name and the description of each embedded skill", func(t *testing.T) {
		set, err := skills.All()
		if err != nil {
			t.Fatalf("skills.All() = %v, want no error", err)
		}
		out, err := runSkills(t, "list")
		if err != nil {
			t.Fatalf("skills list = %v, want no error", err)
		}
		for _, skill := range set {
			if !strings.Contains(out, skill.Name) {
				t.Errorf("output = %q, want it to contain %q", out, skill.Name)
			}
			if !strings.Contains(out, skill.Description) {
				t.Errorf("output = %q, want it to contain the description of %q", out, skill.Name)
			}
		}
	})

	t.Run("prints one line for each embedded skill", func(t *testing.T) {
		out, err := runSkills(t, "list")
		if err != nil {
			t.Fatalf("skills list = %v, want no error", err)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != len(skillNames(t)) {
			t.Errorf("the output holds %d lines, want %d. output = %q",
				len(lines), len(skillNames(t)), out)
		}
	})

	t.Run("writes no file", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if _, err := runSkills(t, "list"); err != nil {
			t.Fatalf("skills list = %v, want no error", err)
		}
		base := filepath.Join(home, ".claude")
		if _, err := os.Stat(base); err == nil {
			t.Errorf("os.Stat(%q) returned no error, want the directory to be absent", base)
		}
	})
}

func TestSkillsCommandRegistration(t *testing.T) {
	t.Run("the root command holds the command skills", func(t *testing.T) {
		found := false
		for _, sub := range rootCommand().Commands() {
			if sub.Name() == "skills" {
				found = true
			}
		}
		if !found {
			t.Errorf("the root command holds no command named %q", "skills")
		}
	})

	t.Run("the command skills holds install and list", func(t *testing.T) {
		want := map[string]bool{"install": false, "list": false}
		for _, sub := range skillsCmd().Commands() {
			if _, ok := want[sub.Name()]; ok {
				want[sub.Name()] = true
			}
		}
		for name, found := range want {
			if !found {
				t.Errorf("the command skills holds no sub-command named %q", name)
			}
		}
	})
}
