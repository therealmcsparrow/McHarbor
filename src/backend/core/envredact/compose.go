// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package envredact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// ErrComposeMaskedValue is returned when a compose document still carries the Marker in a
// place its stored value cannot be put back into, so saving it would persist the bullets.
var ErrComposeMaskedValue = errors.New("compose document carries a masked value that cannot be restored")

// ErrComposeMaskedMoved is returned when a compose document keeps a value nobody may reveal
// while changing what runs with it, or reaches a restored value through a YAML alias outside
// environment: either way the editor would move a value they cannot read.
var ErrComposeMaskedMoved = errors.New("compose document moves a withheld value")

// McHarborService reports whether a compose service is a McHarbor application container,
// from its service name and image; it lets the caller supply core/docker's rule.
type McHarborService func(service, image string) bool

// slotKind says how a compose value is judged and masked.
type slotKind int

const (
	// slotNamed is a named value — environment, build.args, labels — withheld whole.
	slotNamed slotKind = iota
	// slotInline is a command-line string — command, entrypoint, healthcheck.test — whose
	// credentials are masked inside it, leaving the rest readable.
	slotInline
	// slotWhole is a top-level configs.*.content body, withheld whole when it carries one.
	slotWhole
)

// composeEntry is one value McHarbor may mask: where it sits (document, service, field and
// name), its node, and whether it is a list item (`KEY=value`) or a mapping value.
type composeEntry struct {
	doc     int
	service string
	field   string
	name    string
	node    *yaml.Node
	list    bool
	kind    slotKind
}

// key identifies an entry across two versions of the same document.
func (e composeEntry) key() string {
	return strconv.Itoa(e.doc) + "\x00" + e.service + "\x00" + e.field + "\x00" + e.name
}

// value is the entry's value without a list item's `KEY=` prefix.
func (e composeEntry) value() string {
	if e.list {
		_, v, _ := strings.Cut(e.node.Value, "=")
		return v
	}
	return e.node.Value
}

// label names the entry in an audit trail's "still withheld" list.
func (e composeEntry) label() string {
	if e.kind == slotNamed {
		return e.name
	}
	if e.service == "" {
		return e.field + "." + e.name
	}
	return e.service + "." + e.field
}

// policyFor is p with McHarbor set when the entry's service is a McHarbor container.
func (p Policy) policyFor(e composeEntry, docs []*yaml.Node, isMcHarbor McHarborService) Policy {
	if isMcHarbor != nil && e.service != "" && isMcHarbor(e.service, serviceImage(docs[e.doc], e.service)) {
		p.McHarbor = true
	}
	return p
}

// Compose returns text with every withheld value replaced by the Marker, and the names it
// withheld: environment, build.args and labels values, credentials inside command,
// entrypoint and healthcheck.test, and configs content, in every document of the file.
// Formatting and comments are kept; only when a value cannot be edited in place (a block
// scalar) is the document re-encoded.
func (p Policy) Compose(text string, isMcHarbor McHarborService) (string, []string) {
	docs, err := parseDocs(text)
	if err != nil {
		return p.composeLines(text)
	}
	var edits []textEdit
	var withheld []string
	unsafe := false
	for _, e := range composeEntries(docs) {
		policy := p.policyFor(e, docs, isMcHarbor)
		value := e.value()
		masked := ""
		switch e.kind {
		case slotInline:
			masked = policy.MaskInline(value)
			if masked == value {
				continue
			}
		default:
			if isReference(value) || !policy.Withhold(e.name, value) {
				continue
			}
			masked = Marker
		}
		withheld = append(withheld, e.label())
		replacement := masked
		if e.list {
			replacement = e.name + "=" + masked
		}
		edit, ok := scalarEdit(text, e.node, replacement)
		if !ok {
			unsafe = true
		}
		edits = append(edits, edit)
		e.node.Value, e.node.Style = replacement, 0
	}
	if len(edits) == 0 {
		return text, nil
	}
	if unsafe {
		out, err := encodeDocs(docs)
		if err != nil {
			masked, names := p.composeLines(text)
			return masked, names
		}
		return out, withheld
	}
	return applyEdits(text, edits), withheld
}

