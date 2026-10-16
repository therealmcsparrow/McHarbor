// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// itopConfig is the channel config validated once per call; every value that later reaches
// OQL is either an allow-listed word, an integer, or goes through oqlQuote.
type itopConfig struct {
	BaseURL        string
	AuthMode       string
	Token          string
	User           string
	Password       string
	Class          string
	OrgName        string
	CallerEmail    string
	Origin         string
	TeamID         int
	AgentID        int
	ServiceID      int
	ResolutionCode string
	CloseOnResolve bool
	ReopenWindow   int
	CreateTeamID   int
	Severity       map[incident.Severity]severityFields
	WebhookSecret  string
}

// severityFields is the impact/urgency pair one McHarbor severity lands on; both are strings
// because iTop enums are strings, and priority is computed by iTop from the two.
type severityFields struct {
	Impact  string
	Urgency string
}

// secrets lists the credential values sanitize must redact from any text an operator sees.
func (c itopConfig) secrets() []string {
	return []string{c.Token, c.Password, c.WebhookSecret}
}

// parseConfig validates the raw channel map; failures are operator messages because they name
// a field the operator can fix, never a credential.
func parseConfig(raw map[string]string) (itopConfig, error) {
	get := func(k string) string { return strings.TrimSpace(raw[k]) }
	var cfg itopConfig
	base, err := baseURL(get("instance_url"))
	if err != nil {
		return cfg, err
	}
	cfg.BaseURL = base
	cfg.AuthMode = strings.ToLower(orDefault(get("auth_mode"), "token"))
	switch cfg.AuthMode {
	case "token":
		cfg.Token = get("auth_token")
		if cfg.Token == "" {
			return cfg, operatorError("the application token is required for token authentication")
		}
	case "password":
		cfg.User, cfg.Password = get("auth_user"), raw["auth_pwd"]
		if cfg.User == "" || cfg.Password == "" {
			return cfg, operatorError("the REST user login and password are required for password authentication")
		}
	default:
		return cfg, operatorError("authentication must be token or password")
	}
	cfg.Class = orDefault(get("ticket_class"), "Incident")
	if cfg.Class != "Incident" && cfg.Class != "UserRequest" {
		return cfg, operatorError("ticket class must be Incident or UserRequest")
	}
	cfg.OrgName = get("org_name")
	if cfg.OrgName == "" {
		return cfg, operatorError("the organization name is required")
	}
	cfg.CallerEmail = get("caller_email")
	cfg.Origin = orDefault(get("origin"), "monitoring")
	cfg.ResolutionCode = orDefault(get("resolution_code"), "other")
	ints := []struct {
		key      string
		dst      *int
		min, max int
		def      string
	}{
		{"team_id", &cfg.TeamID, 1, 1 << 30, ""},
		{"agent_id", &cfg.AgentID, 1, 1 << 30, ""},
		{"service_id", &cfg.ServiceID, 1, 1 << 30, ""},
		{"reopen_window_minutes", &cfg.ReopenWindow, 0, 1440, "0"},
		{"default_assignment_group", &cfg.CreateTeamID, 0, 1 << 30, "0"},
	}
	for _, f := range ints {
		n, err := strconv.Atoi(orDefault(get(f.key), f.def))
		if err != nil || n < f.min || n > f.max {
			return cfg, operatorError("%s must be a whole number from %d to %d", f.key, f.min, f.max)
		}
		*f.dst = n
	}
	cfg.CloseOnResolve = get("close_on_resolve") == "true"
	if cfg.Severity, err = severityTable(get("severity_map")); err != nil {
		return cfg, err
	}
	cfg.WebhookSecret = raw["_webhook_secret"]
	return cfg, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// baseURL accepts only an absolute https URL without credentials, query or fragment, so a
// config value cannot smuggle a different host or send the form body over cleartext.
func baseURL(raw string) (string, error) {
	raw = strings.TrimRight(raw, "/")
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", operatorError("the iTop URL must be an absolute https:// URL without credentials or query")
	}
	return raw, nil
}

// defaultSeverity is the one table a reviewer reads to know how McHarbor's levels land in
// iTop; a larger impact number is a smaller impact (1 department, 3 person).
var defaultSeverity = map[incident.Severity]severityFields{
	incident.SeverityCritical: {Impact: "1", Urgency: "1"},
	incident.SeverityWarning:  {Impact: "2", Urgency: "2"},
	incident.SeverityInfo:     {Impact: "3", Urgency: "3"},
}

// flexString accepts "1" or 1 in severity_map, because operators write both.
type flexString string

// UnmarshalJSON implements json.Unmarshaler for a JSON string or number.
func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

// severityTable merges severity_map over the default table and refuses unknown severities and
// out-of-range values, so a typo cannot silently fall back to the default or be sent to iTop.
func severityTable(raw string) (map[incident.Severity]severityFields, error) {
	out := make(map[incident.Severity]severityFields, len(defaultSeverity))
	for k, v := range defaultSeverity {
		out[k] = v
	}
	if raw == "" {
		return out, nil
	}
	var override map[string]struct {
		Impact  flexString `json:"impact"`
		Urgency flexString `json:"urgency"`
	}
	if err := json.Unmarshal([]byte(raw), &override); err != nil {
		return nil, operatorError(`severity_map must be a JSON object like {"critical":{"impact":"1","urgency":"1"}}`)
	}
	for k, v := range override {
		sev := incident.Severity(k)
		cur, ok := out[sev]
		if !ok {
			return nil, operatorError("severity_map key %q is not critical, warning or info", k)
		}
		if v.Impact != "" {
			cur.Impact = string(v.Impact)
		}
		if v.Urgency != "" {
			cur.Urgency = string(v.Urgency)
		}
		impact, okI := inRange(cur.Impact, 1, 3)
		urgency, okU := inRange(cur.Urgency, 1, 4)
		if !okI || !okU {
			return nil, operatorError("severity_map %s: impact must be 1 to 3 and urgency 1 to 4", k)
		}
		out[sev] = severityFields{Impact: impact, Urgency: urgency}
	}
	return out, nil
}

// inRange returns s in canonical decimal form, so "01" is sent to iTop as the enum value "1".
func inRange(s string, lo, hi int) (string, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < lo || n > hi {
		return "", false
	}
	return strconv.Itoa(n), true
}
