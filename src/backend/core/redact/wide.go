// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package redact

import "regexp"

// wideConnStringRegex differs from connCredsRegex by letting the user segment
// carry '/' and '@', so a credential after a malformed user part is still cut.
var wideConnStringRegex = regexp.MustCompile(
	`([a-zA-Z][a-zA-Z0-9+.\-]*://)([^:\s]+):([^@\s]+)@`)

// WideHardenedString is HardenedString with the wider connection-string match
// the AI assistant boundary and incident evidence use. Idempotent.
func WideHardenedString(s string) string {
	if s == "" {
		return s
	}
	s = agentTokenRegex.ReplaceAllString(s, "[REDACTED:agent_token]")
	s = jwtRegex.ReplaceAllString(s, "[REDACTED:jwt]")
	s = wideConnStringRegex.ReplaceAllString(s, "$1$2:[REDACTED]@")
	s = dockerCfgAuthRegex.ReplaceAllString(s, `"auth":"[REDACTED]"`)
	return s
}

// WideHardened walks a value and applies WideHardenedString to every string it
// finds; map keys are preserved and the input is not mutated.
func WideHardened(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = WideHardened(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = WideHardened(val)
		}
		return out
	case string:
		return WideHardenedString(t)
	default:
		return v
	}
}
