// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package approvals

import (
	"bytes"
	"encoding/json"
	"strings"
)

const redactedValue = "[redacted]"

// sensitiveKeyFragments are matched case-insensitively against JSON keys. The
// list errs wide: an approver loses a little context, never a credential.
var sensitiveKeyFragments = []string{"password", "passwd", "secret", "token", "apikey", "api_key", "credential", "private", "authorization", "cookie", "passphrase", "key"}

// redactBody returns the canonical snapshot approvers see: JSON re-encoded with
// sorted keys and every sensitive value replaced. A body that is not JSON is
// withheld entirely, because its secrets cannot be located.
func redactBody(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return redactedValue
	}
	out, err := json.Marshal(redactValue(v))
	if err != nil {
		return redactedValue
	}
	return string(out)
}

func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, inner := range t {
			if isSensitiveKey(k) {
				t[k] = redactedValue
				continue
			}
			t[k] = redactValue(inner)
		}
		return t
	case []any:
		for i := range t {
			t[i] = redactValue(t[i])
		}
		return t
	default:
		return v
	}
}

func isSensitiveKey(k string) bool {
	lower := strings.ToLower(k)
	for _, frag := range sensitiveKeyFragments {
		if strings.Contains(lower, frag) {
			return true
		}
	}
	return false
}

// bodyField reads one top-level string field from a JSON body, for naming the
// resource a body-addressed route acts on.
func bodyField(raw []byte, field string) string {
	if field == "" || len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	s, _ := m[field].(string)
	return s
}

// bodyDryRun reports a top-level "dryRun": true.
func bodyDryRun(raw []byte) bool {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	b, _ := m["dryRun"].(bool)
	return b
}
