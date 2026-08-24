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

func TestASectionAbsentFromTheDocumentReturnsAnEmptyList(t *testing.T) {
	doc, err := Parse("{}")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	groups, err := doc.Groups()
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("Groups: got %v, want an empty list", groups)
	}
	acls, err := doc.ACLs()
	if err != nil {
		t.Fatalf("ACLs: %v", err)
	}
	if len(acls) != 0 {
		t.Fatalf("ACLs: got %v, want an empty list", acls)
	}
}

func TestRawSectionsReturnsTheJSONOfEveryTopLevelKey(t *testing.T) {
	text := `{
  "groups": {"group:admins": ["alice@example.com"]},
  "hosts": {"server": "100.64.0.1"},
  "randomizeClientPort": true,
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sections, err := doc.RawSections()
	if err != nil {
		t.Fatalf("RawSections: %v", err)
	}
	if _, ok := sections["groups"]; !ok {
		t.Fatalf("RawSections: got %v, want a key named groups", sections)
	}
	if _, ok := sections["hosts"]; !ok {
		t.Fatalf("RawSections: got %v, want a key named hosts", sections)
	}
	if _, ok := sections["randomizeClientPort"]; !ok {
		t.Fatalf("RawSections: got %v, want a key named randomizeClientPort", sections)
	}
	if _, ok := sections["acls"]; ok {
		t.Fatalf("RawSections: got %v, want no key named acls, which the document does not hold", sections)
	}
}

func TestRawSectionsDoesNotMutateTheDocument(t *testing.T) {
	text := `{
  // a comment RawSections must keep
  "groups": {"group:admins": ["alice@example.com"]},
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := doc.RawSections(); err != nil {
		t.Fatalf("RawSections: %v", err)
	}
	if got := string(doc.Bytes()); got != text {
		t.Fatalf("Bytes after RawSections: got %q, want the original text %q", got, text)
	}
}

func TestGroupsReturnsEveryMember(t *testing.T) {
	text := `{
  "groups": {
    "group:admins": ["alice@example.com", "bob@example.com"],
    "group:dev": ["carol@example.com"],
  },
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	groups, err := doc.Groups()
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	want := map[string][]string{
		"group:admins": {"alice@example.com", "bob@example.com"},
		"group:dev":    {"carol@example.com"},
	}
	if len(groups) != len(want) {
		t.Fatalf("Groups: got %v, want %v", groups, want)
	}
	for name, members := range want {
		if strings.Join(groups[name], ",") != strings.Join(members, ",") {
			t.Fatalf("Groups[%q]: got %v, want %v", name, groups[name], members)
		}
	}
}

func TestACLsReturnsSourceDestinationProtocolAndPosture(t *testing.T) {
	text := `{
  "acls": [
    {"action": "accept", "src": ["group:admins"], "proto": "tcp", "dst": ["*:22"], "srcPosture": ["posture:latest"]},
  ],
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	acls, err := doc.ACLs()
	if err != nil {
		t.Fatalf("ACLs: %v", err)
	}
	if len(acls) != 1 {
		t.Fatalf("ACLs: got %d entries, want 1", len(acls))
	}
	got := acls[0]
	if got.Action != "accept" || got.Proto != "tcp" {
		t.Fatalf("ACLs[0]: got action %q proto %q, want accept tcp", got.Action, got.Proto)
	}
	if strings.Join(got.Src, ",") != "group:admins" || strings.Join(got.Dst, ",") != "*:22" {
		t.Fatalf("ACLs[0]: got src %v dst %v, want [group:admins] [*:22]", got.Src, got.Dst)
	}
	if strings.Join(got.SrcPosture, ",") != "posture:latest" {
		t.Fatalf("ACLs[0]: got srcPosture %v, want [posture:latest]", got.SrcPosture)
	}
}

func TestSSHReturnsCheckPeriodAndAcceptEnv(t *testing.T) {
	text := `{
  "ssh": [
    {"action": "accept", "src": ["group:admins"], "dst": ["tag:prod"], "users": ["root"], "checkPeriod": "12h", "acceptEnv": ["LANG"]},
  ],
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rules, err := doc.SSH()
	if err != nil {
		t.Fatalf("SSH: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("SSH: got %d entries, want 1", len(rules))
	}
	got := rules[0]
	if got.CheckPeriod != "12h" {
		t.Fatalf("SSH[0]: got checkPeriod %q, want 12h", got.CheckPeriod)
	}
	if strings.Join(got.AcceptEnv, ",") != "LANG" {
		t.Fatalf("SSH[0]: got acceptEnv %v, want [LANG]", got.AcceptEnv)
	}
	if strings.Join(got.Users, ",") != "root" {
		t.Fatalf("SSH[0]: got users %v, want [root]", got.Users)
	}
}

func TestAutoApproversReturnsRoutesAndExitNode(t *testing.T) {
	text := `{
  "autoApprovers": {
    "routes": {
      "10.0.0.0/24": ["tag:router"],
    },
    "exitNode": ["tag:exit"],
  },
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	approvers, err := doc.AutoApprovers()
	if err != nil {
		t.Fatalf("AutoApprovers: %v", err)
	}
	if strings.Join(approvers.Routes["10.0.0.0/24"], ",") != "tag:router" {
		t.Fatalf("AutoApprovers.Routes: got %v, want tag:router for 10.0.0.0/24", approvers.Routes)
	}
	if strings.Join(approvers.ExitNode, ",") != "tag:exit" {
		t.Fatalf("AutoApprovers.ExitNode: got %v, want [tag:exit]", approvers.ExitNode)
	}
}

func TestAMalformedEntryReturnsAnErrorNamingTheSectionAndIndex(t *testing.T) {
	text := `{
  "acls": [
    {"action": "accept", "src": "group:admins", "dst": ["*:*"]},
  ],
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	_, err = doc.ACLs()
	if err == nil {
		t.Fatal("ACLs: got no error, want an error naming the section and the entry")
	}
	if !strings.Contains(err.Error(), "acls") || !strings.Contains(err.Error(), "entry 0") {
		t.Fatalf("ACLs: error %q does not name section %q and entry 0", err.Error(), "acls")
	}
}

func TestReadingASectionDoesNotMutateTheDocument(t *testing.T) {
	text := `{
  // a comment a read must keep
  "groups": {
    "group:admins": ["alice@example.com"],
  },
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := doc.Groups(); err != nil {
		t.Fatalf("Groups: %v", err)
	}
	if got := string(doc.root.Pack()); got != text {
		t.Fatalf("Pack after Groups: got %q, want the original text %q", got, text)
	}
}

func TestAddingAnEntryKeepsTheCommentOnAnEarlierEntry(t *testing.T) {
	text := `{
  "acls": [
    // admins reach everything
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
  ],
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.AddEntry("acls", `{"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:22"]}`); err != nil {
		t.Fatalf("AddEntry: %v", err)
	}
	want := `{
  "acls": [
    // admins reach everything
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
    {"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:22"]},
  ],
}
`
	if got := string(doc.root.Pack()); got != want {
		t.Fatalf("Pack: got %q, want %q", got, want)
	}
}

func TestChangingAnEntryKeepsEveryOtherEntryByteForByte(t *testing.T) {
	text := `{
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
    {"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:22"]},
    {"action": "accept", "src": ["group:ops"], "dst": ["10.0.1.0/8:22"]},
    {"action": "accept", "src": ["group:sre"], "dst": ["10.0.2.0/8:22"]},
  ],
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.ReplaceEntry("acls", 1, `{"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:2222"]}`); err != nil {
		t.Fatalf("ReplaceEntry: %v", err)
	}
	want := `{
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
    {"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:2222"]},
    {"action": "accept", "src": ["group:ops"], "dst": ["10.0.1.0/8:22"]},
    {"action": "accept", "src": ["group:sre"], "dst": ["10.0.2.0/8:22"]},
  ],
}
`
	if got := string(doc.root.Pack()); got != want {
		t.Fatalf("Pack: got %q, want %q", got, want)
	}
}

func TestRemovingTheLastEntryKeepsAnEmptySectionKey(t *testing.T) {
	text := `{
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
  ],
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.RemoveEntry("acls", 0); err != nil {
		t.Fatalf("RemoveEntry: %v", err)
	}
	want := `{
  "acls": [
  ],
}
`
	if got := string(doc.root.Pack()); got != want {
		t.Fatalf("Pack: got %q, want %q", got, want)
	}
}

func TestAddingTheFirstEntryCreatesTheSectionKey(t *testing.T) {
	text := "{\n}\n"
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.AddEntry("acls", `{"action": "accept", "src": ["group:admins"], "dst": ["*:*"]}`); err != nil {
		t.Fatalf("AddEntry: %v", err)
	}
	want := `{
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
  ],
}
`
	if got := string(doc.root.Pack()); got != want {
		t.Fatalf("Pack: got %q, want %q", got, want)
	}
}

func TestEditingADocumentWithNoCommentsMatchesNeighbourIndentation(t *testing.T) {
	text := `{
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
    {"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:22"]}
  ]
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.AddEntry("acls", `{"action": "accept", "src": ["group:ops"], "dst": ["10.0.1.0/8:22"]}`); err != nil {
		t.Fatalf("AddEntry: %v", err)
	}
	want := `{
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
    {"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:22"]},
    {"action": "accept", "src": ["group:ops"], "dst": ["10.0.1.0/8:22"]}
  ]
}
`
	if got := string(doc.root.Pack()); got != want {
		t.Fatalf("Pack: got %q, want %q", got, want)
	}
}

