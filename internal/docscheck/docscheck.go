// Package docscheck lists the names that the documentation site and the skills state: each
// configuration key that internal/config reads, and each event type that the daemon
// records. A test of a page compares the page to these lists, so a new key or a new event
// type fails that test until the page states it. The daemon does not import this package.
package docscheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"hydrascale/internal/config"
)

// ConfigKeys returns each key of the configuration file as a dotted path, in sorted order.
// The list holds each parent key, such as `host_dns`, and each child key, such as
// `host_dns.mode`. A key under a list carries the marker [], such as `tailnets[].id`.
// The list holds no field that carries the tag yaml:"-", because the file does not hold
// such a field.
func ConfigKeys() []string {
	var keys []string
	collectKeys(reflect.TypeFor[config.Config](), "", &keys)
	slices.Sort(keys)
	return keys
}

// collectKeys appends the key of each field of the struct type t to keys, with prefix
// before each key.
func collectKeys(t reflect.Type, prefix string, keys *[]string) {
	for i := range t.NumField() {
		field := t.Field(i)
		name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}
		key := prefix + name
		*keys = append(*keys, key)

		child := field.Type
		if child.Kind() == reflect.Pointer {
			child = child.Elem()
		}
		switch {
		case child.Kind() == reflect.Struct:
			collectKeys(child, key+".", keys)
		case child.Kind() == reflect.Slice && child.Elem().Kind() == reflect.Struct:
			collectKeys(child.Elem(), key+"[].", keys)
		}
	}
}

// EventTypes returns each event type that the daemon records, in sorted order, without a
// duplicate.
// root is the repository root. EventTypes parses each Go file under root/internal and
// skips each test file. An event type is one of these:
//   - The string literal that is the first argument of a call to a method named emit or
//     RecordEvent. The control API records an event through RecordEvent.
//   - The string literal value of a constant whose name holds "Event".
//
// EventTypes skips a call whose first argument is not a string literal, such as
// emit(eventType, ...), because the caller passes a constant or a literal to that call.
// EventTypes returns an error when the directory does not exist or a file does not parse.
func EventTypes(root string) ([]string, error) {
	seen := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "emit" || sel.Sel.Name == "RecordEvent") && len(node.Args) > 0 {
					if value, ok := stringLiteral(node.Args[0]); ok {
						seen[value] = true
					}
				}
			case *ast.ValueSpec:
				for i, name := range node.Names {
					if !strings.Contains(name.Name, "Event") || i >= len(node.Values) {
						continue
					}
					if value, ok := stringLiteral(node.Values[i]); ok {
						seen[value] = true
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	events := make([]string, 0, len(seen))
	for event := range seen {
		events = append(events, event)
	}
	slices.Sort(events)
	return events, nil
}

// stringLiteral returns the value of expr and true when expr is a string literal.
func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}
