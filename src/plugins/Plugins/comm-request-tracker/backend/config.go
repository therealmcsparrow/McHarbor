// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const (
	defaultCFName        = "McHarbor Dedup Key"
	defaultResolveStatus = "resolved"
	maxReopenMinutes     = 1440
)

// safeName is the only shape a queue or custom-field name may take, because both are spliced
// into TicketSQL; a quote or brace can never reach the query through configuration.
var safeName = regexp.MustCompile(`^[\p{L}\p{N} _.\-]{1,200}$`)

// config is one channel's settings, validated once per call so every vendor request is built
// from values that already passed the TicketSQL and SSRF checks.
type config struct {
	base          string
	token         string
	queue         string
	cfName        string
	resolveStatus string
	requestor     string
	owner         string
	reopenMinutes int
	statuses      map[string]incident.RemoteState
	priorities    map[incident.Severity]int
	webhookSecret string
}

// defaultStatuses is RT's default lifecycle; status_map entries extend or override it for
// queues with a custom lifecycle.
var defaultStatuses = map[string]incident.RemoteState{
	"new":      incident.RemoteOpen,
	"open":     incident.RemoteAcknowledged,
	"stalled":  incident.RemoteAcknowledged,
	"resolved": incident.RemoteResolved,
	"rejected": incident.RemoteCancelled,
	"deleted":  incident.RemoteCancelled,
}

// defaultPriorities is numeric because REST2 accepts a numeric Priority on every RT version,
// whereas named priorities exist only where %PriorityAsString is configured.
var defaultPriorities = map[incident.Severity]int{
	incident.SeverityCritical: 90,
	incident.SeverityWarning:  50,
	incident.SeverityInfo:     20,
}

func operatorf(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{incident.ErrOperatorMessage}, args...)...)
}

// parseConfig validates every field the plugin uses; its errors are operator messages that name
// the field and never echo the token.
func parseConfig(raw map[string]string) (*config, error) {
	base, err := baseURL(raw["instance_url"])
	if err != nil {
		return nil, err
	}
	c := &config{
		base:          base,
		token:         strings.TrimSpace(raw["auth_token"]),
		queue:         strings.TrimSpace(raw["queue"]),
		cfName:        strings.TrimSpace(raw["dedup_cf_name"]),
		resolveStatus: strings.TrimSpace(raw["resolve_status"]),
		requestor:     strings.TrimSpace(raw["requestor"]),
		owner:         strings.TrimSpace(raw["default_assignment_group"]),
		webhookSecret: raw["_webhook_secret"],
	}
	if c.token == "" {
		return nil, operatorf("auth_token is required")
	}
	if strings.IndexFunc(c.token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return nil, operatorf("auth_token must not contain spaces or control characters")
	}
	if !safeName.MatchString(c.queue) {
		return nil, operatorf("queue must be 1-200 letters, digits, spaces, dots, dashes or underscores")
	}
	if c.cfName == "" {
		c.cfName = defaultCFName
	}
	if !safeName.MatchString(c.cfName) {
		return nil, operatorf("dedup_cf_name must be 1-200 letters, digits, spaces, dots, dashes or underscores")
	}
	if c.resolveStatus == "" {
		c.resolveStatus = defaultResolveStatus
	}
	if !safeName.MatchString(c.resolveStatus) || len(c.resolveStatus) > 64 {
		return nil, operatorf("resolve_status must be an RT status name")
	}
	if c.requestor != "" && !validEmail(c.requestor) {
		return nil, operatorf("requestor must be a single e-mail address")
	}
	if c.owner != "" && (len(c.owner) > 200 || strings.IndexFunc(c.owner, unicode.IsControl) >= 0) {
		return nil, operatorf("default_assignment_group must be an RT user name")
	}
	if v := strings.TrimSpace(raw["reopen_window_minutes"]); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > maxReopenMinutes {
			return nil, operatorf("reopen_window_minutes must be between 0 and %d", maxReopenMinutes)
		}
		c.reopenMinutes = n
	}
	if c.statuses, err = statusTable(raw["status_map"]); err != nil {
		return nil, err
	}
	if c.priorities, err = priorityTable(raw["severity_map"]); err != nil {
		return nil, err
	}
	return c, nil
}

