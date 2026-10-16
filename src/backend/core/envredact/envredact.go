// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package envredact decides which environment values may leave McHarbor in the clear. Every
// surface that shows a container's or a stack's environment asks it, so the inspect page, the
// AI assistant, workflows and the support bundle cannot disagree about what a secret is.
package envredact

import (
	"regexp"
	"strings"

	"github.com/moby/moby/api/types/container"

	"github.com/therealmcsparrow/mcharbor/core/config"
)

// Marker replaces a withheld value. It is the same run of bullets the stack env editor has
// always used, so one merge rule — "the marker means unchanged" — covers both write paths.
const Marker = "••••••••"

// sensitiveName matches variable and field names that carry credentials. A false positive
// costs the operator one Reveal click; a false negative leaks a credential, so it is broad.
var sensitiveName = regexp.MustCompile(`(?i)(key|secret|token|passw|pwd|dsn|credential|private|cookie|session|auth_?header|bearer|cert_?path|pass\b|_pass_|passphrase|(^|_)auth($|_)|connection_?string|salt|kubeconfig|signing)`)

// KeyValuePattern matches a credential written as an assignment inside a longer value: an
// ADO.NET `Password=…;`, a `-Dapp.password=…` flag, a `"token": "…"` pair. Group 1 is the
// name and separator, group 3 the value. The support scrubber shares it.
var KeyValuePattern = regexp.MustCompile(`(?i)("?[A-Za-z0-9_.-]*(password|passwd|pwd|secret|token|api[_-]?key|apikey|access[_-]?key|private[_-]?key|authorization|credential|kubeconfig|bearer)[A-Za-z0-9_.-]*"?\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;&}"']+)`)

// QueryCredentialPattern matches a credential in a URL query or JDBC parameter list, such as
// `?password=…` or `&access_token=…`. Group 1 is the name and `=`.
var QueryCredentialPattern = regexp.MustCompile(`(?i)([?&;](password|pass|pwd|token|secret|key|access_token|api_key|apikey|signature|sig)=)[^&;\s"]+`)

// BearerPattern matches an HTTP authorization scheme followed by its credential, as in
// `Authorization: Bearer …`. Group 1 is the scheme. The support scrubber shares it.
var BearerPattern = regexp.MustCompile(`(?i)\b(bearer|basic|token)\s+[A-Za-z0-9._~+/=-]{8,}`)

// TokenPattern matches credentials by their shape alone: GitHub, Slack, AWS and Stripe keys,
// JSON Web Tokens, a base64-encoded PEM block, and chat webhook URLs whose path is the secret.
var TokenPattern = regexp.MustCompile(`(\bgh[pousr]_[A-Za-z0-9]{20,}|\bgithub_pat_[A-Za-z0-9_]{20,}|\bxox[abposr]-[A-Za-z0-9-]{10,}|\b(AKIA|ASIA)[0-9A-Z]{16}\b|\b[sr]k_live_[0-9A-Za-z]{16,}|\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}|LS0tLS1CRUdJTi[A-Za-z0-9+/=]*|(?i:https?://(hooks\.slack\.com/(services|workflows|triggers)/|(ptb\.|canary\.)?discord(app)?\.com/api/webhooks/|[a-z0-9.-]*webhook\.office\.com/|outlook\.office(365)?\.com/webhook/|hooks\.zapier\.com/hooks/)[^\s"']+))`)

// trivialValue matches values no credential takes — a flag or a small number — so a name
// such as SESSION_TIMEOUT=3600 or USE_TOKEN_AUTH=true is not masked on its name alone.
var trivialValue = regexp.MustCompile(`(?i)^(true|false|yes|no|on|off|null|none|enabled|disabled|[0-9]{1,5})$`)

// sensitiveNameExceptions are names that merely contain a sensitive word.
var sensitiveNameExceptions = map[string]bool{"AUTH_DISABLE": true, "FORCE_SECURE_COOKIES": true}

