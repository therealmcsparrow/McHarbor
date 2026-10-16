// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"fmt"
	"strings"
)

type requirement struct {
	key    string
	op     string
	values []string
}

type labelSelector []requirement

func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, ch := range s {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

func parseLabelSelector(raw string) (labelSelector, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var sel labelSelector
	for _, term := range splitTopLevel(raw) {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		fields := strings.Fields(term)
		switch {
		case len(fields) >= 3 && (fields[1] == "in" || fields[1] == "notin"):
			set := strings.TrimSpace(strings.Join(fields[2:], " "))
			if !strings.HasPrefix(set, "(") || !strings.HasSuffix(set, ")") {
				return nil, fmt.Errorf("%q: expected a parenthesised value set", term)
			}
			var vals []string
			for _, v := range strings.Split(strings.Trim(set, "()"), ",") {
				vals = append(vals, strings.TrimSpace(v))
			}
			sel = append(sel, requirement{key: fields[0], op: fields[1], values: vals})
		case strings.Contains(term, "!="):
			kv := strings.SplitN(term, "!=", 2)
			sel = append(sel, requirement{key: strings.TrimSpace(kv[0]), op: "!=", values: []string{strings.TrimSpace(kv[1])}})
		case strings.Contains(term, "=="):
			kv := strings.SplitN(term, "==", 2)
			sel = append(sel, requirement{key: strings.TrimSpace(kv[0]), op: "=", values: []string{strings.TrimSpace(kv[1])}})
		case strings.Contains(term, "="):
			kv := strings.SplitN(term, "=", 2)
			sel = append(sel, requirement{key: strings.TrimSpace(kv[0]), op: "=", values: []string{strings.TrimSpace(kv[1])}})
		case strings.HasPrefix(term, "!"):
			sel = append(sel, requirement{key: strings.TrimSpace(term[1:]), op: "!exists"})
		default:
			if strings.ContainsAny(term, " ()") {
				return nil, fmt.Errorf("%q: unsupported selector syntax", term)
			}
			sel = append(sel, requirement{key: term, op: "exists"})
		}
	}
	return sel, nil
}

func (sel labelSelector) matches(labels map[string]string) bool {
	for _, r := range sel {
		v, has := labels[r.key]
		switch r.op {
		case "=":
			if !has || v != r.values[0] {
				return false
			}
		case "!=":
			if has && v == r.values[0] {
				return false
			}
		case "exists":
			if !has {
				return false
			}
		case "!exists":
			if has {
				return false
			}
		case "in":
			if !has || !contains(r.values, v) {
				return false
			}
		case "notin":
			if has && contains(r.values, v) {
				return false
			}
		}
	}
	return true
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

type fieldSelector []requirement

func parseFieldSelector(raw string) (fieldSelector, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var sel fieldSelector
	for _, term := range strings.Split(raw, ",") {
		term = strings.TrimSpace(term)
		switch {
		case strings.Contains(term, "!="):
			kv := strings.SplitN(term, "!=", 2)
			sel = append(sel, requirement{key: kv[0], op: "!=", values: []string{kv[1]}})
		case strings.Contains(term, "=="):
			kv := strings.SplitN(term, "==", 2)
			sel = append(sel, requirement{key: kv[0], op: "=", values: []string{kv[1]}})
		case strings.Contains(term, "="):
			kv := strings.SplitN(term, "=", 2)
			sel = append(sel, requirement{key: kv[0], op: "=", values: []string{kv[1]}})
		default:
			return nil, fmt.Errorf("invalid field selector: %q", term)
		}
	}
	return sel, nil
}

func (sel fieldSelector) matches(obj M) bool {
	for _, r := range sel {
		v := fieldValue(obj, r.key)
		if (r.op == "=") != (v == r.values[0]) {
			return false
		}
	}
	return true
}

// fieldValue resolves a dotted field path the way the apiserver's field label
// conversion does for the handful of fields anyone selects on.
func fieldValue(obj M, path string) string {
	var cur any = obj
	for _, part := range strings.Split(path, ".") {
		switch m := cur.(type) {
		case M:
			cur = m[part]
		case map[string]string:
			cur = m[part]
		default:
			return ""
		}
	}
	switch v := cur.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}
