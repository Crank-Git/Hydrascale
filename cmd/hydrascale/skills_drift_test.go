package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/spf13/cobra"

	"hydrascale/internal/docscheck"
	"hydrascale/internal/secrets"
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
	// Flags holds each flag that the form states, such as --dry-run. See formFlags.
	Flags []string
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
				Path:  path,
				Line:  i + 1,
				Raw:   strings.TrimSpace(line),
				Flags: formFlags(after),
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
// commandDrift walks the command tree with resolveCommand.
func commandDrift(root *cobra.Command, file, content string) []string {
	var messages []string
	for _, ref := range commandReferences(content) {
		if cmd, missing := resolveCommand(root, ref.Path); missing != "" {
			messages = append(messages, fmt.Sprintf(
				"%s:%d states %q, and the command %q holds no command named %q",
				file, ref.Line, ref.Raw, cmd.CommandPath(), missing))
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

// formEnd holds each shell word that closes a command form of hydrascale. The separator
// "--" opens the arguments of the command that exec or tailscale runs, and a pipe or a
// list operator opens another command. A later word is not a word of hydrascale.
var formEnd = map[string]bool{"--": true, "|": true, "||": true, "&&": true, ";": true, ">": true, ">>": true}

// formFlags returns each flag of the command form that text opens. text is the part of
// the line after "hydrascale ". The form ends at the first backtick, at the first
// parenthesis, at the end of the line, or at a word of formEnd. A parenthesis closes a
// form of the front matter key allowed-tools, such as Bash(hydrascale status:*).
// A flag is a word that opens with a dash and holds a character after it. formFlags drops
// a value after "=".
func formFlags(text string) []string {
	if end := strings.IndexAny(text, "`()"); end >= 0 {
		text = text[:end]
	}
	var flags []string
	for _, word := range strings.Fields(text) {
		if formEnd[word] {
			break
		}
		if !strings.HasPrefix(word, "-") || word == "-" {
			continue
		}
		word, _, _ = strings.Cut(word, "=")
		flags = append(flags, strings.TrimRight(word, ".,:;)"))
	}
	return flags
}

// resolveCommand returns the command of root that path names, and an empty string. When a
// word of path names no sub-command, resolveCommand returns the parent command and that
// word. resolveCommand stops at the first command that holds no sub-command, because every
// later word is an argument rather than a command.
func resolveCommand(root *cobra.Command, path []string) (*cobra.Command, string) {
	cmd := root
	for _, word := range path {
		if len(cmd.Commands()) == 0 {
			break
		}
		child := subCommand(cmd, word)
		if child == nil {
			return cmd, word
		}
		cmd = child
	}
	return cmd, ""
}

// declaresFlag reports whether cmd accepts flag. flag is the word that a skill states,
// such as --dry-run or -h. cmd.Flag also finds a persistent flag of a parent, such as
// --config. Every command accepts --help and -h, because cobra adds them when it runs.
// declaresFlag reads the first letter of a short flag, because a short flag can carry
// its value in the same word.
func declaresFlag(cmd *cobra.Command, flag string) bool {
	if flag == "--help" || flag == "-h" {
		return true
	}
	if name, long := strings.CutPrefix(flag, "--"); long {
		return cmd.Flag(name) != nil
	}
	short := flag[1:2]
	return cmd.Flags().ShorthandLookup(short) != nil || cmd.InheritedFlags().ShorthandLookup(short) != nil
}

// flagDrift returns one message for each flag of content that its command does not
// declare. file names the skill file, and each message opens with that name and the line.
// flagDrift reads a flag only in a command form that opens with "hydrascale ". It skips a
// form whose command the tree does not hold, because commandDrift reports that form.
// See FR-refresh-29.
func flagDrift(root *cobra.Command, file, content string) []string {
	var messages []string
	for _, ref := range commandReferences(content) {
		cmd, missing := resolveCommand(root, ref.Path)
		if missing != "" {
			continue
		}
		for _, flag := range ref.Flags {
			if !declaresFlag(cmd, flag) {
				messages = append(messages, fmt.Sprintf(
					"%s:%d states %q, and the command %q declares no flag %s",
					file, ref.Line, ref.Raw, cmd.CommandPath(), flag))
			}
		}
	}
	return messages
}

// sourceLine matches a reference to a line of a Go file, such as internal/api/console.go:16.
var sourceLine = regexp.MustCompile(`[A-Za-z0-9_./-]+\.go:[0-9]+`)

// sourceLineDrift returns one message for each source line that content names. file names
// the skill file, and each message opens with that name and the line. A line number moves
// with each edit of the source file. See FR-refresh-33.
func sourceLineDrift(file, content string) []string {
	var messages []string
	for i, line := range strings.Split(content, "\n") {
		for _, ref := range sourceLine.FindAllString(line, -1) {
			messages = append(messages, fmt.Sprintf(
				"%s:%d names the source line %s, which moves with each edit. Name the file or the function",
				file, i+1, ref))
		}
	}
	return messages
}

// knownNames holds each name that a backtick span of a skill can state.
type knownNames struct {
	// names holds each configuration key, each event type, and each other name of the
	// daemon.
	names map[string]bool
	// roots holds the first segment of each configuration key and each event type, such
	// as access. A dotted span with another first segment, such as resolv.conf, names
	// nothing of the daemon.
	roots map[string]bool
}

// newKnownNames returns the names that nameDrift accepts. configKeys is the result of
// docscheck.ConfigKeys, and eventTypes is the result of docscheck.EventTypes. others holds
// each name of the daemon that is neither a configuration key nor an event type, such as
// a key of the secrets file.
func newKnownNames(configKeys, eventTypes, others []string) knownNames {
	known := knownNames{names: map[string]bool{}, roots: map[string]bool{}}
	for _, list := range [][]string{configKeys, eventTypes, others} {
		for _, name := range list {
			known.names[name] = true
		}
	}
	for _, list := range [][]string{configKeys, eventTypes} {
		for _, name := range list {
			known.roots[nameRoot(name)] = true
		}
	}
	return known
}

// nameRoot returns the first segment of a dotted name, without the list marker [].
func nameRoot(name string) string {
	root, _, _ := strings.Cut(name, ".")
	return strings.TrimSuffix(root, "[]")
}

// nameShape matches the shape of a configuration key and of an event type: lower-case
// segments of letters, digits, and underscores, a dot between two segments, and the list
// marker [] after a segment.
var nameShape = regexp.MustCompile(`^[a-z][a-z0-9_]*(\[\])?(\.[a-z][a-z0-9_]*(\[\])?)*$`)

// isCandidate reports whether span states a configuration key or an event type.
// A candidate has the shape of nameShape, and it holds an underscore, a dot, or the list
// marker. A plain word such as observe is therefore no candidate.
// A dotted candidate opens with the first segment of a known name, so that
// net.ipv6.conf.all.forwarding is no candidate. A span with no dot is always a candidate.
// A short name such as force_forwarding therefore fails, and a skill states the full name
// of a kernel parameter.
func (k knownNames) isCandidate(span string) bool {
	if !nameShape.MatchString(span) || !strings.ContainsAny(span, "_.[") {
		return false
	}
	if !strings.Contains(span, ".") {
		return true
	}
	return k.roots[nameRoot(span)]
}

// nameDrift returns one message for each backtick span of content that states a
// configuration key or an event type that names does not hold. file names the skill
// file, and each message opens with that name and the line. nameDrift reads the text of a
// span before its first colon, so `access.mode: observe` states access.mode. nameDrift
// reads no line of a fenced code block, because a code block shows a file or an output.
// See FR-refresh-30 and FR-refresh-31.
func nameDrift(file, content string, names knownNames) []string {
	var messages []string
	fenced := false
	for i, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		parts := strings.Split(line, "`")
		// A part at an odd index lies between two backticks. The last part lies after
		// the last backtick, so it is no span even at an odd index.
		for at := 1; at < len(parts)-1; at += 2 {
			span, _, _ := strings.Cut(parts[at], ":")
			span = strings.TrimSpace(span)
			if !names.isCandidate(span) || names.names[span] {
				continue
			}
			messages = append(messages, fmt.Sprintf(
				"%s:%d states `%s`, which is no configuration key, no event type, and no other name of the daemon",
				file, i+1, span))
		}
	}
	return messages
}

// siteURL opens every link of a skill to the documentation site.
const siteURL = "https://crank-git.github.io/Hydrascale/"

// siteLink matches a link to the documentation site. The group holds the path of the
// page, up to an anchor, a space, or a character that closes a link in Markdown.
var siteLink = regexp.MustCompile(regexp.QuoteMeta(siteURL) + "([^\\s#)>`\"]*)")

// sitePages returns each file under docs/site/ that MkDocs publishes at path. path is the
// part of a link after siteURL, such as reference/command-line/.
func sitePages(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return []string{"index.md"}
	}
	return []string{path + ".md", path + "/index.md"}
}

// siteLinkDrift returns one message for each link of content to a page that site does
// not hold, and one message for each line that names a path under docs/site/. site is
// the directory docs/site. A skill runs on a host that holds no checkout, so only the
// absolute URL reaches the page. See FR-refresh-34.
func siteLinkDrift(file, content string, site fs.FS) []string {
	var messages []string
	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "docs/site/") {
			messages = append(messages, fmt.Sprintf(
				"%s:%d names a path under docs/site/. Link the page under %s", file, i+1, siteURL))
		}
		for _, match := range siteLink.FindAllStringSubmatch(line, -1) {
			path := strings.TrimRight(match[1], ".,:;")
			found := false
			for _, page := range sitePages(path) {
				if _, err := fs.Stat(site, page); err == nil {
					found = true
				}
			}
			if !found {
				messages = append(messages, fmt.Sprintf(
					"%s:%d links %s%s, and docs/site/ holds no page %s",
					file, i+1, siteURL, path, strings.Join(sitePages(path), " or ")))
			}
		}
	}
	return messages
}

