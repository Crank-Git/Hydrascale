package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"hydrascale/skills"
)

// skillCommandPrefix opens each command form that a skill file states. The prefix holds
// the trailing space, so the path /etc/hydrascale/config.yaml matches nothing.
const skillCommandPrefix = "hydrascale "

// commandReference is one command form that a skill file states.
type commandReference struct {
	// Path holds the words after "hydrascale ", up to the first word that names no
	// command. The first word of Path names a sub-command of the root command.
	Path []string
	// Line is the one-based line of the skill file that states the form.
	Line int
	// Raw is the text of the line, and a failure message quotes it.
	Raw string
}

// commandReferences returns each command form that content states, in the order of the
// file. content is the whole skill file, so the result holds a form of a fenced code block
// and a form of prose together.
//
// commandReferences reads the lower-case word "hydrascale" alone, because a command is
// lower case. It reads no form where a letter, a digit, a dash, or a slash comes before
// that word.
func commandReferences(content string) []commandReference {
	var refs []commandReference
	for i, line := range strings.Split(content, "\n") {
		rest := line
		offset := 0
		for {
			at := strings.Index(rest, skillCommandPrefix)
			if at < 0 {
				break
			}
			start := offset + at
			after := rest[at+len(skillCommandPrefix):]
			rest = after
			offset = start + len(skillCommandPrefix)
			if start > 0 && joinsTheWordBefore(line[start-1]) {
				continue
			}
			path := commandPath(after)
			if len(path) == 0 {
				continue
			}
			refs = append(refs, commandReference{
				Path: path,
				Line: i + 1,
				Raw:  strings.TrimSpace(line),
			})
		}
	}
	return refs
}

// joinsTheWordBefore reports whether b joins the word "hydrascale" to the text before it.
// A path such as /etc/hydrascale/config.yaml names no command, and neither does the skill
// name hydrascale-setup.
func joinsTheWordBefore(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '-', b == '/', b == '_':
		return true
	}
	return false
}

// commandPath returns the words of text that can name a command. text is the part of the
// line after "hydrascale ".
//
// commandPath stops at a flag, at a placeholder such as <id>, and at the first word that
// holds no command character. A word carries the characters a-z, 0-9, and the dash.
// commandPath cuts the punctuation that follows a word, and it stops there, because a
// backtick or a full stop closes the command form and the prose continues after it.
func commandPath(text string) []string {
	var path []string
	for _, field := range strings.Fields(text) {
		if strings.HasPrefix(field, "-") {
			break
		}
		word := commandWord(field)
		if word == "" {
			break
		}
		path = append(path, word)
		if word != field {
			break
		}
	}
	return path
}

// commandWord returns the leading command characters of field, and it returns an empty
// string when field opens with another character. A trailing backtick, colon, comma, or
// full stop therefore falls away.
func commandWord(field string) string {
	end := 0
	for end < len(field) {
		c := field[end]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			end++
			continue
		}
		break
	}
	return field[:end]
}

// commandDrift returns one message for each command form of content that root does not
// hold. file names the skill file, and each message opens with that name and the line.
// commandDrift returns an empty result when the command tree holds every form.
//
// commandDrift walks the command tree word by word. It stops at the first command that
// holds no sub-command, because every later word is an argument rather than a command.
func commandDrift(root *cobra.Command, file, content string) []string {
	var messages []string
	for _, ref := range commandReferences(content) {
		cmd := root
		for _, word := range ref.Path {
			if len(cmd.Commands()) == 0 {
				break
			}
			child := subCommand(cmd, word)
			if child == nil {
				messages = append(messages, fmt.Sprintf(
					"%s:%d states %q, and the command %q holds no command named %q",
					file, ref.Line, ref.Raw, cmd.CommandPath(), word))
				break
			}
			cmd = child
		}
	}
	return messages
}

// subCommand returns the sub-command of parent that word names, and it returns nil when
// parent holds none. subCommand reads an alias as well as a name.
func subCommand(parent *cobra.Command, word string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == word {
			return child
		}
		if child.HasAlias(word) {
			return child
		}
	}
	return nil
}

// skillFilePath returns the path of the skill file that the name identifies. A failure
// message quotes this path, so the operator opens the file that states the command.
func skillFilePath(name string) string {
	return "skills/" + name + "/SKILL.md"
}

func TestTheSkillSetStatesNoUnknownCommand(t *testing.T) {
	t.Run("states no command that the command tree does not hold", func(t *testing.T) {
		set, err := skills.All()
		if err != nil {
			t.Fatalf("skills.All() = %v, want no error", err)
		}
		root := rootCommand()
		for _, skill := range set {
			file := skillFilePath(skill.Name)
			refs := commandReferences(string(skill.Content))
			if len(refs) == 0 {
				t.Errorf("%s states no command form, so the test reads the wrong file", file)
			}
			for _, message := range commandDrift(root, file, string(skill.Content)) {
				t.Error(message)
			}
		}
	})
}