func TestAddingAnEntryToAMissingSectionReturnsAnError(t *testing.T) {
	doc, err := Parse("{}")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.ReplaceEntry("acls", 0, `{"action": "accept"}`); err == nil {
		t.Fatal("ReplaceEntry: got no error for a document with no acls section")
	}
	if err := doc.RemoveEntry("acls", 0); err == nil {
		t.Fatal("RemoveEntry: got no error for a document with no acls section")
	}
}

func TestReplacingAnEntryOutOfRangeReturnsAnError(t *testing.T) {
	doc, err := Parse(`{"acls": [{"action": "accept"}]}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.ReplaceEntry("acls", 1, `{"action": "accept"}`); err == nil {
		t.Fatal("ReplaceEntry: got no error for an out-of-range index")
	}
}

func TestSerializingAnUneditedDocumentReturnsTheExactInputBytes(t *testing.T) {
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
	if got := string(doc.Bytes()); got != text {
		t.Fatalf("Bytes: got %q, want the original text %q", got, text)
	}
}

func TestSerializingAfterOneEditPreservesEveryOtherByte(t *testing.T) {
	text := `{
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
    {"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:22"]},
    {"action": "accept", "src": ["group:ops"], "dst": ["10.0.1.0/8:22"]},
    {"action": "accept", "src": ["group:sre"], "dst": ["10.0.2.0/8:22"]},
  ],
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.ReplaceEntry("acls", 1, `{"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:2222"]}`); err != nil {
		t.Fatalf("ReplaceEntry: %v", err)
	}
	want := `{
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
    {"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:2222"]},
    {"action": "accept", "src": ["group:ops"], "dst": ["10.0.1.0/8:22"]},
    {"action": "accept", "src": ["group:sre"], "dst": ["10.0.2.0/8:22"]},
  ],
}
`
	if got := string(doc.Bytes()); got != want {
		t.Fatalf("Bytes: got %q, want %q", got, want)
	}
}

func TestParseEditSerializeIsAPureFunction(t *testing.T) {
	text := `{
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
  ],
}
`
	edit := `{"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:22"]}`

	first, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := first.AddEntry("acls", edit); err != nil {
		t.Fatalf("AddEntry: %v", err)
	}

	second, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := second.AddEntry("acls", edit); err != nil {
		t.Fatalf("AddEntry: %v", err)
	}

	if string(first.Bytes()) != string(second.Bytes()) {
		t.Fatalf("Bytes: two parses of the same text with the same edit produced different output: %q vs %q", first.Bytes(), second.Bytes())
	}
}

func TestAnUnmodelledKeyRoundTripsUnchangedThroughAnEditAndSerialize(t *testing.T) {
	text := `{
  "randomizeClientPort": true,
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
  ],
}
`
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.AddEntry("acls", `{"action": "accept", "src": ["group:eng"], "dst": ["10.0.0.0/8:22"]}`); err != nil {
		t.Fatalf("AddEntry: %v", err)
	}
	if got := string(doc.Bytes()); !strings.Contains(got, `"randomizeClientPort": true,`) {
		t.Fatalf("Bytes: got %q, want the unmodelled key randomizeClientPort unchanged", got)
	}
}
