// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package redact

import (
	"regexp"
	"strings"
)

// ErrorDetailMaxLen caps a redacted error detail. A compose or qemu failure can
// carry a whole command transcript, and the cause is almost always in the first
// few lines, so the tail is cut rather than the envelope growing without bound.
const ErrorDetailMaxLen = 4000

// pemBlockRegex matches a whole PEM block, so a TLS error that echoes a key or
// certificate never carries it to the browser.
var pemBlockRegex = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]+-----.*?-----END [A-Z0-9 ]+-----`)

// authHeaderRegex matches an HTTP Authorization value echoed in an error.
var authHeaderRegex = regexp.MustCompile(`(?i)\b(bearer|basic|token)\s+[A-Za-z0-9._~+/=-]{8,}`)

// secretPairRegex matches key=value, key: value and "key":"value" pairs whose key
// names a credential. The key is kept, so the operator still learns which field
// the failing call carried.
var secretPairRegex = regexp.MustCompile(
	`(?i)("?\b[a-z0-9_.-]*(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|client[_-]?key-data|client[_-]?certificate-data|certificate-authority-data|credential|authorization|cookie|session[_-]?id)"?\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;&}\]]+)`)

// socketPathRegex matches a unix socket path or URL. Security rule: a socket
// path never reaches a client, and an error from a refused dial always names it.
var socketPathRegex = regexp.MustCompile(`(?:unix://)?/[A-Za-z0-9._/-]*\.sock\b`)

// ErrorDetail turns an error's text into the detail an API failure envelope may
// carry: credentials, tokens, keys, ciphertext and socket paths are cut, and the
// rest — the daemon's own sentence, the address that refused, the field that
// failed to parse — is kept because it is what the operator acts on.
func ErrorDetail(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = pemBlockRegex.ReplaceAllString(s, "[REDACTED:pem]")
	s = WideHardenedString(s)
	s = authHeaderRegex.ReplaceAllString(s, "$1 "+Placeholder)
	s = secretPairRegex.ReplaceAllString(s, "${1}"+Placeholder)
	s = encryptedBlobRegex.ReplaceAllString(s, Placeholder)
	s = socketPathRegex.ReplaceAllString(s, "[socket]")
	if len(s) > ErrorDetailMaxLen {
		cut := ErrorDetailMaxLen
		for cut > 0 && !utf8Start(s[cut]) {
			cut--
		}
		s = s[:cut] + " …"
	}
	return s
}

// utf8Start reports whether b begins a UTF-8 sequence, so a cut never splits a rune.
func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