func TestCommandReferences(t *testing.T) {
	t.Run("reads a command of a fenced code block", func(t *testing.T) {
		content := "# A skill\n\n```sh\nhydrascale ping personal peer\n```\n"
		refs := commandReferences(content)
		if len(refs) != 1 {
			t.Fatalf("commandReferences() returned %d forms, want 1. forms = %+v", len(refs), refs)
		}
		if refs[0].Path[0] != "ping" {
			t.Errorf("the first word = %q, want %q", refs[0].Path[0], "ping")
		}
		if refs[0].Line != 4 {
			t.Errorf("the line = %d, want %d", refs[0].Line, 4)
		}
	})

	t.Run("reads a command of prose", func(t *testing.T) {
		content := "Run `hydrascale list`. It prints the identifier of each tailnet.\n"
		refs := commandReferences(content)
		if len(refs) != 1 {
			t.Fatalf("commandReferences() returned %d forms, want 1. forms = %+v", len(refs), refs)
		}
		if refs[0].Path[0] != "list" {
			t.Errorf("the first word = %q, want %q", refs[0].Path[0], "list")
		}
	})

	t.Run("reads two commands of one line", func(t *testing.T) {
		content := "Read `hydrascale list` and then `hydrascale status`.\n"
		refs := commandReferences(content)
		if len(refs) != 2 {
			t.Fatalf("commandReferences() returned %d forms, want 2. forms = %+v", len(refs), refs)
		}
		if refs[0].Path[0] != "list" || refs[1].Path[0] != "status" {
			t.Errorf("the words = %q and %q, want %q and %q",
				refs[0].Path[0], refs[1].Path[0], "list", "status")
		}
	})

	t.Run("reads a sub-command as the second word", func(t *testing.T) {
		refs := commandReferences("Run `hydrascale skills install`.\n")
		if len(refs) != 1 {
			t.Fatalf("commandReferences() returned %d forms, want 1. forms = %+v", len(refs), refs)
		}
		want := []string{"skills", "install"}
		if strings.Join(refs[0].Path, " ") != strings.Join(want, " ") {
			t.Errorf("the path = %q, want %q", refs[0].Path, want)
		}
	})

	t.Run("reads no command in a file path", func(t *testing.T) {
		if refs := commandReferences("The file /etc/hydrascale/config.yaml holds it.\n"); len(refs) != 0 {
			t.Errorf("commandReferences() returned %+v, want no form", refs)
		}
	})

	t.Run("stops at a placeholder", func(t *testing.T) {
		refs := commandReferences("Run `hydrascale env <id>`.\n")
		if len(refs) != 1 {
			t.Fatalf("commandReferences() returned %d forms, want 1. forms = %+v", len(refs), refs)
		}
		if len(refs[0].Path) != 1 {
			t.Errorf("the path = %q, want one word", refs[0].Path)
		}
	})

	t.Run("stops at a flag", func(t *testing.T) {
		refs := commandReferences("Run `hydrascale apply --dry-run`.\n")
		if len(refs) != 1 {
			t.Fatalf("commandReferences() returned %d forms, want 1. forms = %+v", len(refs), refs)
		}
		if len(refs[0].Path) != 1 {
			t.Errorf("the path = %q, want one word", refs[0].Path)
		}
	})
}

func TestCommandDrift(t *testing.T) {
	const file = "skills/demo/SKILL.md"

	t.Run("reports a command that the command tree does not hold", func(t *testing.T) {
		messages := commandDrift(rootCommand(), file, "Run `hydrascale frobnicate` first.\n")
		if len(messages) != 1 {
			t.Fatalf("commandDrift() returned %d messages, want 1. messages = %q", len(messages), messages)
		}
		if !strings.Contains(messages[0], file) {
			t.Errorf("the message = %q, want it to name the file %q", messages[0], file)
		}
		if !strings.Contains(messages[0], "frobnicate") {
			t.Errorf("the message = %q, want it to name the command %q", messages[0], "frobnicate")
		}
	})

	t.Run("reports the line of the command", func(t *testing.T) {
		messages := commandDrift(rootCommand(), file, "# A skill\n\nRun `hydrascale frobnicate`.\n")
		if len(messages) != 1 {
			t.Fatalf("commandDrift() returned %d messages, want 1. messages = %q", len(messages), messages)
		}
		if !strings.Contains(messages[0], file+":3") {
			t.Errorf("the message = %q, want it to name %q", messages[0], file+":3")
		}
	})

	t.Run("reports a sub-command that the command tree does not hold", func(t *testing.T) {
		messages := commandDrift(rootCommand(), file, "Run `hydrascale skills frobnicate`.\n")
		if len(messages) != 1 {
			t.Fatalf("commandDrift() returned %d messages, want 1. messages = %q", len(messages), messages)
		}
		if !strings.Contains(messages[0], "hydrascale skills") {
			t.Errorf("the message = %q, want it to name the parent command %q",
				messages[0], "hydrascale skills")
		}
	})

	t.Run("reports no message for a command that the command tree holds", func(t *testing.T) {
		content := "Run `hydrascale status`, then `hydrascale skills install`.\n"
		if messages := commandDrift(rootCommand(), file, content); len(messages) != 0 {
			t.Errorf("commandDrift() returned %q, want no message", messages)
		}
	})

	t.Run("reads an argument of a command as no command", func(t *testing.T) {
		content := "Run `hydrascale ping personal peer`.\n"
		if messages := commandDrift(rootCommand(), file, content); len(messages) != 0 {
			t.Errorf("commandDrift() returned %q, want no message", messages)
		}
	})

	t.Run("reads a command of the first word after a wrapper command", func(t *testing.T) {
		content := "Run `ssh host hydrascale list` from another machine.\n"
		if messages := commandDrift(rootCommand(), file, content); len(messages) != 0 {
			t.Errorf("commandDrift() returned %q, want no message", messages)
		}
	})
}