// repositoryRoot returns the root of the checkout. go test runs a package from the
// directory of that package, so the root is two levels up. repositoryRoot fails the test
// when the root holds no internal and no docs/site, because a check that reads no file
// passes on any drift.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..")
	for _, dir := range []string{"internal", filepath.Join("docs", "site")} {
		info, err := os.Stat(filepath.Join(root, dir))
		if err != nil || !info.IsDir() {
			t.Fatalf("%s holds no directory %s, so the drift test cannot read the tree", root, dir)
		}
	}
	return root
}

// daemonNames returns each name that a skill can state in backticks: each configuration
// key, each event type, each key of the secrets file, and the control API field that a
// skill reads.
func daemonNames(t *testing.T, root string) knownNames {
	t.Helper()
	events, err := docscheck.EventTypes(root)
	if err != nil {
		t.Fatalf("docscheck.EventTypes(%q) = %v, want no error", root, err)
	}
	if len(events) == 0 {
		t.Fatalf("docscheck.EventTypes(%q) returned no event type, so the test reads the wrong tree", root)
	}
	var others []string
	secret := reflect.TypeFor[secrets.Tailnet]()
	for i := range secret.NumField() {
		name, _, _ := strings.Cut(secret.Field(i).Tag.Get("yaml"), ",")
		others = append(others, name)
	}
	// The troubleshoot skill reads credential_state from the answer of GET /api/policy
	// to diagnose a rejected credential.
	others = append(others, "credential_state")
	return newKnownNames(docscheck.ConfigKeys(), events, others)
}

