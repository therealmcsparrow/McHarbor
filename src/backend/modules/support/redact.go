// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"encoding/base64"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// Redacted is the placeholder written wherever a value was withheld, so a reader can tell
// "set but hidden" apart from "not set".
const Redacted = "[redacted]"

// safeEnvKeys are the only environment variables whose values may leave the host. Everything
// else McHarbor reads is listed by name only, because an allow-list fails closed when a new
// secret-bearing variable is added and a deny-list does not.
var safeEnvKeys = map[string]bool{
	"PORT": true, "HOST": true, "DATA_DIR": true, "DATABASE_PATH": true,
	"MCHARBOR_DB_DRIVER": true, "MCHARBOR_NODE_ID": true, "MCHARBOR_HA_MODE": true,
	"MCHARBOR_PLUGIN_STORE": true, "MCHARBOR_ADVERTISE_URL": true, "MCHARBOR_INSTALL_KIND": true,
	"MCHARBOR_TRUSTED_PROXY_CIDRS": true, "MCHARBOR_RECONCILE_INTERVAL": true,
	"MCHARBOR_VERSION": true, "MCHARBOR_BUILD": true, "MCHARBOR_PUSH_CONTACT": true,
	"DOCKER_TLS_VERIFY": true, "AUTH_DISABLE": true, "FORCE_SECURE_COOKIES": true,
	"TLS_ENABLED": true, "TLS_FORCE_HTTPS": true, "TLS_MIN_VERSION": true, "HSTS_MAX_AGE": true,
	"HTTP_REDIRECT_ADDR": true, "HTTP_LISTEN_ADDR": true, "TRANSFER_PORT": true,
	"FORCE_AGENT_TLS": true, "SELF_SIGNED_SAN_EXTRA": true, "LOG_LEVEL": true, "LOG_JSON": true,
	"OTEL_EXPORTER_OTLP_ENDPOINT": true, "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": true,
	"ALLOWED_ORIGINS": true, "APPSTORE_CATALOG_URL": true, "APPSTORE_SYNC_CRON": true,
	"TZ": true,
}

// knownEnvKeys are the variables McHarbor reads that are not safe to print; they are listed
// as set or unset so a support engineer can still see the shape of the configuration.
var knownEnvKeys = []string{
	"MCHARBOR_SECRET", "ENCRYPTION_KEY", "MCHARBOR_DB_DSN", "BACKUP_ENCRYPTION_KEY_FILE",
	"DOCKER_HOST", "DOCKER_CERT_PATH", "MCHARBOR_PLUGIN_CATALOG_DIRS",
	"MCHARBOR_PLUGIN_CATALOG_ROOT", "MCHARBOR_THEME_CATALOG_ROOT", "MCHARBOR_RESET_KEY_CHECK",
}

// EnvEntry is one environment variable as it appears in a support bundle.
type EnvEntry struct {
	Name  string `json:"name"`
	Set   bool   `json:"set"`
	Value string `json:"value,omitempty"`
}

// envSafe reports whether name's value may be printed at all.
func envSafe(name string) bool {
	if !safeEnvKeys[name] {
		return false
	}
	// The name classifier overrides the allow-list, so adding a secret to safeEnvKeys by
	// mistake still prints nothing.
	return !envredact.SensitiveName(name)
}

// configuredLookup answers "set" for a variable the loaded configuration holds even though
// the process environment does not: MCHARBOR_SECRET is unset after load and a _FILE
// variant never enters the environment, and neither means the secret is missing.
func configuredLookup(lookup func(string) (string, bool), configured map[string]bool) func(string) (string, bool) {
	return func(name string) (string, bool) {
		if value, ok := lookup(name); ok && value != "" {
			return value, ok
		}
		if configured[name] {
			return Redacted, true
		}
		return lookup(name)
	}
}

