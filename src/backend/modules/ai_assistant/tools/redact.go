// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"encoding/json"

	"github.com/therealmcsparrow/mcharbor/core/redact"
)

// RedactionProfile controls how aggressive redaction is for a tool's output;
// it is core/redact's Profile, so tools and other redacting modules agree.
type RedactionProfile = redact.Profile

const (
	// RedactStandard strips passwords/tokens/api_keys/kubeconfigs and masks
	// IPs and emails. Use for tool output that crosses the LLM boundary.
	RedactStandard = redact.Standard

	// RedactPermissive only strips secret-shaped keys. Use for tools whose
	// output is already redacted upstream (e.g. audit log search).
	RedactPermissive = redact.Permissive
)

// RedactedValue is the placeholder substituted for stripped secrets.
const RedactedValue = redact.Placeholder

// Redact returns a copy of v with secrets stripped per profile; the original
// input is not mutated.
func Redact(v any, profile RedactionProfile) any { return redact.Value(v, profile) }

// RedactJSON redacts raw bytes for the rare handler that has no decoded value,
// returning raw unchanged when it is not valid JSON.
func RedactJSON(raw json.RawMessage, profile RedactionProfile) json.RawMessage {
	return redact.JSON(raw, profile)
}

// HardenedRedactString masks agent tokens, JWTs, URL credentials and
// dockerconfig auth values; idempotent.
func HardenedRedactString(s string) string { return redact.HardenedString(s) }

// HardenedRedact applies HardenedRedactString to every string inside v.
func HardenedRedact(v any) any { return redact.Hardened(v) }

// DetectInjection returns the first known prompt-injection phrase in s,
// lower-cased, or "" when clean.
func DetectInjection(s string) string { return redact.DetectInjection(s) }

// SanitizeResult hardens a tool result and replaces it with a warning envelope
// when it carries a prompt-injection phrase, so hostile text never reaches the model.
func SanitizeResult(v any) (out any, sanitized bool, matched string) {
	return redact.SanitizeResult(v)
}