func TestTheSkillSetStatesNoUnknownFlagKeyEventOrLink(t *testing.T) {
	set, err := skills.All()
	if err != nil {
		t.Fatalf("skills.All() = %v, want no error", err)
	}
	root := repositoryRoot(t)
	names := daemonNames(t, root)
	site := os.DirFS(filepath.Join(root, "docs", "site"))
	command := rootCommand()
	for _, skill := range set {
		file := skillFilePath(skill.Name)
		content := string(skill.Content)
		var messages []string
		messages = append(messages, flagDrift(command, file, content)...)
		messages = append(messages, sourceLineDrift(file, content)...)
		messages = append(messages, nameDrift(file, content, names)...)
		messages = append(messages, siteLinkDrift(file, content, site)...)
		for _, message := range messages {
			t.Error(message)
		}
	}
}

func TestFlagDrift(t *testing.T) {
	const file = "skills/demo/SKILL.md"

	t.Run("reports a flag that the command does not declare, with the file and the line", func(t *testing.T) {
		content := "# A skill\n\nRun `hydrascale uninstall --no-such-flag`.\n"
		messages := flagDrift(rootCommand(), file, content)
		if len(messages) != 1 {
			t.Fatalf("flagDrift() returned %d messages, want 1. messages = %q", len(messages), messages)
		}
		if !strings.Contains(messages[0], file+":3") {
			t.Errorf("the message = %q, want it to name %q", messages[0], file+":3")
		}
		if !strings.Contains(messages[0], "--no-such-flag") {
			t.Errorf("the message = %q, want it to name the flag %q", messages[0], "--no-such-flag")
		}
	})

	t.Run("reports a short flag that the command does not declare", func(t *testing.T) {
		messages := flagDrift(rootCommand(), file, "Run `hydrascale status -Z`.\n")
		if len(messages) != 1 {
			t.Fatalf("flagDrift() returned %d messages, want 1. messages = %q", len(messages), messages)
		}
	})

	t.Run("reports a flag of a fenced code block", func(t *testing.T) {
		messages := flagDrift(rootCommand(), file, "```sh\nsudo hydrascale apply --no-such-flag\n```\n")
		if len(messages) != 1 {
			t.Fatalf("flagDrift() returned %d messages, want 1. messages = %q", len(messages), messages)
		}
	})

	for _, content := range []string{
		"Run `hydrascale uninstall --keep-nodes --yes`.\n",
		"Run `hydrascale apply --dry-run` first.\n",
		"Run `sudo hydrascale wrap <service> <id> --apply`.\n",
		"Run `hydrascale status --config /etc/hydrascale/config.yaml`.\n",
		"Run `hydrascale status --config=/etc/hydrascale/config.yaml`.\n",
		"Run `hydrascale apply --help` or `hydrascale apply -h`.\n",
		"```sh\nsudo hydrascale exec personal -- curl -s http://peer:8080\n```\n",
		"```sh\nsudo hydrascale status | grep -c running\n```\n",
		"Run `hydrascale status`, then `grep --count running`.\n",
		"allowed-tools: Read, Bash(hydrascale version:*), Bash(sudo iptables -S:*)\n",
	} {
		t.Run("reports no message for "+strings.TrimSpace(content), func(t *testing.T) {
			if messages := flagDrift(rootCommand(), file, content); len(messages) != 0 {
				t.Errorf("flagDrift() returned %q, want no message", messages)
			}
		})
	}
}

