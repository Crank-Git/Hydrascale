// Package skills holds the skill set that a coding agent loads, and it embeds that set in
// the binary.
//
// go:embed places every SKILL.md of this directory in the binary, so
// `hydrascale skills install` writes the skills on a host that holds no checkout of this
// repository. See FR-skills-21 of docs/specs/features/10-agent-skills.md.
package skills

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed */SKILL.md
var files embed.FS

// Skill carries one skill of the embedded set.
type Skill struct {
	// Name is the value of the front matter key name, and it equals the directory name.
	Name string
	// Description is the value of the front matter key description. A coding agent reads
	// the description to decide when it loads the skill.
	Description string
	// Content is the whole file, the front matter included.
	Content []byte
}

// All returns every embedded skill, in the order of the name.
//
// All returns an error in each of these cases:
//   - A file opens with no front matter.
//   - The front matter holds no name.
//   - The front matter holds no description.
//   - The name and the directory name differ.
//
// All reads every skill and returns the errors together.
func All() ([]Skill, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("skills: read the embedded set: %w", err)
	}

	set := make([]Skill, 0, len(entries))
	var errs []error
	// fs.ReadDir sorts the entries by name, so the caller needs no second sort.
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := entry.Name() + "/SKILL.md"
		content, readErr := fs.ReadFile(files, path)
		if readErr != nil {
			errs = append(errs, fmt.Errorf("skills: read %s: %w", path, readErr))
			continue
		}
		name, description, matterErr := frontMatter(content)
		if matterErr != nil {
			errs = append(errs, fmt.Errorf("skills: %s: %w", path, matterErr))
			continue
		}
		// The installer joins the name onto a directory path, so a name that differs from
		// the directory name would write outside the skill directory.
		if name != entry.Name() {
			errs = append(errs, fmt.Errorf("skills: %s: the front matter names %q and the directory names %q",
				path, name, entry.Name()))
			continue
		}
		set = append(set, Skill{Name: name, Description: description, Content: content})
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return set, nil
}

// frontMatterDelimiter opens and closes the front matter of a skill file.
const frontMatterDelimiter = "---"

// frontMatter returns the name and the description that the front matter of one skill file
// holds. content is the whole file.
//
// frontMatter returns an error in each of these cases:
//   - The file opens with no delimiter.
//   - The front matter holds no end delimiter.
//   - The front matter holds no name.
//   - The front matter holds no description.
func frontMatter(content []byte) (name, description string, err error) {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != frontMatterDelimiter {
		return "", "", fmt.Errorf("the file opens with no front matter delimiter %s", frontMatterDelimiter)
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == frontMatterDelimiter {
			end = i
			break
		}
	}
	if end < 0 {
		return "", "", fmt.Errorf("the front matter holds no end delimiter %s", frontMatterDelimiter)
	}

	for _, line := range lines[1:end] {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			name = strings.TrimSpace(value)
		case "description":
			description = strings.TrimSpace(value)
		}
	}
	if name == "" {
		return "", "", errors.New("the front matter holds no name")
	}
	if description == "" {
		return "", "", errors.New("the front matter holds no description")
	}
	return name, description, nil
}