// baseURL accepts only an absolute https:// URL without credentials, query or fragment, so the
// auth token is never sent in clear text or to a host smuggled in through userinfo.
func baseURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	raw = strings.TrimSuffix(raw, "/REST/2.0")
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" {
		return "", operatorf("instance_url must be an absolute https:// URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", operatorf("instance_url must not carry credentials, a query or a fragment")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func validEmail(s string) bool {
	if len(s) > 254 || strings.ContainsAny(s, " \t\r\n,;<>\"'") {
		return false
	}
	at := strings.LastIndexByte(s, '@')
	return at > 0 && at < len(s)-1 && strings.Contains(s[at+1:], ".")
}

// statusTable merges status_map over the default lifecycle and refuses values that are not one
// of the five remote states, so a typo cannot silently drop every update for a status.
func statusTable(raw string) (map[string]incident.RemoteState, error) {
	out := make(map[string]incident.RemoteState, len(defaultStatuses))
	for k, v := range defaultStatuses {
		out[k] = v
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	var override map[string]string
	if err := json.Unmarshal([]byte(raw), &override); err != nil {
		return nil, operatorf("status_map is not a JSON object of strings")
	}
	for k, v := range override {
		st := incident.RemoteState(strings.ToLower(strings.TrimSpace(v)))
		switch st {
		case incident.RemoteOpen, incident.RemoteAcknowledged, incident.RemoteResolved, incident.RemoteClosed, incident.RemoteCancelled:
		default:
			return nil, operatorf("status_map value for %q must be open, acknowledged, resolved, closed or cancelled", sanitize(k))
		}
		out[strings.ToLower(strings.TrimSpace(k))] = st
	}
	return out, nil
}

// priorityTable merges severity_map over the default priorities; values may be JSON numbers
// or numeric strings, and keys other than the three severities are refused.
func priorityTable(raw string) (map[incident.Severity]int, error) {
	out := make(map[incident.Severity]int, len(defaultPriorities))
	for k, v := range defaultPriorities {
		out[k] = v
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	var override map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &override); err != nil {
		return nil, operatorf("severity_map is not a JSON object")
	}
	for k, v := range override {
		sev := incident.Severity(k)
		if _, ok := defaultPriorities[sev]; !ok {
			return nil, operatorf("severity_map key %q is not critical, warning or info", sanitize(k))
		}
		s := strings.Trim(strings.TrimSpace(string(v)), `"`)
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 255 {
			return nil, operatorf("severity_map value for %q must be an RT priority between 0 and 255", k)
		}
		out[sev] = n
	}
	return out, nil
}

// state maps an RT status through the channel's table; an unknown status reports ok=false so
// the caller drops it instead of guessing.
func (c *config) state(status string) (incident.RemoteState, bool) {
	st, ok := c.statuses[strings.ToLower(strings.TrimSpace(status))]
	return st, ok
}

// priority reads the table for sev, falling back to the warning priority for a severity the
// host has not validated, so a create is never refused over the priority alone.
func (c *config) priority(sev incident.Severity) int {
	if p, ok := c.priorities[sev]; ok {
		return p
	}
	return c.priorities[incident.SeverityWarning]
}

// ticketURL deep-links into RT's web UI, which lives beside REST2 under the same web root.
func (c *config) ticketURL(id string) string {
	return c.base + "/Ticket/Display.html?id=" + id
}

// hashKey always hashes: the dedup value lands in TicketSQL and in a CF, so a fixed-length hex
// string keeps both safe whatever characters the host's dedup key carries.
func hashKey(key string, max int) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	if max > 0 && max < len(h) {
		return h[:max]
	}
	return h
}

// sqlQuote escapes a value for a single-quoted TicketSQL literal; every caller already passed
// safeName or is hex, so this is defence in depth rather than the only guard.
func sqlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return "'" + strings.ReplaceAll(s, `'`, `\'`) + "'"
}

// ticketID accepts only a positive decimal RT id, because it is spliced into REST2 paths and
// TicketSQL; anything else is a host or storage bug, not a vendor failure.
func ticketID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 18 {
		return "", operatorf("ticket id %q is not an RT ticket number", sanitize(s))
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", operatorf("ticket id %q is not an RT ticket number", sanitize(s))
		}
	}
	if strings.TrimLeft(s, "0") == "" {
		return "", operatorf("ticket id %q is not an RT ticket number", sanitize(s))
	}
	return s, nil
}
