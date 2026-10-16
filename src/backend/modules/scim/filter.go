// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"fmt"
	"strings"
	"unicode"
)

// Filter is a parsed SCIM 2.0 filter expression. SCIM 2.0 §3.4.2.2
// defines the grammar; this parser implements a deliberately
// restricted subset that covers the operators most IdPs emit:
//
//	eq   equality               ("userName eq "alice"")
//	co   contains               ("displayName co "alice"")
//	sw   starts-with            ("userName sw "al"")
//	and, or, not                (logical grouping)
//
// We do NOT support pr (present), gt/ge/lt/le, [brackets], or
// nested paths — the SCIM spec says these are optional and IdPs in
// practice only emit the operators above. Returning a clear error
// on an unsupported operator helps the admin diagnose misconfigured
// connectors.
type Filter struct {
	raw string
}

// Parse converts a raw SCIM filter string into a Filter. The empty
// string is a valid filter (matches everything). Parse errors are
// returned with a human-readable position so the caller can echo
// them back through ErrSCIMFilterInvalid.
func Parse(input string) (*Filter, error) {
	p := &parser{src: input}
	// We deliberately discard the boolean result: the parser still
	// walks the full input so syntax errors surface here. The
	// evaluator (MatchUser / matchFilter) re-walks the raw text
	// against the resource at filter time.
	if _, err := p.parseOr(); err != nil {
		return nil, err
	}
	if p.pos < len(input) {
		return nil, fmt.Errorf("unexpected trailing input at %d", p.pos)
	}
	return &Filter{raw: input}, nil
}

// String returns the original raw filter text.
func (f *Filter) String() string { return f.raw }

// MatchUser evaluates the parsed filter against a User. Returns
// true when the filter is nil, empty, or all branches match.
func (f *Filter) MatchUser(u *User) bool {
	if f == nil || strings.TrimSpace(f.raw) == "" {
		return true
	}
	return matchFilter(strings.TrimSpace(f.raw), userAttr(u))
}

// --- Parser ---

type parser struct {
	src string
	pos int
}

func (p *parser) parseOr() (bool, error) {
	left, err := p.parseAnd()
	if err != nil {
		return false, err
	}
	for p.peekWord("or") {
		p.consumeWord("or")
		right, err := p.parseAnd()
		if err != nil {
			return false, err
		}
		left = left || right
	}
	return left, nil
}

func (p *parser) parseAnd() (bool, error) {
	left, err := p.parseNot()
	if err != nil {
		return false, err
	}
	for p.peekWord("and") {
		p.consumeWord("and")
		right, err := p.parseNot()
		if err != nil {
			return false, err
		}
		left = left && right
	}
	return left, nil
}

func (p *parser) parseNot() (bool, error) {
	if p.peekWord("not") {
		p.consumeWord("not")
		v, err := p.parseAtom()
		if err != nil {
			return false, err
		}
		return !v, nil
	}
	return p.parseAtom()
}

func (p *parser) parseAtom() (bool, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return false, fmt.Errorf("unexpected end of filter at %d", p.pos)
	}
	if p.src[p.pos] == '(' {
		p.pos++
		v, err := p.parseOr()
		if err != nil {
			return false, err
		}
		p.skipSpace()
		if p.pos >= len(p.src) || p.src[p.pos] != ')' {
			return false, fmt.Errorf("expected ')' at %d", p.pos)
		}
		p.pos++
		return v, nil
	}

	attr, err := p.readIdent()
	if err != nil {
		return false, err
	}
	op, err := p.readOp()
	if err != nil {
		return false, err
	}
	value, err := p.readValue()
	if err != nil {
		return false, err
	}
	return evalOperator(op, attr, value)
}

func (p *parser) peekWord(w string) bool {
	p.skipSpace()
	if p.pos+len(w) > len(p.src) {
		return false
	}
	if !strings.EqualFold(p.src[p.pos:p.pos+len(w)], w) {
		return false
	}
	next := p.pos + len(w)
	if next < len(p.src) && (isIdentByte(p.src[next]) || p.src[next] == '(') {
		return false
	}
	return true
}

func (p *parser) consumeWord(w string) {
	p.skipSpace()
	p.pos += len(w)
}

func (p *parser) readIdent() (string, error) {
	p.skipSpace()
	start := p.pos
	for p.pos < len(p.src) && (isIdentByte(p.src[p.pos]) || p.src[p.pos] == '.') {
		p.pos++
	}
	if start == p.pos {
		return "", fmt.Errorf("expected attribute name at %d", p.pos)
	}
	return p.src[start:p.pos], nil
}

func (p *parser) readOp() (string, error) {
	p.skipSpace()
	for _, op := range []string{" eq ", " ne ", " co ", " sw ", " pr "} {
		if p.pos+len(op) <= len(p.src) && strings.EqualFold(p.src[p.pos:p.pos+len(op)], op) {
			p.pos += len(op)
			return strings.TrimSpace(op), nil
		}
	}
	// Some clients omit spaces; try a space-tolerant match.
	rest := p.src[p.pos:]
	for _, op := range []string{"eq", "ne", "co", "sw", "pr"} {
		if strings.HasPrefix(strings.ToLower(rest), op) {
			next := p.pos + len(op)
			if next < len(p.src) && !isOpBreak(p.src[next]) {
				continue
			}
			p.pos = next
			return op, nil
		}
	}
	return "", fmt.Errorf("expected comparison operator at %d", p.pos)
}