// RestoreCompose is Policy{}.RestoreCompose with no McHarbor detection: it puts stored values
// back wherever incoming still carries the Marker, and refuses what it cannot place.
func RestoreCompose(incoming, stored string) (string, error) {
	return Policy{}.RestoreCompose(incoming, stored, nil)
}

// RestoreCompose puts the stored value back wherever incoming still carries the Marker,
// reading it from stored (the file on disk). It refuses, with ErrComposeMaskedValue, a marker
// it cannot place: a key with no stored value, an edited command around a masked credential,
// a marker anywhere else, or a document it cannot parse. It refuses, with
// ErrComposeMaskedMoved, keeping a value nobody may reveal while that service's image,
// build, command, entrypoint, labels or healthcheck change, and any restored value that a
// YAML alias outside environment can reach.
func (p Policy) RestoreCompose(incoming, stored string, isMcHarbor McHarborService) (string, error) {
	if !strings.Contains(incoming, Marker) {
		return incoming, nil
	}
	inDocs, err := parseDocs(incoming)
	if err != nil {
		return "", fmt.Errorf("%w: the document does not parse", ErrComposeMaskedValue)
	}
	storedValues := map[string]storedValue{}
	stDocs, stErr := parseDocs(stored)
	if stErr == nil {
		for _, e := range composeEntries(stDocs) {
			v := storedValue{value: e.value(), list: e.list}
			if tok, ok := scalarToken(stored, e.node); ok {
				v.token = tok
			}
			storedValues[e.key()] = v
		}
	}
	var pending []textEditFor
	var restored []composeEntry
	moved := map[serviceRef]bool{}
	for _, e := range composeEntries(inDocs) {
		value := e.value()
		if !strings.Contains(value, Marker) {
			continue
		}
		prev, ok := storedValues[e.key()]
		policy := p.policyFor(e, inDocs, isMcHarbor)
		switch e.kind {
		case slotInline:
			if !ok || policy.MaskInline(prev.value) != value {
				return "", fmt.Errorf("%w: %s changed around a masked value", ErrComposeMaskedValue, e.label())
			}
		default:
			if value != Marker {
				continue
			}
			if !ok {
				return "", fmt.Errorf("%w: %s in %s has no stored value", ErrComposeMaskedValue, e.name, e.service)
			}
		}
		replacement := prev.token
		if replacement == "" || (e.kind == slotNamed && prev.list != e.list) {
			plain := prev.value
			if e.list {
				plain = e.name + "=" + prev.value
			}
			quoted, err := json.Marshal(plain)
			if err != nil {
				return "", fmt.Errorf("%w: %s", ErrComposeMaskedValue, e.label())
			}
			replacement = string(quoted)
		}
		pending = append(pending, textEditFor{entry: e, token: replacement})
		restored = append(restored, e)
		if e.service != "" && policy.neverRevealed(e.name, prev.value) {
			moved[serviceRef{doc: e.doc, service: e.service}] = true
		}
	}
	if aliasReachesRestored(inDocs, restored) {
		return "", fmt.Errorf("%w: a restored value is reachable through a YAML alias outside environment", ErrComposeMaskedMoved)
	}
	for ref := range moved {
		if stErr != nil || serviceExecChanged(inDocs, stDocs, ref.doc, ref.service) {
			return "", fmt.Errorf("%w: service %s keeps a withheld value while what it runs changes", ErrComposeMaskedMoved, ref.service)
		}
	}
	edits := make([]textEdit, 0, len(pending))
	for _, pe := range pending {
		edit, ok := scalarEditRaw(incoming, pe.entry.node, pe.token)
		if !ok {
			return "", fmt.Errorf("%w: %s is not a single-line value", ErrComposeMaskedValue, pe.entry.label())
		}
		edits = append(edits, edit)
	}
	out := applyEdits(incoming, edits)
	if strings.Contains(out, Marker) {
		return "", fmt.Errorf("%w: the marker appears outside a restorable value", ErrComposeMaskedValue)
	}
	if _, err := parseDocs(out); err != nil {
		return "", fmt.Errorf("%w: restoring a value broke the document", ErrComposeMaskedValue)
	}
	return out, nil
}

