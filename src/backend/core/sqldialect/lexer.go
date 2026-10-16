// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package sqldialect

import "strings"

type tokKind int

const (
	tokWord tokKind = iota
	tokQuoted
	tokString
	tokNumber
	tokPunct
	tokParam
)

// token is one lexeme with its byte span in the source, so a rewrite can splice the
// statement without re-rendering anything it did not touch.
type token struct {
	kind       tokKind
	start, end int
	text       string
}

// lex splits a statement into tokens, dropping whitespace and comments. String
// literals use SQL doubling for an embedded quote; the MySQL session runs with
// NO_BACKSLASH_ESCAPES, so a backslash is an ordinary character there too.
func lex(q string) []token {
	var out []token
	i := 0
	for i < len(q) {
		c := q[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '-' && i+1 < len(q) && q[i+1] == '-':
			j := strings.IndexByte(q[i:], '\n')
			if j < 0 {
				return out
			}
			i += j + 1
		case c == '/' && i+1 < len(q) && q[i+1] == '*':
			j := strings.Index(q[i+2:], "*/")
			if j < 0 {
				return out
			}
			i += 2 + j + 2
		case c == '\'':
			j := skipQuoted(q, i, '\'')
			out = append(out, token{kind: tokString, start: i, end: j, text: q[i:j]})
			i = j
		case c == '"' || c == '`':
			j := skipQuoted(q, i, c)
			inner := q[i+1 : max(i+1, j-1)]
			out = append(out, token{kind: tokQuoted, start: i, end: j, text: strings.ToLower(inner)})
			i = j
		case isWordStart(c):
			j := i + 1
			for j < len(q) && isWordPart(q[j]) {
				j++
			}
			out = append(out, token{kind: tokWord, start: i, end: j, text: strings.ToLower(q[i:j])})
			i = j
		case c >= '0' && c <= '9':
			j := i + 1
			for j < len(q) && (isWordPart(q[j]) || q[j] == '.') {
				j++
			}
			out = append(out, token{kind: tokNumber, start: i, end: j, text: q[i:j]})
			i = j
		case c == '?':
			out = append(out, token{kind: tokParam, start: i, end: i + 1, text: "?"})
			i++
		default:
			n := 1
			if i+1 < len(q) {
				switch q[i : i+2] {
				case "<=", ">=", "<>", "!=", "||", "==":
					n = 2
				}
			}
			out = append(out, token{kind: tokPunct, start: i, end: i + n, text: q[i : i+n]})
			i += n
		}
	}
	return out
}

// skipQuoted returns the index just past the quote that closes the one at i; an
// unterminated quote runs to the end of the statement.
func skipQuoted(q string, i int, quote byte) int {
	j := i + 1
	for j < len(q) {
		if q[j] == quote {
			if j+1 < len(q) && q[j+1] == quote {
				j += 2
				continue
			}
			return j + 1
		}
		j++
	}
	return len(q)
}

func isWordStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isWordPart(c byte) bool {
	return isWordStart(c) || (c >= '0' && c <= '9') || c == '$'
}

// isName reports a token that can name a table or column.
func (t token) isName() bool { return t.kind == tokWord || t.kind == tokQuoted }

func (t token) is(text string) bool {
	return (t.kind == tokWord || t.kind == tokPunct) && t.text == text
}

// at returns the token at i, or a zero token outside the slice, so neighbour checks
// never need a bounds test.
func at(toks []token, i int) token {
	if i < 0 || i >= len(toks) {
		return token{kind: tokPunct, start: -1, end: -1}
	}
	return toks[i]
}
