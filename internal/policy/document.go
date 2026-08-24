package policy

import (
	"bytes"
	"fmt"

	"github.com/tailscale/hujson"
)

// Document is a policy document held as a byte-preserving syntax tree.
// Parsing, reading, and serializing a Document changes no byte the caller
// did not edit.
type Document struct {
	root hujson.Value
}

// Parse parses text as a policy document.
// Parse preserves every comment, every trailing comma, and the exact byte
// range of every value. Parse returns an error that states the line and the
// column of a syntax failure, and it builds no document on that failure.
func Parse(text string) (*Document, error) {
	b := []byte(text)
	root, err := hujson.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("policy: parsing document: %w", err)
	}
	if err := rejectDuplicateTopLevelKeys(b, root); err != nil {
		return nil, fmt.Errorf("policy: parsing document: %w", err)
	}
	return &Document{root: root}, nil
}

// rejectDuplicateTopLevelKeys returns an error naming the line and the
// column of the second occurrence of a top-level key that the document
// holds twice. huJSON grammar allows a duplicate key inside one object; the
// policy document format forbids it.
func rejectDuplicateTopLevelKeys(text []byte, root hujson.Value) error {
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	seen := make(map[string]bool, len(obj.Members))
	for _, member := range obj.Members {
		name := member.Name.Value.(hujson.Literal).String()
		if seen[name] {
			line, column := lineColumn(text, member.Name.StartOffset)
			return fmt.Errorf("line %d, column %d: duplicate top-level key %q", line, column, name)
		}
		seen[name] = true
	}
	return nil
}

// lineColumn returns the 1-based line and column of byte offset n in text.
func lineColumn(text []byte, n int) (line, column int) {
	line = 1 + bytes.Count(text[:n], []byte("\n"))
	column = 1 + n - (bytes.LastIndexByte(text[:n], '\n') + len("\n"))
	return line, column
}