// serviceRef names one service in one document of a compose stream.
type serviceRef struct {
	doc     int
	service string
}

// textEditFor is a restore decided for one entry, applied once every refusal has been checked.
type textEditFor struct {
	entry composeEntry
	token string
}

// storedValue is one value as it sits in the file on disk.
type storedValue struct {
	value string
	token string
	list  bool
}

// referencePattern matches a value that is only an interpolation, such as ${DB_PASSWORD} or
// ${DB_PASSWORD:?required}: it names a variable held elsewhere and carries no credential. A
// default (${X:-literal}) carries its literal, so it is not a reference.
var referencePattern = regexp.MustCompile(`^(\$\{[A-Za-z_][A-Za-z0-9_]*(:?\?[^${}]*)?\}|\$[A-Za-z_][A-Za-z0-9_]*)$`)

func isReference(value string) bool {
	return referencePattern.MatchString(strings.TrimSpace(value))
}

// parseDocs decodes every document of a multi-document stream; a later document is masked
// and restored like the first.
func parseDocs(text string) ([]*yaml.Node, error) {
	dec := yaml.NewDecoder(strings.NewReader(text))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, &doc)
	}
}

// encodeDocs re-encodes every document, for a value that could not be edited in place.
func encodeDocs(docs []*yaml.Node) (string, error) {
	var buf bytes.Buffer
	for i, doc := range docs {
		if i > 0 {
			buf.WriteString("---\n")
		}
		out, err := yaml.Marshal(doc)
		if err != nil {
			return "", err
		}
		buf.Write(out)
	}
	return buf.String(), nil
}

// composeEntries lists every maskable value in every document, following anchors and merge
// keys; a node shared through an anchor is listed once.
func composeEntries(docs []*yaml.Node) []composeEntry {
	seen := map[*yaml.Node]bool{}
	var out []composeEntry
	for d, doc := range docs {
		root := resolve(doc)
		if root == nil || root.Kind != yaml.MappingNode {
			continue
		}
		if services := resolve(mappingValue(root, "services")); services != nil && services.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(services.Content); i += 2 {
				name := services.Content[i].Value
				svc := resolve(services.Content[i+1])
				if svc == nil || svc.Kind != yaml.MappingNode {
					continue
				}
				at := composeEntry{doc: d, service: name}
				out = appendNamed(out, seen, at, "environment", resolve(mappingValue(svc, "environment")))
				out = appendNamed(out, seen, at, "labels", resolve(mappingValue(svc, "labels")))
				if build := resolve(mappingValue(svc, "build")); build != nil && build.Kind == yaml.MappingNode {
					out = appendNamed(out, seen, at, "build.args", resolve(mappingValue(build, "args")))
				}
				out = appendInline(out, seen, at, "command", resolve(mappingValue(svc, "command")))
				out = appendInline(out, seen, at, "entrypoint", resolve(mappingValue(svc, "entrypoint")))
				if hc := resolve(mappingValue(svc, "healthcheck")); hc != nil && hc.Kind == yaml.MappingNode {
					out = appendInline(out, seen, at, "healthcheck.test", resolve(mappingValue(hc, "test")))
				}
			}
		}
		if configs := resolve(mappingValue(root, "configs")); configs != nil && configs.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(configs.Content); i += 2 {
				cfg := resolve(configs.Content[i+1])
				if cfg == nil || cfg.Kind != yaml.MappingNode {
					continue
				}
				content := resolve(mappingValue(cfg, "content"))
				if content == nil || content.Kind != yaml.ScalarNode || seen[content] {
					continue
				}
				seen[content] = true
				out = append(out, composeEntry{doc: d, field: "configs", name: configs.Content[i].Value, node: content, kind: slotWhole})
			}
		}
	}
	return out
}

