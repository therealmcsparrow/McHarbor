// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package versionorder is the one version order shared by updates, catalogs and
// plugin floors; its cases live in src/contracts/versionorder/cases.yaml.
package versionorder

import (
	"errors"
	"strings"
)

// maxLen bounds the work for hostile input arriving from catalogs and release feeds.
const maxLen = 256

// ErrInvalid is returned by Parse for anything that is not MAJOR.MINOR.PATCH[-pre][+meta].
var ErrInvalid = errors.New("invalid version")

// Version is a parsed version; build metadata is dropped because it never affects order.
type Version struct {
	core [3]string
	pre  []string
}

// IsPrerelease reports whether the version carries a "-" suffix.
func (v Version) IsPrerelease() bool { return len(v.pre) > 0 }

// Parse reads s strictly and fails on anything unparsable, for callers (floors,
// self-update targets) that must refuse bad input rather than read it as 0.
func Parse(s string) (Version, error) {
	var v Version
	if s == "" || len(s) > maxLen {
		return v, ErrInvalid
	}
	rest := s
	if i := strings.IndexByte(rest, '+'); i >= 0 {
		if !validIdents(rest[i+1:], true) {
			return v, ErrInvalid
		}
		rest = rest[:i]
	}
	if i := strings.IndexByte(rest, '-'); i >= 0 {
		pre := rest[i+1:]
		if !validIdents(pre, false) {
			return v, ErrInvalid
		}
		v.pre = strings.Split(pre, ".")
		rest = rest[:i]
	}
	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return v, ErrInvalid
	}
	for i, p := range parts {
		if p == "" || !isDigits(p) {
			return v, ErrInvalid
		}
		v.core[i] = trimZeros(p)
	}
	return v, nil
}

// Compare orders a against b as -1, 0 or 1. Unparsable input reads as 0.0.0, so a
// dev build sorts below every release, which is how the agent gates already read it.
func Compare(a, b string) int {
	return parseLenient(a).compare(parseLenient(b))
}

// IsPrerelease reports whether s parses and carries a "-" suffix; a "+" suffix is not one.
func IsPrerelease(s string) bool {
	v, err := Parse(s)
	return err == nil && v.IsPrerelease()
}

// IsPlainStable reports whether s is a strictly parsable version with no "-" suffix. Unlike
// !IsPrerelease it fails closed: "rc-1", "V1.2.3" and anything else Parse refuses are not stable.
func IsPlainStable(s string) bool {
	v, err := Parse(s)
	return err == nil && !v.IsPrerelease()
}

func parseLenient(s string) Version {
	v, err := Parse(s)
	if err != nil {
		return Version{core: [3]string{"0", "0", "0"}}
	}
	return v
}

func (v Version) compare(o Version) int {
	for i := range v.core {
		if c := cmpNumeric(v.core[i], o.core[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(v.pre) == 0 && len(o.pre) == 0:
		return 0
	case len(v.pre) == 0:
		return 1
	case len(o.pre) == 0:
		return -1
	}
	for i := 0; i < len(v.pre) && i < len(o.pre); i++ {
		if c := cmpIdent(v.pre[i], o.pre[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(v.pre), len(o.pre))
}

func cmpIdent(a, b string) int {
	an, bn := isDigits(a), isDigits(b)
	switch {
	case an && bn:
		return cmpNumeric(trimZeros(a), trimZeros(b))
	case an:
		return -1
	case bn:
		return 1
	}
	return strings.Compare(a, b)
}

// cmpNumeric compares zero-trimmed digit strings by length then bytes, so a value
// of any size orders correctly without being converted to an integer.
func cmpNumeric(a, b string) int {
	if c := cmpInt(len(a), len(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func trimZeros(s string) string {
	t := strings.TrimLeft(s, "0")
	if t == "" {
		return "0"
	}
	return t
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// validIdents accepts dot-separated non-empty [0-9A-Za-z] identifiers; build
// metadata additionally allows "-", the prerelease form does not so "..", "/" and "-" stay impossible.
func validIdents(s string, allowHyphen bool) bool {
	if s == "" {
		return false
	}
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return false
		}
		for i := 0; i < len(id); i++ {
			c := id[i]
			ok := c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || (allowHyphen && c == '-')
			if !ok {
				return false
			}
		}
	}
	return true
}
