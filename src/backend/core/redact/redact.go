// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package redact strips secrets from values before they leave the process —
// into an LLM prompt, an audit trail or incident evidence — so every exit
// applies one pattern set.
package redact

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Profile controls how aggressive redaction is for a tool's
// output. Read tools default to Standard. Tools that handle already
// redacted data (audit logs, settings) can use Permissive.
type Profile string

const (
	// Standard strips passwords/tokens/api_keys/kubeconfigs and
	// masks IPs and emails. Use for tool output that crosses the LLM
	// boundary.
	Standard Profile = "standard"

	// Permissive only strips secret-shaped keys. Use for tools
	// whose output is already redacted upstream (e.g. audit log
	// search).
	Permissive Profile = "permissive"
)

// sensitiveKeyRegex matches JSON keys whose values should be stripped
// regardless of depth.
var sensitiveKeyRegex = regexp.MustCompile(
	`(?i)\b(password|passwd|pwd|token|secret|api_?key|client_?secret|private_?key|bearer|kubeconfig|tls_?key|auth_?token|access_?token|refresh_?token|session_?id|cookie)\b`)

// envSecretRegex matches ENV-style keys whose VALUES contain secrets.
var envSecretRegex = regexp.MustCompile(
	`(?i)^[A-Z0-9_]*(KEY|SECRET|TOKEN|PASSWORD|PASSWD|CREDENTIAL)\b`)

// emailRegex is intentionally permissive; the redaction mask keeps the
// domain so the model can reason about scope.
var emailRegex = regexp.MustCompile(
	`([a-zA-Z0-9._%+-])[a-zA-Z0-9._%+-]*(@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)

// ipv4Regex captures the last octet so we can replace it with 'x'.
var ipv4Regex = regexp.MustCompile(
	`(\d{1,3}\.\d{1,3}\.\d{1,3})\.\d{1,3}`)

// highEntropyRegex matches ≥32-char runs of [A-Za-z0-9+/=_-] — typical
// accidental-secret signature inside free-text tool output.
var highEntropyRegex = regexp.MustCompile(
	`[A-Za-z0-9+/=_-]{32,}`)

// encryptedBlobRegex matches McHarbor's own ciphertext envelope
// (`core/encryption`, `enc:v1:` + base64). A blob is useless to the model and
// it is a stored credential, so it never crosses the LLM boundary — not even
// as ciphertext, which would otherwise be one offline key compromise away
// from a plaintext leak through whatever the model's transcript is retained
// in.
var encryptedBlobRegex = regexp.MustCompile(`enc:v1:[A-Za-z0-9+/=]+`)

// A `${secret:NAME}` reference is deliberately NOT redacted: it is a name,
// not a value, and it is useful context for the model. Only the resolved
// value is dangerous, and resolution never happens on a path that reaches
// assistant context — see modules/secrets/resolver.go.

// Placeholder is the text substituted for stripped secrets.
const Placeholder = "[REDACTED]"

// Value walks an arbitrary value (typically a map[string]any or
// []any produced by json.Unmarshal) and returns a copy with secrets
// stripped. Original input is not mutated.
func Value(v any, profile Profile) any {
	switch profile {
	case Permissive:
		return redactMap(v, true)
	default:
		return redactMap(v, false)
	}
}

func redactMap(v any, onlyKeys bool) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if sensitiveKeyRegex.MatchString(k) {
				out[k] = Placeholder
				continue
			}
			out[k] = redactMap(val, onlyKeys)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactMap(val, onlyKeys)
		}
		return out
	case string:
		return redactString(t, onlyKeys)
	default:
		return v
	}
}

func redactString(s string, onlyKeys bool) string {
	if s == "" {
		return s
	}
	// Whole-token patterns run before the entropy mask below, which would
	// otherwise cut a JWT's signature out and leave its claims unrecognised.
	s = HardenedString(s)
	// Key-shaped strings (e.g. env-var dump rows): redact the value
	// half of `KEY=VALUE` rows when the key matches envSecretRegex.
	if !onlyKeys {
		s = redactEnvLine(s)
	}
	// Ciphertext first: an enc:v1: blob is base64 and would otherwise be
	// caught by highEntropyRegex only when it happens to be long enough and
	// only when onlyKeys is false. It is a stored credential either way.
	s = encryptedBlobRegex.ReplaceAllString(s, Placeholder)
	s = emailRegex.ReplaceAllString(s, "$1***$2")
	s = ipv4Regex.ReplaceAllString(s, "$1.x")
	if !onlyKeys {
		s = highEntropyRegex.ReplaceAllStringFunc(s, func(match string) string {
			if looksLikeNaturalLanguage(match) {
				return match
			}
			return Placeholder
		})
	}
	return s
}

// redactEnvLine handles "KEY=VALUE\n..." style strings (typical of
// container inspect output and env dumps).
func redactEnvLine(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := line[:eq]
		if envSecretRegex.MatchString(key) {
			val := line[eq+1:]
			lines[i] = key + "=" + Placeholder + suffixIfNotEmpty(val)
			_ = val
		}
	}
	return strings.Join(lines, "\n")
}

func suffixIfNotEmpty(_ string) string { return "" }

// looksLikeNaturalLanguage rejects obvious English / Docker output
// that happens to be ≥32 chars. We only mask high-entropy strings
// that look like base64 / hex.
func looksLikeNaturalLanguage(s string) bool {
	// All-uppercase + spaces → English-y. Allow through.
	hasSpace := strings.ContainsAny(s, " \t")
	hasLower := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			hasLower = true
			break
		}
	}
	if hasSpace && hasLower {
		return true
	}
	// Pure-digit runs are not secrets (timestamps, IDs).
	onlyDigits := true
	for _, r := range s {
		if r < '0' || r > '9' {
			onlyDigits = false
			break
		}
	}
	return onlyDigits
}

// JSON is a convenience wrapper: marshal-then-redecode is too
// slow, so callers should normally use Value on a value already in
// memory. Provided for the rare handler that only has the raw bytes.
func JSON(raw json.RawMessage, profile Profile) json.RawMessage {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	out := Value(v, profile)
	enc, err := json.Marshal(out)
	if err != nil {
		return raw
	}
	return enc
}

// --- Phase 7: hardened patterns + prompt-injection sanitization ---

// agentTokenRegex matches McHarbor agent tokens (agt_<32 hex>).
var agentTokenRegex = regexp.MustCompile(`\bagt_[0-9a-fA-F]{32,}\b`)

// jwtRegex matches the three-segment JWT shape so we can mask it
// wholesale. We do not try to validate the signature; the goal is
// to scrub the whole token, not to confirm it is signed.
var jwtRegex = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)

// connCredsRegex matches URLs with embedded credentials
// (scheme://user:password@host).
var connCredsRegex = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://)([^:/@\s]+):([^@\s]+)@`)

