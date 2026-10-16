// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const redactedValue = "[REDACTED]"

var sensitiveKeyFragments = []string{
	"password",
	"token",
	"secret",
	"kubeconfig",
	"bearer_token",
	"bearertoken",
	"private_key",
	"privatekey",
}

// Diff contains sanitized before/after state for an audit entry.
type Diff struct {
	StateBefore string
	StateAfter  string
	ChangedKeys string
}

// NewDiff builds a redacted, JSON-encoded audit diff from arbitrary values.
func NewDiff(before, after any) (Diff, error) {
	beforeState, err := normalizeState(before)
	if err != nil {
		return Diff{}, fmt.Errorf("normalizing before state: %w", err)
	}
	afterState, err := normalizeState(after)
	if err != nil {
		return Diff{}, fmt.Errorf("normalizing after state: %w", err)
	}

	beforeJSON, err := json.Marshal(beforeState)
	if err != nil {
		return Diff{}, fmt.Errorf("marshaling before state: %w", err)
	}
	afterJSON, err := json.Marshal(afterState)
	if err != nil {
		return Diff{}, fmt.Errorf("marshaling after state: %w", err)
	}
	keysJSON, err := json.Marshal(changedKeys(beforeState, afterState))
	if err != nil {
		return Diff{}, fmt.Errorf("marshaling changed keys: %w", err)
	}

	return Diff{
		StateBefore: string(beforeJSON),
		StateAfter:  string(afterJSON),
		ChangedKeys: string(keysJSON),
	}, nil
}

// WithDiff returns a copy of the entry with sanitized structured state attached.
func (e Entry) WithDiff(before, after any) (Entry, error) {
	diff, err := NewDiff(before, after)
	if err != nil {
		return e, err
	}
	e.StateBefore = diff.StateBefore
	e.StateAfter = diff.StateAfter
	e.ChangedKeys = diff.ChangedKeys
	return e, nil
}

func normalizeState(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(b, &decoded); err != nil {
		return nil, err
	}
	return redactValue(decoded, ""), nil
}

func redactValue(value any, key string) any {
	if isSensitiveKey(key) {
		return redactedValue
	}

	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = redactValue(item, k)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactValue(item, key)
		}
		return out
	default:
		return value
	}
}

func changedKeys(before, after any) []string {
	beforeMap, beforeOK := before.(map[string]any)
	afterMap, afterOK := after.(map[string]any)
	if !beforeOK || !afterOK {
		if reflect.DeepEqual(before, after) {
			return []string{}
		}
		return []string{"$"}
	}

	seen := map[string]struct{}{}
	for key := range beforeMap {
		seen[key] = struct{}{}
	}
	for key := range afterMap {
		seen[key] = struct{}{}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		if !reflect.DeepEqual(beforeMap[key], afterMap[key]) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func isSensitiveKey(key string) bool {
	normalized := strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(strings.ToLower(key))
	for _, fragment := range sensitiveKeyFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