// rootOfTrust names the variables that hold McHarbor's own keys. Whoever reads one can sign
// node-link calls or decrypt every stored credential, so on a McHarbor container they are
// withheld from everybody, whatever permission the caller holds.
var rootOfTrust = map[string]bool{
	"MCHARBOR_SECRET":     true,
	"ENCRYPTION_KEY":      true,
	"MCHARBOR_DB_DSN":     true,
	"MCHARBOR_BACKUP_KEY": true,
}

// credentialURL matches a URL that carries a password in its userinfo, which is how a
// DATABASE_URL-style variable leaks a credential under a name no heuristic recognises.
var credentialURL = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^/\s:@]*:[^/\s@]+@`)

// minPinnedLength keeps a short pinned value from matching unrelated text by accident.
const minPinnedLength = 8

// SensitiveName reports whether name looks like it carries a credential.
func SensitiveName(name string) bool {
	if sensitiveNameExceptions[name] {
		return false
	}
	return sensitiveName.MatchString(name)
}

// RootOfTrustName reports whether name is one of McHarbor's own key variables.
func RootOfTrustName(name string) bool {
	return rootOfTrust[name]
}

// sensitiveValue reports whether a value is a credential whatever its variable is called.
func sensitiveValue(value string) bool {
	return credentialURL.MatchString(value) || strings.Contains(value, "-----BEGIN ") ||
		KeyValuePattern.MatchString(value) || QueryCredentialPattern.MatchString(value) || TokenPattern.MatchString(value)
}

// SensitiveValue reports whether value carries a credential by its shape, whatever name it
// is stored under.
func SensitiveValue(value string) bool {
	return sensitiveValue(value)
}

// Policy is one caller's view of an environment: what they may reveal, and what nobody may.
type Policy struct {
	// Reveal is true when the caller holds the reveal permission for this target.
	Reveal bool
	// McHarbor is true when the target is a McHarbor application container, whose
	// root-of-trust variables stay withheld even when Reveal is true.
	McHarbor bool
	// Pinned are the root-of-trust values this process runs with; a value containing one is
	// always withheld, which catches a peer node or a stack that copied the secret under
	// another name.
	Pinned []string
}

// PinnedFromConfig returns the root-of-trust values cfg runs with, plus any extra key
// material the caller holds (the backup key), ready for Policy.Pinned.
func PinnedFromConfig(cfg *config.Config, extra ...string) []string {
	if cfg == nil {
		return PinnedValues(extra...)
	}
	return PinnedValues(append([]string{cfg.Secret, cfg.EncryptionKey, cfg.DBDSN}, extra...)...)
}

// PinnedValues keeps the values long enough to match safely, for Policy.Pinned.
func PinnedValues(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if len(v) >= minPinnedLength {
			out = append(out, v)
		}
	}
	return out
}

// Withhold reports whether the value of name must be replaced by the Marker for this caller.
func (p Policy) Withhold(name, value string) bool {
	if value == "" || value == Marker {
		return false
	}
	for _, pinned := range p.allPinned() {
		if pinned != "" && strings.Contains(value, pinned) {
			return true
		}
	}
	if p.McHarbor && RootOfTrustName(name) {
		return true
	}
	if p.Reveal {
		return false
	}
	if RootOfTrustName(name) || sensitiveValue(value) {
		return true
	}
	return SensitiveName(name) && !trivialValue.MatchString(strings.TrimSpace(value))
}

// neverRevealed reports whether value stays withheld even from a caller allowed to reveal:
// it carries a pinned key, or it is a root-of-trust variable of a McHarbor container.
func (p Policy) neverRevealed(name, value string) bool {
	q := p
	q.Reveal = true
	return q.Withhold(name, value)
}

// MaskInline returns value with every credential inside it replaced by the Marker, leaving
// the rest readable: `--password=••••••••` rather than a whole masked command line.
func (p Policy) MaskInline(value string) string {
	if value == "" {
		return value
	}
	out := value
	for _, pinned := range p.allPinned() {
		if pinned != "" {
			out = strings.ReplaceAll(out, pinned, Marker)
		}
	}
	if p.Reveal {
		return out
	}
	out = BearerPattern.ReplaceAllString(out, "${1} "+Marker)
	out = KeyValuePattern.ReplaceAllStringFunc(out, func(m string) string {
		sub := KeyValuePattern.FindStringSubmatch(m)
		if strings.Contains(sub[3], "${") || sub[3] == Marker || bearerScheme.MatchString(sub[3]) {
			return m
		}
		return sub[1] + Marker
	})
	out = QueryCredentialPattern.ReplaceAllString(out, "${1}"+Marker)
	out = TokenPattern.ReplaceAllString(out, Marker)
	out = credentialURLPassword.ReplaceAllString(out, "${1}"+Marker+"@")
	if strings.Contains(out, "-----BEGIN ") {
		return Marker
	}
	return out
}

// bearerScheme is an authorization scheme word, which BearerPattern has already handled.
var bearerScheme = regexp.MustCompile(`(?i)^(bearer|basic|token)$`)

// credentialURLPassword captures the part of a URL before its password, for MaskInline.
var credentialURLPassword = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^/\s:@]*:)[^/\s@]+@`)