// appendNamed lists a mapping (`KEY: value`) or list (`- KEY=value`) of named values.
func appendNamed(out []composeEntry, seen map[*yaml.Node]bool, at composeEntry, field string, n *yaml.Node) []composeEntry {
	if n == nil {
		return out
	}
	at.field, at.kind = field, slotNamed
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, value := n.Content[i], n.Content[i+1]
			if key.Value == "<<" {
				for _, merged := range mergeSources(value) {
					out = appendNamed(out, seen, at, field, merged)
				}
				continue
			}
			value = resolve(value)
			if value == nil || value.Kind != yaml.ScalarNode || value.Tag == "!!null" || seen[value] {
				continue
			}
			seen[value] = true
			e := at
			e.name, e.node = key.Value, value
			out = append(out, e)
		}
	case yaml.SequenceNode:
		for _, item := range n.Content {
			item = resolve(item)
			if item == nil || item.Kind != yaml.ScalarNode || seen[item] {
				continue
			}
			name, _, ok := strings.Cut(item.Value, "=")
			if !ok {
				continue
			}
			seen[item] = true
			e := at
			e.name, e.node, e.list = name, item, true
			out = append(out, e)
		}
	}
	return out
}

// appendInline lists a command-line value: one scalar, or each item of a list by position.
func appendInline(out []composeEntry, seen map[*yaml.Node]bool, at composeEntry, field string, n *yaml.Node) []composeEntry {
	if n == nil {
		return out
	}
	at.field, at.kind = field, slotInline
	add := func(name string, node *yaml.Node) {
		if node == nil || node.Kind != yaml.ScalarNode || node.Tag == "!!null" || seen[node] {
			return
		}
		seen[node] = true
		e := at
		e.name, e.node = name, node
		out = append(out, e)
	}
	switch n.Kind {
	case yaml.ScalarNode:
		add("s", n)
	case yaml.SequenceNode:
		for i, item := range n.Content {
			add(strconv.Itoa(i), resolve(item))
		}
	}
	return out
}

// aliasReachesRestored reports whether an alias can carry a restored value somewhere else:
// any alias to a restored command, label, build arg or config, or an alias outside
// environment to a restored environment value. The alias's own position decides, so
// `environment: *shared` and `<<: *shared` inside environment stay allowed.
func aliasReachesRestored(docs []*yaml.Node, restored []composeEntry) bool {
	if len(restored) == 0 {
		return false
	}
	targets := map[*yaml.Node]composeEntry{}
	for _, e := range restored {
		targets[e.node] = e
	}
	leak := false
	var walk func(n *yaml.Node, inEnv bool)
	walk = func(n *yaml.Node, inEnv bool) {
		if n == nil || leak {
			return
		}
		switch n.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for _, c := range n.Content {
				walk(c, inEnv)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				walk(n.Content[i], inEnv)
				walk(n.Content[i+1], inEnv || n.Content[i].Value == "environment")
			}
		case yaml.AliasNode:
			if subtreeHasTarget(n.Alias, targets, inEnv, map[*yaml.Node]bool{}) {
				leak = true
			}
		}
	}
	for _, doc := range docs {
		walk(doc, false)
	}
	return leak
}

func subtreeHasTarget(n *yaml.Node, targets map[*yaml.Node]composeEntry, inEnv bool, visited map[*yaml.Node]bool) bool {
	if n == nil || visited[n] {
		return false
	}
	visited[n] = true
	if e, ok := targets[n]; ok && (!inEnv || e.kind != slotNamed || e.field != "environment") {
		return true
	}
	if n.Kind == yaml.AliasNode {
		return subtreeHasTarget(n.Alias, targets, inEnv, visited)
	}
	for _, c := range n.Content {
		if subtreeHasTarget(c, targets, inEnv, visited) {
			return true
		}
	}
	return false
}

// execFields are the service fields that decide what a withheld value is handed to.
var execFields = []string{"image", "build", "command", "entrypoint", "labels", "healthcheck"}