// dockerCfgAuthRegex matches `"auth": "<base64>"` values in
// dockerconfigjson payloads. The whole value is replaced with a
// `[REDACTED]` placeholder.
var dockerCfgAuthRegex = regexp.MustCompile(`"auth"\s*:\s*"[A-Za-z0-9+/=._-]{8,}"`)

// injectionPhrases is a closed set of phrases that strongly suggest
// the tool output is trying to hijack the LLM. Matching is case-
// insensitive. The list is intentionally small to limit false
// positives on normal log output.
var injectionPhrases = []string{
	"ignore previous instructions",
	"ignore all previous",
	"disregard the above",
	"forget everything above",
	"new instructions:",
	"system: you are",
	"<|im_start|>",
	"<|im_end|>",
	"### instruction:",
	"### response:",
	"</s><s>",
}

// HardenedString applies the extra Phase 7 patterns on top
// of the base RedactString behaviour. Safe to call on already-
// redacted output (idempotent).
func HardenedString(s string) string {
	if s == "" {
		return s
	}
	s = agentTokenRegex.ReplaceAllString(s, "[REDACTED:agent_token]")
	s = jwtRegex.ReplaceAllString(s, "[REDACTED:jwt]")
	s = connCredsRegex.ReplaceAllString(s, "$1$2:[REDACTED]@")
	s = dockerCfgAuthRegex.ReplaceAllString(s, `"auth":"[REDACTED]"`)
	return s
}

// Hardened walks a value and applies HardenedString
// to every string it finds. Map keys are preserved.
func Hardened(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = Hardened(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = Hardened(val)
		}
		return out
	case string:
		return HardenedString(t)
	default:
		return v
	}
}

// DetectInjection scans a string for known prompt-injection
// phrases. Returns the first matching phrase (lower-cased), or ""
// when clean. The caller decides whether to redact, warn, or
// reject.
func DetectInjection(s string) string {
	low := strings.ToLower(s)
	for _, p := range injectionPhrases {
		if strings.Contains(low, p) {
			return p
		}
	}
	return ""
}

// SanitizeResult runs Hardened + DetectInjection on a tool
// result. When an injection attempt is detected, the returned
// "sanitized" flag is true and the value is replaced with a small
// envelope that explains the situation without forwarding the
// hostile content to the model. The Sanitized value, the matched
// phrase, and the boolean are returned so the caller can audit-log
// and surface a UI warning.
func SanitizeResult(v any) (out any, sanitized bool, matched string) {
	switch t := v.(type) {
	case string:
		cleaned := HardenedString(t)
		if phrase := DetectInjection(cleaned); phrase != "" {
			return map[string]any{
				"sanitized": true,
				"reason":    "prompt_injection_detected",
				"matched":   phrase,
				"note":      "Tool output contained a phrase that looks like a prompt-injection attempt. The original content is not forwarded to the model.",
				"hint":      "Use the tool again with a narrower query, or call entity.relate to inspect the underlying entity directly.",
			}, true, phrase
		}
		return cleaned, false, ""
	default:
		cleaned := Hardened(v)
		return cleaned, false, ""
	}
}