// MovedWithheld lists the names whose incoming value is the Marker and whose current value
// nobody may reveal. A recreate that keeps one while changing the image or command would
// hand a value the editor cannot read to a program they chose.
func (p Policy) MovedWithheld(incoming, current []string) []string {
	values := make(map[string]string, len(current))
	for _, entry := range current {
		if name, value, ok := strings.Cut(entry, "="); ok {
			if _, seen := values[name]; !seen {
				values[name] = value
			}
		}
	}
	var out []string
	for _, entry := range incoming {
		name, value, ok := strings.Cut(entry, "=")
		if ok && value == Marker && p.neverRevealed(name, values[name]) {
			out = append(out, name)
		}
	}
	return out
}

// EnvList returns a copy of a Docker-style KEY=VALUE list with withheld values replaced by
// the Marker, and the names it withheld. The input is never modified.
func (p Policy) EnvList(env []string) ([]string, []string) {
	if env == nil {
		return nil, nil
	}
	out := make([]string, len(env))
	var withheld []string
	for i, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if ok && p.Withhold(name, value) {
			out[i] = name + "=" + Marker
			withheld = append(withheld, name)
			continue
		}
		out[i] = entry
	}
	return out, withheld
}

// EnvMap is EnvList for a name-to-value map.
func (p Policy) EnvMap(env map[string]string) (map[string]string, []string) {
	if env == nil {
		return nil, nil
	}
	out := make(map[string]string, len(env))
	var withheld []string
	for name, value := range env {
		if p.Withhold(name, value) {
			out[name] = Marker
			withheld = append(withheld, name)
			continue
		}
		out[name] = value
	}
	return out, withheld
}

// Container redacts info's environment in place and returns the names it withheld. Config is
// replaced by a copy rather than edited, so a caller's other reference to it keeps the values.
func (p Policy) Container(info *container.InspectResponse) []string {
	if info == nil || info.Config == nil {
		return nil
	}
	cfg := *info.Config
	env, withheld := p.EnvList(cfg.Env)
	cfg.Env = env
	info.Config = &cfg
	return withheld
}

// Restore puts the current value back wherever incoming still carries the Marker, so an
// editor that round-trips a redacted list cannot overwrite a credential with bullets. A
// marker whose name has no current value becomes empty, never the Marker itself.
func Restore(incoming, current []string) []string {
	if incoming == nil {
		return nil
	}
	values := make(map[string]string, len(current))
	for _, entry := range current {
		if name, value, ok := strings.Cut(entry, "="); ok {
			if _, seen := values[name]; !seen {
				values[name] = value
			}
		}
	}
	out := make([]string, len(incoming))
	for i, entry := range incoming {
		name, value, ok := strings.Cut(entry, "=")
		if ok && value == Marker {
			out[i] = name + "=" + values[name]
			continue
		}
		out[i] = entry
	}
	return out
}