// serviceExecChanged reports whether service in document d runs something different in
// incoming than in stored.
func serviceExecChanged(inDocs, stDocs []*yaml.Node, d int, service string) bool {
	if d >= len(inDocs) || d >= len(stDocs) {
		return true
	}
	in, st := serviceNode(inDocs[d], service), serviceNode(stDocs[d], service)
	if in == nil || st == nil {
		return true
	}
	for _, f := range execFields {
		if canonical(mappingValue(in, f), 0) != canonical(mappingValue(st, f), 0) {
			return true
		}
	}
	return false
}

// canonical renders a node with aliases resolved and styles ignored, for comparison.
func canonical(n *yaml.Node, depth int) string {
	n = resolve(n)
	if n == nil || depth > 32 {
		return ""
	}
	var b strings.Builder
	switch n.Kind {
	case yaml.ScalarNode:
		b.WriteString("s:" + strconv.Quote(n.Value))
	case yaml.SequenceNode:
		b.WriteString("[")
		for _, c := range n.Content {
			b.WriteString(canonical(c, depth+1) + ",")
		}
		b.WriteString("]")
	case yaml.MappingNode:
		b.WriteString("{")
		for i := 0; i+1 < len(n.Content); i += 2 {
			b.WriteString(canonical(n.Content[i], depth+1) + "=" + canonical(n.Content[i+1], depth+1) + ",")
		}
		b.WriteString("}")
	}
	return b.String()
}

func mergeSources(n *yaml.Node) []*yaml.Node {
	n = resolve(n)
	if n == nil {
		return nil
	}
	if n.Kind == yaml.SequenceNode {
		var out []*yaml.Node
		for _, c := range n.Content {
			if r := resolve(c); r != nil {
				out = append(out, r)
			}
		}
		return out
	}
	return []*yaml.Node{n}
}

func resolve(n *yaml.Node) *yaml.Node {
	for n != nil {
		switch n.Kind {
		case yaml.DocumentNode:
			if len(n.Content) == 0 {
				return nil
			}
			n = n.Content[0]
		case yaml.AliasNode:
			n = n.Alias
		default:
			return n
		}
	}
	return nil
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func serviceNode(doc *yaml.Node, service string) *yaml.Node {
	root := resolve(doc)
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	services := resolve(mappingValue(root, "services"))
	if services == nil || services.Kind != yaml.MappingNode {
		return nil
	}
	svc := resolve(mappingValue(services, service))
	if svc == nil || svc.Kind != yaml.MappingNode {
		return nil
	}
	return svc
}

func serviceImage(doc *yaml.Node, service string) string {
	if img := resolve(mappingValue(serviceNode(doc, service), "image")); img != nil {
		return img.Value
	}
	return ""
}

// textEdit replaces text[start:end] with replacement.
type textEdit struct {
	start, end  int
	replacement string
}

func applyEdits(text string, edits []textEdit) string {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := text
	last := len(text) + 1
	for _, e := range edits {
		if e.end > last || e.start < 0 || e.end > len(out) {
			continue
		}
		out = out[:e.start] + e.replacement + out[e.end:]
		last = e.start
	}
	return out
}

// nodeOffset converts a node's 1-based line and character column into a byte offset.
func nodeOffset(text string, n *yaml.Node) (int, bool) {
	if n.Line < 1 || n.Column < 1 {
		return 0, false
	}
	offset := 0
	for line := 1; line < n.Line; line++ {
		i := strings.IndexByte(text[offset:], '\n')
		if i < 0 {
			return 0, false
		}
		offset += i + 1
	}
	for col := 1; col < n.Column; col++ {
		if offset >= len(text) || text[offset] == '\n' {
			return 0, false
		}
		_, size := utf8.DecodeRuneInString(text[offset:])
		offset += size
	}
	return offset, true
}

// scalarToken returns the exact source text of a single-line scalar, quotes included.
func scalarToken(text string, n *yaml.Node) (string, bool) {
	start, end, ok := scalarSpan(text, n)
	if !ok {
		return "", false
	}
	return text[start:end], true
}

func scalarSpan(text string, n *yaml.Node) (int, int, bool) {
	start, ok := nodeOffset(text, n)
	if !ok {
		return 0, 0, false
	}
	if n.Anchor != "" && strings.HasPrefix(text[start:], "&"+n.Anchor) {
		start += len(n.Anchor) + 1
		for start < len(text) && (text[start] == ' ' || text[start] == '\t') {
			start++
		}
	}
	lineEnd := strings.IndexByte(text[start:], '\n')
	if lineEnd < 0 {
		lineEnd = len(text) - start
	}
	line := strings.TrimSuffix(text[start:start+lineEnd], "\r")
	switch n.Style {
	case yaml.DoubleQuotedStyle:
		for i := 1; i < len(line); i++ {
			if line[i] == '\\' {
				i++
				continue
			}
			if line[i] == '"' {
				return start, start + i + 1, true
			}
		}
	case yaml.SingleQuotedStyle:
		for i := 1; i < len(line); i++ {
			if line[i] == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					i++
					continue
				}
				return start, start + i + 1, true
			}
		}
	case 0, yaml.TaggedStyle:
		if !strings.HasPrefix(line, n.Value) {
			return 0, 0, false
		}
		rest := line[len(n.Value):]
		if rest == "" || strings.ContainsRune(" \t,]}#", rune(rest[0])) {
			return start, start + len(n.Value), true
		}
	}
	return 0, 0, false
}