func TestSourceLineDrift(t *testing.T) {
	const file = "skills/demo/SKILL.md"

	t.Run("reports a source line, with the file and the line", func(t *testing.T) {
		content := "# A skill\n\nThe route is in `internal/api/console.go:16`.\n"
		messages := sourceLineDrift(file, content)
		if len(messages) != 1 {
			t.Fatalf("sourceLineDrift() returned %d messages, want 1. messages = %q", len(messages), messages)
		}
		if !strings.Contains(messages[0], file+":3") {
			t.Errorf("the message = %q, want it to name %q", messages[0], file+":3")
		}
		if !strings.Contains(messages[0], "internal/api/console.go:16") {
			t.Errorf("the message = %q, want it to name %q", messages[0], "internal/api/console.go:16")
		}
	})

	t.Run("reports no message for a source file without a line", func(t *testing.T) {
		if messages := sourceLineDrift(file, "The route is in `internal/api/console.go`.\n"); len(messages) != 0 {
			t.Errorf("sourceLineDrift() returned %q, want no message", messages)
		}
	})
}

// testNames returns a small set of names, so that a case of the name check reads alone.
func testNames() knownNames {
	return newKnownNames(
		[]string{"access", "access.mode", "host_dns", "host_dns.mode", "ipv6", "route_table", "tailnets", "tailnets[].alias"},
		[]string{"access.jump_displaced", "ipv6.state"},
		[]string{"tailscale_oauth_client_secret"},
	)
}

