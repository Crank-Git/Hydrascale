package hostaccess

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	hostsBeginMarker = "# BEGIN HYDRASCALE MANAGED BLOCK - DO NOT EDIT"
	hostsEndMarker   = "# END HYDRASCALE MANAGED BLOCK"
)

// buildManagedBlock generates the managed block content from v4 and v6 records.
// Names are sorted alphabetically for deterministic output.
func buildManagedBlock(v4 map[string]string, v6 map[string]string) string {
	// Collect all unique names
	nameSet := make(map[string]struct{})
	for name := range v4 {
		nameSet[name] = struct{}{}
	}
	for name := range v6 {
		nameSet[name] = struct{}{}
	}

	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString(hostsBeginMarker + "\n")
	for _, name := range names {
		if ip, ok := v4[name]; ok {
			fmt.Fprintf(&sb, "%s  %s\n", ip, name)
		}
		if ip, ok := v6[name]; ok {
			fmt.Fprintf(&sb, "%s  %s\n", ip, name)
		}
	}
	sb.WriteString(hostsEndMarker + "\n")
	return sb.String()
}

// UpdateHostsFile reads the existing hosts file at path, replaces the managed
// block with new content derived from v4 and v6, and writes atomically.
// If v4 and v6 are both nil or empty, the managed block is removed.
// Skips the write if block content is unchanged.
// Preserves original file permissions.
func UpdateHostsFile(path string, v4 map[string]string, v6 map[string]string) error {
	// Read existing file, tolerating non-existence
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	// Determine original permissions; default to 0644 if file doesn't exist
	perm := os.FileMode(0644)
	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode().Perm()
	}

	content := string(existing)

	// Build the new managed block (empty when no records)
	var newBlock string
	if len(v4) > 0 || len(v6) > 0 {
		newBlock = buildManagedBlock(v4, v6)
	}

	// Extract existing block and surrounding content
	beginIdx := strings.Index(content, hostsBeginMarker)
	endIdx := strings.Index(content, hostsEndMarker)

	var updated string
	if beginIdx != -1 && endIdx != -1 && endIdx > beginIdx {
		// Block exists — extract it for change detection
		blockEnd := endIdx + len(hostsEndMarker)
		// Include trailing newline if present
		if blockEnd < len(content) && content[blockEnd] == '\n' {
			blockEnd++
		}
		existingBlock := content[beginIdx:blockEnd]

		if newBlock == existingBlock {
			// No change needed
			return nil
		}

		before := content[:beginIdx]
		after := content[blockEnd:]
		if newBlock == "" {
			updated = before + after
		} else {
			updated = before + newBlock + after
		}
	} else {
		// No existing block
		if newBlock == "" {
			// Nothing to do
			return nil
		}
		// Append with a separating newline if needed
		if len(content) > 0 && !strings.HasSuffix(content, "\n") {
			updated = content + "\n" + newBlock
		} else {
			updated = content + newBlock
		}
	}

	// Atomic write via temp file + rename
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".hydrascale-hosts-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.WriteString(updated); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}
