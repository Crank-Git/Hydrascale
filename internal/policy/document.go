package policy

import (
	"bytes"
	"encoding/json"
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

// Bytes serializes the document back to huJSON text.
// Bytes returns the exact input bytes when the document received no edit.
// After an edit, Bytes returns the original bytes outside the edited range,
// and valid huJSON inside it. Bytes is a pure function of the document's
// input bytes and the edits applied to it.
func (d *Document) Bytes() []byte {
	return d.root.Pack()
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

// ACLRule is one entry of the acls section.
type ACLRule struct {
	Action     string   `json:"action"`
	Src        []string `json:"src"`
	Proto      string   `json:"proto,omitempty"`
	Dst        []string `json:"dst"`
	SrcPosture []string `json:"srcPosture,omitempty"`
}

// SSHRule is one entry of the ssh section.
type SSHRule struct {
	Action      string   `json:"action"`
	Src         []string `json:"src"`
	Dst         []string `json:"dst"`
	Users       []string `json:"users"`
	CheckPeriod string   `json:"checkPeriod,omitempty"`
	AcceptEnv   []string `json:"acceptEnv,omitempty"`
	SrcPosture  []string `json:"srcPosture,omitempty"`
}

// AutoApprovers is the autoApprovers section.
type AutoApprovers struct {
	Routes   map[string][]string `json:"routes,omitempty"`
	ExitNode []string            `json:"exitNode,omitempty"`
}

// Groups returns every group and its member list.
// Groups returns an empty map when the document holds no groups section.
func (d *Document) Groups() (map[string][]string, error) {
	return readMap[[]string](d, "groups")
}

// ACLs returns every allow rule of the acls section.
// ACLs returns an empty list when the document holds no acls section.
func (d *Document) ACLs() ([]ACLRule, error) {
	return readList[ACLRule](d, "acls")
}

// SSH returns every SSH rule of the ssh section.
// SSH returns an empty list when the document holds no ssh section.
func (d *Document) SSH() ([]SSHRule, error) {
	return readList[SSHRule](d, "ssh")
}

// AutoApprovers returns the routes map and the exit node list.
// AutoApprovers returns a zero-value AutoApprovers when the document holds no
// autoApprovers section.
func (d *Document) AutoApprovers() (AutoApprovers, error) {
	raw, ok, err := sectionRaw(d, "autoApprovers")
	if err != nil {
		return AutoApprovers{}, err
	}
	if !ok {
		return AutoApprovers{}, nil
	}
	var approvers AutoApprovers
	if err := json.Unmarshal(raw, &approvers); err != nil {
		return AutoApprovers{}, fmt.Errorf("policy: section %q: %w", "autoApprovers", err)
	}
	return approvers, nil
}

// RawSections returns the JSON of every top-level key that the document holds, keyed by
// name. A caller that reads a section FR-model-2 does not name a typed accessor for
// (`internal/api` reads `hosts`, `tagOwners`, `ipsets`, `grants`, `nodeAttrs`,
// `postures`, `tests`, and `sshTests` this way) unmarshals the raw value itself.
// RawSections reads a clone of d.root, so it never mutates d.root: Minimize strips
// every comment and trailing comma from the value it runs on, and running it on d.root
// would corrupt the byte-preservation guarantee that a later serialize depends on.
func (d *Document) RawSections() (map[string]json.RawMessage, error) {
	clone := d.root.Clone()
	clone.Minimize()
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(clone.Pack(), &sections); err != nil {
		return nil, fmt.Errorf("policy: reading the document: %w", err)
	}
	return sections, nil
}

// sectionRaw returns the raw JSON of the top-level key name, and whether the
// document holds that key.
func sectionRaw(d *Document, name string) (json.RawMessage, bool, error) {
	sections, err := d.RawSections()
	if err != nil {
		return nil, false, err
	}
	raw, ok := sections[name]
	return raw, ok, nil
}

// readList returns every entry of the list-shaped section name.
// readList returns a nil list when the document holds no section by that
// name, which the caller reads as an empty list. An entry that does not
// match T returns an error naming the section and the entry index.
func readList[T any](d *Document, name string) ([]T, error) {
	raw, ok, err := sectionRaw(d, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	var rawEntries []json.RawMessage
	if err := json.Unmarshal(raw, &rawEntries); err != nil {
		return nil, fmt.Errorf("policy: section %q: %w", name, err)
	}
	entries := make([]T, len(rawEntries))
	for i, rawEntry := range rawEntries {
		if err := json.Unmarshal(rawEntry, &entries[i]); err != nil {
			return nil, fmt.Errorf("policy: section %q: entry %d: %w", name, i, err)
		}
	}
	return entries, nil
}

// readMap returns every entry of the map-shaped section name.
// readMap returns a nil map when the document holds no section by that
// name, which the caller reads as an empty map. An entry that does not
// match V returns an error naming the section and the entry key.
func readMap[V any](d *Document, name string) (map[string]V, error) {
	raw, ok, err := sectionRaw(d, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	var rawEntries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawEntries); err != nil {
		return nil, fmt.Errorf("policy: section %q: %w", name, err)
	}
	entries := make(map[string]V, len(rawEntries))
	for key, rawEntry := range rawEntries {
		var entry V
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			return nil, fmt.Errorf("policy: section %q: entry %q: %w", name, key, err)
		}
		entries[key] = entry
	}
	return entries, nil
}

// lineColumn returns the 1-based line and column of byte offset n in text.
func lineColumn(text []byte, n int) (line, column int) {
	line = 1 + bytes.Count(text[:n], []byte("\n"))
	column = 1 + n - (bytes.LastIndexByte(text[:n], '\n') + len("\n"))
	return line, column
}

// AddEntry appends one entry to the named section.
// AddEntry parses entry as one JSON or huJSON value. The new entry matches
// the indentation of the section's other entries. AddEntry creates the
// section when the document does not hold it yet. AddEntry keeps every
// other byte of the document unchanged, including a comment on an existing
// entry.
func (d *Document) AddEntry(section, entry string) error {
	value, err := hujson.Parse([]byte(entry))
	if err != nil {
		return fmt.Errorf("policy: adding an entry: parsing the entry: %w", err)
	}
	obj, ok := d.root.Value.(*hujson.Object)
	if !ok {
		return fmt.Errorf("policy: adding an entry: the document root is not an object")
	}
	for i := range obj.Members {
		if memberName(obj.Members[i]) != section {
			continue
		}
		arr, ok := obj.Members[i].Value.Value.(*hujson.Array)
		if !ok {
			return fmt.Errorf("policy: adding an entry: section %q is not an array", section)
		}
		indent := arrayEntryIndent(obj.Members[i])
		arr.Elements = append(arr.Elements, newArrayElement(value.Value, indent, newTrailingComma(arr)))
		return nil
	}
	obj.Members = append(obj.Members, newSectionMember(section, value.Value, topLevelIndent(obj)))
	return nil
}

// ReplaceEntry replaces the entry at index in the named section.
// ReplaceEntry parses entry as one JSON or huJSON value and keeps the
// indentation and the comment that surround the replaced entry. It returns
// an error when the document holds no section named section, when the
// section is not an array, or when index is out of range.
func (d *Document) ReplaceEntry(section string, index int, entry string) error {
	arr, err := d.arraySection(section)
	if err != nil {
		return fmt.Errorf("policy: replacing an entry: %w", err)
	}
	if index < 0 || index >= len(arr.Elements) {
		return fmt.Errorf("policy: replacing an entry: index %d is out of range for section %q, which holds %d entries", index, section, len(arr.Elements))
	}
	value, err := hujson.Parse([]byte(entry))
	if err != nil {
		return fmt.Errorf("policy: replacing an entry: parsing the entry: %w", err)
	}
	arr.Elements[index].Value = value.Value
	return nil
}

// RemoveEntry removes the entry at index from the named section.
// RemoveEntry keeps the section's key when the removal empties the section.
// It returns an error when the document holds no section named section,
// when the section is not an array, or when index is out of range.
func (d *Document) RemoveEntry(section string, index int) error {
	arr, err := d.arraySection(section)
	if err != nil {
		return fmt.Errorf("policy: removing an entry: %w", err)
	}
	if index < 0 || index >= len(arr.Elements) {
		return fmt.Errorf("policy: removing an entry: index %d is out of range for section %q, which holds %d entries", index, section, len(arr.Elements))
	}
	arr.Elements = append(arr.Elements[:index], arr.Elements[index+1:]...)
	return nil
}

// AddMapEntry adds one key to the map-shaped section name.
// AddMapEntry parses entry as one JSON or huJSON value. The new entry
// matches the indentation of the section's other entries. AddMapEntry
// creates the section when the document does not hold it yet, and it
// returns an error when the section already holds key.
func (d *Document) AddMapEntry(section, key, entry string) error {
	value, err := hujson.Parse([]byte(entry))
	if err != nil {
		return fmt.Errorf("policy: adding an entry: parsing the entry: %w", err)
	}
	obj, ok := d.root.Value.(*hujson.Object)
	if !ok {
		return fmt.Errorf("policy: adding an entry: the document root is not an object")
	}
	for i := range obj.Members {
		if memberName(obj.Members[i]) != section {
			continue
		}
		inner, ok := obj.Members[i].Value.Value.(*hujson.Object)
		if !ok {
			return fmt.Errorf("policy: adding an entry: section %q is not an object", section)
		}
		if mapMemberIndex(inner, key) >= 0 {
			return fmt.Errorf("policy: adding an entry: section %q already holds key %q", section, key)
		}
		indent := mapEntryIndent(obj.Members[i])
		inner.Members = append(inner.Members, newMapMember(key, value.Value, indent, newMapTrailingComma(inner)))
		return nil
	}
	obj.Members = append(obj.Members, newMapSectionMember(section, key, value.Value, topLevelIndent(obj)))
	return nil
}

// ReplaceMapEntry replaces the value at key in the map-shaped section name.
// ReplaceMapEntry parses entry as one JSON or huJSON value and keeps the
// key, the indentation, and the comment that surround the entry. It returns
// an error when the document holds no section named section, when the
// section is not an object, or when the section holds no such key.
func (d *Document) ReplaceMapEntry(section, key, entry string) error {
	inner, err := d.mapSection(section)
	if err != nil {
		return fmt.Errorf("policy: replacing an entry: %w", err)
	}
	index := mapMemberIndex(inner, key)
	if index < 0 {
		return fmt.Errorf("policy: replacing an entry: section %q holds no key %q", section, key)
	}
	value, err := hujson.Parse([]byte(entry))
	if err != nil {
		return fmt.Errorf("policy: replacing an entry: parsing the entry: %w", err)
	}
	inner.Members[index].Value.Value = value.Value
	return nil
}

// RemoveMapEntry removes the entry at key from the map-shaped section name.
// RemoveMapEntry keeps the section's key when the removal empties the
// section. It returns an error when the document holds no section named
// section, when the section is not an object, or when the section holds no
// such key.
func (d *Document) RemoveMapEntry(section, key string) error {
	inner, err := d.mapSection(section)
	if err != nil {
		return fmt.Errorf("policy: removing an entry: %w", err)
	}
	index := mapMemberIndex(inner, key)
	if index < 0 {
		return fmt.Errorf("policy: removing an entry: section %q holds no key %q", section, key)
	}
	inner.Members = append(inner.Members[:index], inner.Members[index+1:]...)
	return nil
}

// RenameMapEntry changes the key of one entry of the map-shaped section
// name, and keeps its value unchanged. It returns an error when the
// document holds no section named section, when the section is not an
// object, when the section holds no oldKey, or when the section already
// holds newKey.
func (d *Document) RenameMapEntry(section, oldKey, newKey string) error {
	inner, err := d.mapSection(section)
	if err != nil {
		return fmt.Errorf("policy: renaming an entry: %w", err)
	}
	index := mapMemberIndex(inner, oldKey)
	if index < 0 {
		return fmt.Errorf("policy: renaming an entry: section %q holds no key %q", section, oldKey)
	}
	if mapMemberIndex(inner, newKey) >= 0 {
		return fmt.Errorf("policy: renaming an entry: section %q already holds key %q", section, newKey)
	}
	inner.Members[index].Name.Value = hujson.String(newKey)
	return nil
}

// AddAutoApproverRoute adds one route to autoApprovers.routes, keyed by cidr, per
// FR-vadv-6 and FR-vadv-7. AddAutoApproverRoute creates the autoApprovers section, the
// routes section, or both, when the document holds neither yet. It returns an error
// when autoApprovers.routes already holds cidr.
func (d *Document) AddAutoApproverRoute(cidr string, approvers []string) error {
	routes, indent, err := d.autoApproverRoutesObject(true)
	if err != nil {
		return fmt.Errorf("policy: adding an auto-approver route: %w", err)
	}
	if mapMemberIndex(routes, cidr) >= 0 {
		return fmt.Errorf("policy: adding an auto-approver route: autoApprovers.routes already holds key %q", cidr)
	}
	value, err := autoApproverListValue(approvers)
	if err != nil {
		return fmt.Errorf("policy: adding an auto-approver route: %w", err)
	}
	routes.Members = append(routes.Members, newMapMember(cidr, value, indent, newMapTrailingComma(routes)))
	return nil
}

// ReplaceAutoApproverRoute replaces the approver list at cidr in autoApprovers.routes,
// keeping the key, the indentation, and the comment that surround the entry. It returns
// an error when the document holds no autoApprovers.routes section, or when the section
// holds no such cidr.
func (d *Document) ReplaceAutoApproverRoute(cidr string, approvers []string) error {
	routes, _, err := d.autoApproverRoutesObject(false)
	if err != nil {
		return fmt.Errorf("policy: replacing an auto-approver route: %w", err)
	}
	index := mapMemberIndex(routes, cidr)
	if index < 0 {
		return fmt.Errorf("policy: replacing an auto-approver route: autoApprovers.routes holds no key %q", cidr)
	}
	value, err := autoApproverListValue(approvers)
	if err != nil {
		return fmt.Errorf("policy: replacing an auto-approver route: %w", err)
	}
	routes.Members[index].Value.Value = value
	return nil
}

// RemoveAutoApproverRoute removes the route at cidr from autoApprovers.routes.
// RemoveAutoApproverRoute keeps the routes key when the removal empties the section. It
// returns an error when the document holds no autoApprovers.routes section, or when the
// section holds no such cidr.
func (d *Document) RemoveAutoApproverRoute(cidr string) error {
	routes, _, err := d.autoApproverRoutesObject(false)
	if err != nil {
		return fmt.Errorf("policy: removing an auto-approver route: %w", err)
	}
	index := mapMemberIndex(routes, cidr)
	if index < 0 {
		return fmt.Errorf("policy: removing an auto-approver route: autoApprovers.routes holds no key %q", cidr)
	}
	routes.Members = append(routes.Members[:index], routes.Members[index+1:]...)
	return nil
}

// SetAutoApproverExitNode replaces the whole approver list of autoApprovers.exitNode,
// per FR-vadv-6 and FR-vadv-7. The exit node is a single field, not a keyed collection,
// so SetAutoApproverExitNode always replaces its whole value; an empty approvers list
// clears it. SetAutoApproverExitNode creates the autoApprovers section when the document
// holds none yet.
func (d *Document) SetAutoApproverExitNode(approvers []string) error {
	approversObj, indent, err := d.autoApproversObject(true)
	if err != nil {
		return fmt.Errorf("policy: setting the auto-approver exit node: %w", err)
	}
	value, err := autoApproverListValue(approvers)
	if err != nil {
		return fmt.Errorf("policy: setting the auto-approver exit node: %w", err)
	}
	for i := range approversObj.Members {
		if memberName(approversObj.Members[i]) == "exitNode" {
			approversObj.Members[i].Value.Value = value
			return nil
		}
	}
	approversObj.Members = append(approversObj.Members, newMapMember("exitNode", value, indent, newMapTrailingComma(approversObj)))
	return nil
}

// autoApproverListValue parses approvers as the huJSON value that an autoApprovers
// route or the exit node holds.
func autoApproverListValue(approvers []string) (hujson.ValueTrimmed, error) {
	raw, err := json.Marshal(approvers)
	if err != nil {
		return nil, fmt.Errorf("marshaling the approver list: %w", err)
	}
	value, err := hujson.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing the approver list: %w", err)
	}
	return value.Value, nil
}

// autoApproversObject returns the object behind the top-level autoApprovers section,
// and the indentation that a new entry of that object matches. It creates the section,
// holding an empty object, when create is true and the document holds no autoApprovers
// section yet, per FR-model-7 extended one level deeper: adding the first route or
// setting the exit node creates the section key.
func (d *Document) autoApproversObject(create bool) (*hujson.Object, []byte, error) {
	root, ok := d.root.Value.(*hujson.Object)
	if !ok {
		return nil, nil, fmt.Errorf("the document root is not an object")
	}
	for i := range root.Members {
		if memberName(root.Members[i]) != "autoApprovers" {
			continue
		}
		inner, ok := root.Members[i].Value.Value.(*hujson.Object)
		if !ok {
			return nil, nil, fmt.Errorf("section %q is not an object", "autoApprovers")
		}
		return inner, mapEntryIndent(root.Members[i]), nil
	}
	if !create {
		return nil, nil, fmt.Errorf("the document holds no section %q", "autoApprovers")
	}
	sectionIndent := topLevelIndent(root)
	inner := &hujson.Object{AfterExtra: append([]byte("\n"), sectionIndent...)}
	root.Members = append(root.Members, newMapMember("autoApprovers", inner, sectionIndent, true))
	return inner, mapEntryIndent(root.Members[len(root.Members)-1]), nil
}

// autoApproverRoutesObject returns the object behind autoApprovers.routes, and the
// indentation that a new route entry matches. It creates autoApprovers, routes, or
// both, when create is true and the document holds neither yet.
func (d *Document) autoApproverRoutesObject(create bool) (*hujson.Object, []byte, error) {
	approvers, indent, err := d.autoApproversObject(create)
	if err != nil {
		return nil, nil, err
	}
	for i := range approvers.Members {
		if memberName(approvers.Members[i]) != "routes" {
			continue
		}
		routes, ok := approvers.Members[i].Value.Value.(*hujson.Object)
		if !ok {
			return nil, nil, fmt.Errorf("section %q is not an object", "autoApprovers.routes")
		}
		return routes, mapEntryIndent(approvers.Members[i]), nil
	}
	if !create {
		return nil, nil, fmt.Errorf("the document holds no section %q", "autoApprovers.routes")
	}
	routes := &hujson.Object{AfterExtra: append([]byte("\n"), indent...)}
	approvers.Members = append(approvers.Members, newMapMember("routes", routes, indent, newMapTrailingComma(approvers)))
	return routes, mapEntryIndent(approvers.Members[len(approvers.Members)-1]), nil
}

// mapSection returns the object behind the named top-level section.
func (d *Document) mapSection(section string) (*hujson.Object, error) {
	obj, ok := d.root.Value.(*hujson.Object)
	if !ok {
		return nil, fmt.Errorf("the document root is not an object")
	}
	for i := range obj.Members {
		if memberName(obj.Members[i]) != section {
			continue
		}
		inner, ok := obj.Members[i].Value.Value.(*hujson.Object)
		if !ok {
			return nil, fmt.Errorf("section %q is not an object", section)
		}
		return inner, nil
	}
	return nil, fmt.Errorf("the document holds no section %q", section)
}

// mapMemberIndex returns the index of the member of obj named key, and -1
// when obj holds no such member.
func mapMemberIndex(obj *hujson.Object, key string) int {
	for i := range obj.Members {
		if memberName(obj.Members[i]) == key {
			return i
		}
	}
	return -1
}

// arraySection returns the array behind the named top-level section.
func (d *Document) arraySection(section string) (*hujson.Array, error) {
	obj, ok := d.root.Value.(*hujson.Object)
	if !ok {
		return nil, fmt.Errorf("the document root is not an object")
	}
	for i := range obj.Members {
		if memberName(obj.Members[i]) != section {
			continue
		}
		arr, ok := obj.Members[i].Value.Value.(*hujson.Array)
		if !ok {
			return nil, fmt.Errorf("section %q is not an array", section)
		}
		return arr, nil
	}
	return nil, fmt.Errorf("the document holds no section %q", section)
}

// memberName returns the unquoted name of an object member.
func memberName(member hujson.ObjectMember) string {
	return member.Name.Value.(hujson.Literal).String()
}

// newArrayElement returns the array element that AddEntry inserts.
// The element carries indent as its leading whitespace, and it carries a
// trailing comma when comma is true.
func newArrayElement(value hujson.ValueTrimmed, indent []byte, comma bool) hujson.Value {
	element := hujson.Value{
		BeforeExtra: append([]byte("\n"), indent...),
		Value:       value,
	}
	if comma {
		element.AfterExtra = hujson.Extra{}
	}
	return element
}

// newSectionMember returns the top-level member that AddEntry inserts when
// the document does not hold section yet. The member holds one entry.
func newSectionMember(section string, value hujson.ValueTrimmed, indent []byte) hujson.ObjectMember {
	entryIndent := append(append([]byte{}, indent...), []byte("  ")...)
	arr := &hujson.Array{
		Elements:   []hujson.Value{newArrayElement(value, entryIndent, true)},
		AfterExtra: append([]byte("\n"), indent...),
	}
	return hujson.ObjectMember{
		Name: hujson.Value{
			BeforeExtra: append([]byte("\n"), indent...),
			Value:       hujson.String(section),
		},
		Value: hujson.Value{
			BeforeExtra: []byte(" "),
			Value:       arr,
			AfterExtra:  hujson.Extra{},
		},
	}
}

// arrayEntryIndent returns the indentation that a new entry of member's
// array section matches. It copies the indentation of the array's last
// entry, or, when the array holds no entry yet, the section key's own
// indentation plus one level.
func arrayEntryIndent(member hujson.ObjectMember) []byte {
	arr := member.Value.Value.(*hujson.Array)
	if len(arr.Elements) > 0 {
		if indent := trailingIndent(arr.Elements[len(arr.Elements)-1].BeforeExtra); indent != nil {
			return indent
		}
	}
	return append(trailingIndent(member.Name.BeforeExtra), []byte("  ")...)
}

// topLevelIndent returns the indentation that a new top-level section key
// matches. It copies the indentation of the last existing top-level key, or
// two spaces when the document holds no top-level key yet.
func topLevelIndent(obj *hujson.Object) []byte {
	for i := len(obj.Members) - 1; i >= 0; i-- {
		if indent := trailingIndent(obj.Members[i].Name.BeforeExtra); indent != nil {
			return indent
		}
	}
	return []byte("  ")
}

// newTrailingComma reports whether a new last entry of arr keeps a trailing
// comma, matching the array's own convention. An empty array defaults to a
// trailing comma, which matches this project's own document style.
func newTrailingComma(arr *hujson.Array) bool {
	if len(arr.Elements) == 0 {
		return true
	}
	return arr.Elements[len(arr.Elements)-1].AfterExtra != nil
}

// newMapMember returns the object member that AddMapEntry inserts. The
// member's name carries indent as its leading whitespace, and it carries a
// trailing comma when comma is true.
func newMapMember(key string, value hujson.ValueTrimmed, indent []byte, comma bool) hujson.ObjectMember {
	member := hujson.ObjectMember{
		Name: hujson.Value{
			BeforeExtra: append([]byte("\n"), indent...),
			Value:       hujson.String(key),
		},
		Value: hujson.Value{
			BeforeExtra: []byte(" "),
			Value:       value,
		},
	}
	if comma {
		member.Value.AfterExtra = hujson.Extra{}
	}
	return member
}

// newMapSectionMember returns the top-level member that AddMapEntry inserts
// when the document does not hold section yet. The member holds one entry.
func newMapSectionMember(section, key string, value hujson.ValueTrimmed, indent []byte) hujson.ObjectMember {
	entryIndent := append(append([]byte{}, indent...), []byte("  ")...)
	obj := &hujson.Object{
		Members:    []hujson.ObjectMember{newMapMember(key, value, entryIndent, true)},
		AfterExtra: append([]byte("\n"), indent...),
	}
	return hujson.ObjectMember{
		Name: hujson.Value{
			BeforeExtra: append([]byte("\n"), indent...),
			Value:       hujson.String(section),
		},
		Value: hujson.Value{
			BeforeExtra: []byte(" "),
			Value:       obj,
			AfterExtra:  hujson.Extra{},
		},
	}
}

// mapEntryIndent returns the indentation that a new entry of member's
// object section matches. It copies the indentation of the section's last
// entry, or, when the section holds no entry yet, the section key's own
// indentation plus one level.
func mapEntryIndent(member hujson.ObjectMember) []byte {
	inner := member.Value.Value.(*hujson.Object)
	if len(inner.Members) > 0 {
		if indent := trailingIndent(inner.Members[len(inner.Members)-1].Name.BeforeExtra); indent != nil {
			return indent
		}
	}
	return append(trailingIndent(member.Name.BeforeExtra), []byte("  ")...)
}

// newMapTrailingComma reports whether a new last entry of obj keeps a
// trailing comma, matching the object's own convention. An empty object
// defaults to a trailing comma, which matches this project's own document
// style.
func newMapTrailingComma(obj *hujson.Object) bool {
	if len(obj.Members) == 0 {
		return true
	}
	return obj.Members[len(obj.Members)-1].Value.AfterExtra != nil
}

// trailingIndent returns the whitespace after the last newline in extra, or
// nil when extra holds no newline.
func trailingIndent(extra hujson.Extra) []byte {
	i := bytes.LastIndexByte(extra, '\n')
	if i < 0 {
		return nil
	}
	return append([]byte{}, extra[i+1:]...)
}