func TestConfigKeyDrift(t *testing.T) {
	const file = "skills/demo/SKILL.md"

	t.Run("reports a key that the configuration does not read, with the file and the line", func(t *testing.T) {
		content := "# A skill\n\nSet `route_tables` to 100.\n"
		messages := nameDrift(file, content, testNames())
		if len(messages) != 1 {
			t.Fatalf("nameDrift() returned %d messages, want 1. messages = %q", len(messages), messages)
		}
		if !strings.Contains(messages[0], file+":3") {
			t.Errorf("the message = %q, want it to name %q", messages[0], file+":3")
		}
		if !strings.Contains(messages[0], "route_tables") {
			t.Errorf("the message = %q, want it to name %q", messages[0], "route_tables")
		}
	})

	t.Run("reports a child key that the configuration does not read", func(t *testing.T) {
		if messages := nameDrift(file, "Set `host_dns.style`.\n", testNames()); len(messages) != 1 {
			t.Errorf("nameDrift() returned %q, want 1 message", messages)
		}
	})

	t.Run("reports the key before the colon of a key and a value", func(t *testing.T) {
		if messages := nameDrift(file, "Set `access.style: observe`.\n", testNames()); len(messages) != 1 {
			t.Errorf("nameDrift() returned %q, want 1 message", messages)
		}
	})

	t.Run("reports a short name of a kernel parameter", func(t *testing.T) {
		if messages := nameDrift(file, "The kernel holds no `force_forwarding`.\n", testNames()); len(messages) != 1 {
			t.Errorf("nameDrift() returned %q, want 1 message", messages)
		}
	})

	for _, content := range []string{
		"Set `route_table` and `host_dns.mode`.\n",
		"Set `access.mode: observe`.\n",
		"Set `tailnets[].alias`.\n",
		"Set `tailscale_oauth_client_secret` in the secrets file.\n",
		"Edit `/etc/resolv.conf` or `resolv.conf`.\n",
		"Read `sysctl -n net.ipv6.conf.all.force_forwarding`.\n",
		"Read `net.ipv6.conf.all.force_forwarding`.\n",
		"The policy holds `tagOwners`.\n",
		"Set `ipv6: true`.\n",
		"Open `<peer>.<alias>.ts.internal`.\n",
		"```yaml\nroute_tables: 100\n```\n",
	} {
		t.Run("reports no message for "+strings.TrimSpace(content), func(t *testing.T) {
			if messages := nameDrift(file, content, testNames()); len(messages) != 0 {
				t.Errorf("nameDrift() returned %q, want no message", messages)
			}
		})
	}
}

func TestEventTypeDrift(t *testing.T) {
	const file = "skills/demo/SKILL.md"

	t.Run("reports an event type that the daemon does not record", func(t *testing.T) {
		messages := nameDrift(file, "Read the event `access.jump_moved`.\n", testNames())
		if len(messages) != 1 {
			t.Fatalf("nameDrift() returned %d messages, want 1. messages = %q", len(messages), messages)
		}
		if !strings.Contains(messages[0], "access.jump_moved") {
			t.Errorf("the message = %q, want it to name %q", messages[0], "access.jump_moved")
		}
	})

	t.Run("reports no message for an event type that the daemon records", func(t *testing.T) {
		content := "Read `access.jump_displaced` and `ipv6.state`.\n"
		if messages := nameDrift(file, content, testNames()); len(messages) != 0 {
			t.Errorf("nameDrift() returned %q, want no message", messages)
		}
	})
}

func TestSiteLinkDrift(t *testing.T) {
	const file = "skills/demo/SKILL.md"
	site := fstest.MapFS{
		"index.md":                  {},
		"reference/command-line.md": {},
		"concepts/index.md":         {},
	}

	t.Run("reports a link to a page that the site does not hold, with the file and the line", func(t *testing.T) {
		content := "# A skill\n\nRead https://crank-git.github.io/Hydrascale/reference/no-such-page/.\n"
		messages := siteLinkDrift(file, content, site)
		if len(messages) != 1 {
			t.Fatalf("siteLinkDrift() returned %d messages, want 1. messages = %q", len(messages), messages)
		}
		if !strings.Contains(messages[0], file+":3") {
			t.Errorf("the message = %q, want it to name %q", messages[0], file+":3")
		}
		if !strings.Contains(messages[0], "reference/no-such-page") {
			t.Errorf("the message = %q, want it to name %q", messages[0], "reference/no-such-page")
		}
	})

	t.Run("reports a relative path of the site", func(t *testing.T) {
		messages := siteLinkDrift(file, "Read `docs/site/reference/command-line.md`.\n", site)
		if len(messages) != 1 {
			t.Fatalf("siteLinkDrift() returned %d messages, want 1. messages = %q", len(messages), messages)
		}
	})

	for _, content := range []string{
		"Read https://crank-git.github.io/Hydrascale/reference/command-line/.\n",
		"Read https://crank-git.github.io/Hydrascale/reference/command-line/#exec, then act.\n",
		"Read https://crank-git.github.io/Hydrascale/concepts/ and https://crank-git.github.io/Hydrascale/.\n",
	} {
		t.Run("reports no message for "+strings.TrimSpace(content), func(t *testing.T) {
			if messages := siteLinkDrift(file, content, site); len(messages) != 0 {
				t.Errorf("siteLinkDrift() returned %q, want no message", messages)
			}
		})
	}
}