// RedactedEnv lists every variable McHarbor reads plus any MCHARBOR_* extra, printing a value
// only for allow-listed names and passing even those through the scrubber.
func RedactedEnv(lookup func(string) (string, bool), environ []string, s *Scrubber) []EnvEntry {
	names := map[string]bool{}
	for k := range safeEnvKeys {
		names[k] = true
	}
	for _, k := range knownEnvKeys {
		names[k] = true
	}
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "MCHARBOR_") || strings.HasPrefix(name, "OTEL_") {
			names[name] = true
		}
	}
	out := make([]EnvEntry, 0, len(names))
	for name := range names {
		value, set := lookup(name)
		e := EnvEntry{Name: name, Set: set && value != ""}
		if e.Set {
			if envSafe(name) {
				e.Value = s.Scrub(value)
			} else {
				e.Value = Redacted
			}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

var (
	pemBlock      = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]+-----.*?-----END [A-Z0-9 ]+-----`)
	urlUserinfo   = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/\s:@]*(:[^/\s@]*)?@`)
	mysqlDSN      = regexp.MustCompile(`([A-Za-z0-9_.-]+):[^@\s/]+@(tcp|unix)\(`)
	bearer        = envredact.BearerPattern
	ciphertext    = regexp.MustCompile(`enc:v1:[A-Za-z0-9+/=]+`)
	keyValue      = envredact.KeyValuePattern
	queryCredPart = envredact.QueryCredentialPattern
	tokenShape    = envredact.TokenPattern
)

// Scrubber removes credentials from free text. Patterns catch the shapes secrets usually
// take; the known-value list catches the ones that look like nothing in particular.
type Scrubber struct {
	known []string
}

// NewScrubber returns a Scrubber that also replaces every literal value in known, including
// its base64 form, so a key logged verbatim by some other package is still withheld.
func NewScrubber(known ...string) *Scrubber {
	s := &Scrubber{}
	seen := map[string]bool{}
	add := func(v string) {
		if len(v) < 6 || seen[v] {
			return
		}
		seen[v] = true
		s.known = append(s.known, v)
	}
	for _, v := range known {
		v = strings.TrimSpace(v)
		add(v)
		add(base64.StdEncoding.EncodeToString([]byte(v)))
		add(url.QueryEscape(v))
		if u, err := url.Parse(v); err == nil && u.User != nil {
			if pw, ok := u.User.Password(); ok {
				add(pw)
			}
		}
		if m := mysqlPassword(v); m != "" {
			add(m)
		}
	}
	sort.Slice(s.known, func(i, j int) bool { return len(s.known[i]) > len(s.known[j]) })
	return s
}

func mysqlPassword(dsn string) string {
	at := strings.Index(dsn, "@")
	if at < 0 || strings.Contains(dsn[:at], "://") {
		return ""
	}
	_, pw, ok := strings.Cut(dsn[:at], ":")
	if !ok {
		return ""
	}
	return pw
}

// Scrub returns text with every recognised credential replaced by Redacted.
func (s *Scrubber) Scrub(text string) string {
	if text == "" {
		return text
	}
	for _, k := range s.known {
		text = strings.ReplaceAll(text, k, Redacted)
	}
	text = pemBlock.ReplaceAllString(text, Redacted)
	text = urlUserinfo.ReplaceAllString(text, "${1}"+Redacted+"@")
	text = mysqlDSN.ReplaceAllString(text, "${1}:"+Redacted+"@${2}(")
	text = bearer.ReplaceAllString(text, "${1} "+Redacted)
	text = keyValue.ReplaceAllString(text, "${1}"+Redacted)
	text = queryCredPart.ReplaceAllString(text, "${1}"+Redacted)
	text = tokenShape.ReplaceAllString(text, Redacted)
	text = ciphertext.ReplaceAllString(text, Redacted)
	return text
}

// knownSecretsFromEnv collects every value of a sensitive-looking variable in the process
// environment so the scrubber can withhold it wherever it turns up.
func knownSecretsFromEnv(environ []string) []string {
	var out []string
	for _, kv := range environ {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || value == "" {
			continue
		}
		if envredact.SensitiveName(name) {
			out = append(out, value)
		}
	}
	return out
}

// processEnv is the real environment, swapped in tests.
var processEnv = func() ([]string, func(string) (string, bool)) { return os.Environ(), os.LookupEnv }
