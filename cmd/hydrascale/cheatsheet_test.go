package main

import (
	"strings"
	"testing"
)

// cheatSheetName labels the cheat sheet in a drift message. The line number of the
// message counts the lines of the cheat sheet rather than the lines of the file.
const cheatSheetName = "the cheat sheet of cmd/hydrascale/init.go"

// hstnCommandForm is the form that runs one command through the function hstn. The cheat
// sheet states it on one line, and a test reads that line.
const hstnCommandForm = "hstn <cmd>"

// cheatSheetLineWith returns the line of the cheat sheet that holds text, and it returns
// an empty string when the cheat sheet holds no such line.
func cheatSheetLineWith(text string) string {
	for _, line := range strings.Split(cheatSheet(), "\n") {
		if strings.Contains(line, text) {
			return line
		}
	}
	return ""
}

// cheatSheetLine returns the line of the cheat sheet that states the command form
// "hydrascale <name>". name is the first word after "hydrascale". cheatSheetLine returns
// an empty string when the cheat sheet states no such form.
func cheatSheetLine(name string) string {
	for _, line := range strings.Split(cheatSheet(), "\n") {
		for _, ref := range commandReferences(line) {
			if len(ref.Path) > 0 && ref.Path[0] == name {
				return line
			}
		}
	}
	return ""
}

func TestTheCheatSheet(t *testing.T) {
	t.Run("states no command that the command tree does not hold", func(t *testing.T) {
		text := cheatSheet()
		refs := commandReferences(text)
		if len(refs) == 0 {
			t.Fatalf("the cheat sheet states no command form, so the test reads the wrong text")
		}
		for _, message := range commandDrift(rootCommand(), cheatSheetName, text) {
			t.Error(message)
		}
	})

	t.Run("names the shell function hstn beside hydrascale env", func(t *testing.T) {
		line := cheatSheetLine("env")
		if line == "" {
			t.Fatal("the cheat sheet states no form of hydrascale env")
		}
		if !strings.Contains(line, "hstn") {
			t.Errorf("the line = %q, want it to name the function %q", line, "hstn")
		}
	})

	t.Run("defines hstn without a root shell", func(t *testing.T) {
		// The function hstn calls sudo itself, so the operator runs the eval as
		// themselves. A root shell is an unnecessary step. See issue #263.
		line := cheatSheetLine("env")
		if line == "" {
			t.Fatal("the cheat sheet states no form of hydrascale env")
		}
		if strings.Contains(line, "sudo") {
			t.Errorf("the line = %q, want no %q, because the eval needs no root permission",
				line, "sudo")
		}
	})

	t.Run("names sudo beside the command form of hstn", func(t *testing.T) {
		// The line above states sudo hydrascale exec. The two forms need equal
		// privilege, so the sheet states that hstn elevates the command as well.
		line := cheatSheetLineWith(hstnCommandForm)
		if line == "" {
			t.Fatalf("the cheat sheet holds no line with %q", hstnCommandForm)
		}
		if !strings.Contains(line, "sudo") {
			t.Errorf("the line = %q, want it to name %q", line, "sudo")
		}
	})

	t.Run("describes hydrascale switch as a print of the namespace name", func(t *testing.T) {
		line := cheatSheetLine("switch")
		if line == "" {
			t.Fatal("the cheat sheet states no form of hydrascale switch")
		}
		if !strings.Contains(line, "namespace name") {
			t.Errorf("the line = %q, want it to name %q", line, "namespace name")
		}
	})

	t.Run("promises no default namespace for the shell", func(t *testing.T) {
		// hydrascale switch changes no state and the eval defines no namespace default.
		// A cheat sheet that holds the word "default" teaches a procedure that fails.
		// See FR-skills-1 and FR-skills-7.
		if strings.Contains(strings.ToLower(cheatSheet()), "default") {
			t.Errorf("the cheat sheet = %q, want no promise of a default", cheatSheet())
		}
	})
}