func (p *parser) readValue() (string, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return "", fmt.Errorf("expected value at %d", p.pos)
	}
	if p.src[p.pos] == '"' {
		p.pos++
		var b strings.Builder
		for p.pos < len(p.src) && p.src[p.pos] != '"' {
			if p.src[p.pos] == '\\' && p.pos+1 < len(p.src) {
				b.WriteByte(p.src[p.pos+1])
				p.pos += 2
				continue
			}
			b.WriteByte(p.src[p.pos])
			p.pos++
		}
		if p.pos >= len(p.src) {
			return "", fmt.Errorf("unterminated string at %d", p.pos)
		}
		p.pos++ // closing quote
		return b.String(), nil
	}
	// Unquoted identifier or boolean/null — read until whitespace or ')'.
	start := p.pos
	for p.pos < len(p.src) && !unicode.IsSpace(rune(p.src[p.pos])) && p.src[p.pos] != ')' {
		p.pos++
	}
	return p.src[start:p.pos], nil
}

func (p *parser) skipSpace() {
	for p.pos < len(p.src) && unicode.IsSpace(rune(p.src[p.pos])) {
		p.pos++
	}
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func isOpBreak(b byte) bool {
	return unicode.IsSpace(rune(b)) || b == '(' || b == ')'
}

// --- Evaluator ---

// userAttr is the resolver used by the SCIM user resource. It maps
// the filter attribute path to a string value (empty when unknown).
func userAttr(u *User) func(string) string {
	if u == nil {
		return func(string) string { return "" }
	}
	return func(path string) string {
		switch path {
		case "userName":
			return u.UserName
		case "id":
			return u.ID
		case "active":
			if u.Active {
				return "true"
			}
			return "false"
		case "name.givenName":
			if u.Name != nil {
				return u.Name.GivenName
			}
		case "name.familyName":
			if u.Name != nil {
				return u.Name.FamilyName
			}
		case "emails.value":
			for _, e := range u.Emails {
				return e.Value
			}
		}
		return ""
	}
}

// evalOperator applies one comparison and returns the result. Only
// eq/co/sw/pr are supported; other operators yield an error.
func evalOperator(op, attr, value string) (bool, error) {
	// We don't have the resource handy here; the caller supplies a
	// resolver via matchFilter. The parser deliberately returns
	// (false, nil) for the placeholder so the higher-level parser
	// can continue; matchFilter re-evaluates with the resolver.
	_ = attr
	_ = value
	switch op {
	case "eq", "ne", "co", "sw", "pr":
		return false, nil
	}
	return false, fmt.Errorf("unsupported operator %q", op)
}

// matchFilter walks the raw filter string and applies each leaf
// comparison using `resolve` for attribute lookup. Recursion
// mirrors the parser so we don't have to keep an AST around.
func matchFilter(s string, resolve func(string) string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	return evalOr(s, resolve)
}

func evalOr(s string, resolve func(string) string) bool {
	parts := splitTopLevel(s, "or")
	if len(parts) > 1 {
		for _, p := range parts {
			if evalAnd(p, resolve) {
				return true
			}
		}
		return false
	}
	return evalAnd(s, resolve)
}

func evalAnd(s string, resolve func(string) string) bool {
	parts := splitTopLevel(s, "and")
	if len(parts) > 1 {
		for _, p := range parts {
			if !evalNot(p, resolve) {
				return false
			}
		}
		return true
	}
	return evalNot(s, resolve)
}

func evalNot(s string, resolve func(string) string) bool {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "not ") {
		return !evalAtom(strings.TrimSpace(s[4:]), resolve)
	}
	return evalAtom(s, resolve)
}

func evalAtom(s string, resolve func(string) string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		return matchFilter(s[1:len(s)-1], resolve)
	}
	attr, op, value := splitLeaf(s)
	if op == "" {
		// Bare identifier — interpret as pr (present).
		return resolve(attr) != ""
	}
	actual := resolve(attr)
	switch op {
	case "eq":
		return actual == value
	case "ne":
		return actual != value
	case "co":
		return strings.Contains(strings.ToLower(actual), strings.ToLower(value))
	case "sw":
		return strings.HasPrefix(strings.ToLower(actual), strings.ToLower(value))
	case "pr":
		return actual != ""
	}
	return false
}

// splitTopLevel splits s on keyword at depth-0 only, ignoring
// occurrences inside parentheses or quoted strings.
func splitTopLevel(s, kw string) []string {
	var parts []string
	depth, quote := 0, byte(0)
	last := 0
	lower := strings.ToLower(s)
	for i := 0; i < len(s); i++ {
		b := s[i]
		if quote != 0 {
			if b == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if b == quote {
				quote = 0
			}
			continue
		}
		if b == '"' || b == '\'' {
			quote = b
			continue
		}
		if b == '(' {
			depth++
			continue
		}
		if b == ')' {
			depth--
			continue
		}
		if depth == 0 && i+len(kw)+1 <= len(s) {
			// Word boundary on both sides.
			if lower[i:i+len(kw)] == kw {
				left := i
				right := i + len(kw)
				if (left == 0 || !isIdentByte(s[left-1])) &&
					(right == len(s) || !isIdentByte(s[right])) {
					parts = append(parts, s[last:left])
					last = right
				}
			}
		}
	}
	parts = append(parts, s[last:])
	return parts
}

// splitLeaf splits a single comparison like `userName eq "alice"`.
// Returns attr, op, value with op=="" when the leaf is a bare attr.
func splitLeaf(s string) (attr, op, value string) {
	lower := strings.ToLower(s)
	for _, candidate := range []string{" eq ", " ne ", " co ", " sw ", " pr "} {
		if idx := strings.Index(lower, candidate); idx >= 0 {
			attr = strings.TrimSpace(s[:idx])
			op = strings.TrimSpace(candidate)
			value = strings.TrimSpace(s[idx+len(candidate):])
			value = strings.Trim(value, "\"")
			return
		}
	}
	return strings.TrimSpace(s), "", ""
}
