package policy

import (
	"strings"
	"testing"
)

func TestParseReturnsLineAndColumnOnASyntaxError(t *testing.T) {
	text := "{\n  \"groups\": {\n"
	_, err := Parse(text)
	if err == nil {
		t.Fatal("Parse: got no error, want an error naming the line and the column")
	}
	if !strings.Contains(err.Error(), "line") || !strings.Contains(err.Error(), "column") {
		t.Fatalf("Parse: error %q does not name a line and a column", err.Error())
	}
}

func TestParsePreservesEveryByteWhenNoEditFollows(t *testing.T) {
	text := `{
  // a comment the visual editor must keep
  "groups": {
    "group:admins": ["alice@example.com"],
  },
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
  ],
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := string(doc.root.Pack()); got != text {
		t.Fatalf("Pack: got %q, want the original text %q", got, text)
	}
}

func TestParseAcceptsADocumentWithNoTopLevelKeys(t *testing.T) {
	doc, err := Parse("{}")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc == nil {
		t.Fatal("Parse: got a nil document for a document with no top-level keys")
	}
}

func TestParseRejectsADuplicateTopLevelKey(t *testing.T) {
	text := "{\n  \"groups\": {},\n  \"groups\": {}\n}\n"
	_, err := Parse(text)
	if err == nil {
		t.Fatal("Parse: got no error, want an error naming the duplicate key")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("Parse: error %q does not name line 3, the second occurrence", err.Error())
	}
}