// scalarEdit replaces a node's value in place, keeping its quote style; ok is false when
// the node spans lines or its source cannot be matched.
func scalarEdit(text string, n *yaml.Node, value string) (textEdit, bool) {
	start, end, ok := scalarSpan(text, n)
	if !ok {
		return textEdit{}, false
	}
	switch n.Style {
	case yaml.DoubleQuotedStyle:
		if strings.ContainsAny(value, `"\`) {
			return textEdit{start: start, end: end, replacement: jsonQuote(value)}, true
		}
		return textEdit{start: start, end: end, replacement: `"` + value + `"`}, true
	case yaml.SingleQuotedStyle:
		return textEdit{start: start, end: end, replacement: "'" + strings.ReplaceAll(value, "'", "''") + "'"}, true
	default:
		return textEdit{start: start, end: end, replacement: value}, true
	}
}

// jsonQuote renders v as a JSON string, which is also a valid YAML double-quoted scalar.
func jsonQuote(v string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return strconv.Quote(v)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// scalarEditRaw replaces a node's whole source token with token, which is already valid YAML.
func scalarEditRaw(text string, n *yaml.Node, token string) (textEdit, bool) {
	start, end, ok := scalarSpan(text, n)
	if !ok {
		return textEdit{}, false
	}
	return textEdit{start: start, end: end, replacement: token}, true
}

// composeLine matches `KEY: value`, `KEY=value` and `- KEY=value` lines, for a document
// that does not parse and so cannot be walked.
var composeLine = regexp.MustCompile(`^(\s*-?\s*["']?)([A-Za-z_][A-Za-z0-9_.-]*)(["']?\s*[:=]\s*)(.+?)\s*$`)

// composeLines is the fallback for a document that does not parse: every line that looks
// like an assignment of a withheld value has its value replaced, and credentials anywhere
// else on a line are masked in place.
func (p Policy) composeLines(text string) (string, []string) {
	lines := strings.Split(text, "\n")
	var withheld []string
	for i, line := range lines {
		trimmed := strings.TrimSuffix(line, "\r")
		if m := composeLine.FindStringSubmatch(trimmed); m != nil {
			value := strings.Trim(m[4], `"'`)
			if !isReference(value) && p.Withhold(m[2], value) {
				withheld = append(withheld, m[2])
				lines[i] = m[1] + m[2] + m[3] + Marker + strings.TrimPrefix(line, trimmed)
				continue
			}
		}
		if masked := p.MaskInline(trimmed); masked != trimmed {
			withheld = append(withheld, "line "+strconv.Itoa(i+1))
			lines[i] = masked + strings.TrimPrefix(line, trimmed)
		}
	}
	return strings.Join(lines, "\n"), withheld
}
