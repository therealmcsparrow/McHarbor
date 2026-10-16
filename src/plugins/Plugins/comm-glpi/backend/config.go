// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const (
	apiV1 = "v1"
	apiV2 = "v2"
)

// v2PathPattern keeps v2_path a bare version segment, so an operator value can never steer
// requests to another path on the instance.
var v2PathPattern = regexp.MustCompile(`^v2\.[0-9]{1,2}$`)

// sevFields is one row of the severity table: GLPI's urgency and impact (1-5) and priority
// (1-6, where 6 is Major).
type sevFields struct {
	Urgency  int `json:"urgency"`
	Impact   int `json:"impact"`
	Priority int `json:"priority"`
}

// defaultSeverity is the one table a reviewer reads to know how McHarbor's three levels land
// in GLPI; priority is explicit because the instance's matrix may ignore urgency x impact.
var defaultSeverity = map[incident.Severity]sevFields{
	incident.SeverityCritical: {Urgency: 5, Impact: 5, Priority: 6},
	incident.SeverityWarning:  {Urgency: 4, Impact: 3, Priority: 4},
	incident.SeverityInfo:     {Urgency: 2, Impact: 2, Priority: 2},
}

// glpiConfig is the validated channel config; every vendor call starts from one so no call
// path reads a raw map value it has not checked.
type glpiConfig struct {
	BaseURL        string
	API            string
	AppToken       string
	UserToken      string
	V2Path         string
	ClientID       string
	ClientSecret   string
	Username       string
	Password       string
	EntityID       int
	CategoryID     int
	GroupID        int
	CloseOnResolve bool
	Severity       map[incident.Severity]sevFields
}

// secrets lists every credential a vendor message could echo back, for redaction.
func (c glpiConfig) secrets() []string {
	return []string{c.AppToken, c.UserToken, c.ClientSecret, c.Password}
}

func operatorError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", incident.ErrOperatorMessage, fmt.Sprintf(format, args...))
}

// baseURL validates instance_url: https only, no credentials, query or fragment, and a
// trailing /apirest.php or /api.php is tolerated because operators paste the API URL.
func baseURL(cfg map[string]string) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(cfg["instance_url"]), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", operatorError("the GLPI URL must be an absolute https:// URL without credentials or query")
	}
	for _, suffix := range []string{"/apirest.php", "/api.php"} {
		raw = strings.TrimSuffix(raw, suffix)
	}
	return strings.TrimRight(raw, "/"), nil
}

func intField(cfg map[string]string, key string) (int, error) {
	v := strings.TrimSpace(cfg[key])
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, operatorError("%s must be a whole number of 0 or more", key)
	}
	return n, nil
}

// severityTable merges the operator's severity_map JSON over the default table and refuses
// unknown keys or out-of-range values, so a typo cannot silently fall back to the default.
func severityTable(raw string) (map[incident.Severity]sevFields, error) {
	out := make(map[incident.Severity]sevFields, len(defaultSeverity))
	for k, v := range defaultSeverity {
		out[k] = v
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	var override map[string]sevFields
	if err := json.Unmarshal([]byte(raw), &override); err != nil {
		return nil, operatorError("severity_map is not a JSON object of {urgency, impact, priority}")
	}
	for k, v := range override {
		sev := incident.Severity(k)
		if _, ok := defaultSeverity[sev]; !ok {
			return nil, operatorError("severity_map key %q is not critical, warning or info", k)
		}
		if v.Urgency < 1 || v.Urgency > 5 || v.Impact < 1 || v.Impact > 5 || v.Priority < 1 || v.Priority > 6 {
			return nil, operatorError("severity_map %q needs urgency and impact 1-5 and priority 1-6", k)
		}
		out[sev] = v
	}
	return out, nil
}

// parseConfig validates the whole channel config once per call and demands only the
// credentials of the selected API, so a v1 channel never needs OAuth fields.
func parseConfig(cfg map[string]string) (glpiConfig, error) {
	base, err := baseURL(cfg)
	if err != nil {
		return glpiConfig{}, err
	}
	c := glpiConfig{
		BaseURL:        base,
		API:            strings.ToLower(strings.TrimSpace(cfg["api_version"])),
		AppToken:       strings.TrimSpace(cfg["app_token"]),
		UserToken:      strings.TrimSpace(cfg["user_token"]),
		V2Path:         strings.TrimSpace(cfg["v2_path"]),
		ClientID:       strings.TrimSpace(cfg["oauth_client_id"]),
		ClientSecret:   cfg["oauth_client_secret"],
		Username:       strings.TrimSpace(cfg["oauth_username"]),
		Password:       cfg["oauth_password"],
		CloseOnResolve: cfg["close_on_resolve"] == "true",
	}
	if c.API == "" {
		c.API = apiV1
	}
	if c.V2Path == "" {
		c.V2Path = "v2.0"
	}
	switch c.API {
	case apiV1:
		if c.UserToken == "" {
			return glpiConfig{}, operatorError("the user API token is required for API v1")
		}
	case apiV2:
		if !v2PathPattern.MatchString(c.V2Path) {
			return glpiConfig{}, operatorError("the HL API version path must look like v2.0")
		}
		if c.ClientID == "" || c.ClientSecret == "" || c.Username == "" || c.Password == "" {
			return glpiConfig{}, operatorError("API v2 needs the OAuth client ID and secret and the API user login and password")
		}
	default:
		return glpiConfig{}, operatorError("api_version must be v1 or v2")
	}
	if c.EntityID, err = intField(cfg, "entities_id"); err != nil {
		return glpiConfig{}, err
	}
	if c.CategoryID, err = intField(cfg, "itilcategories_id"); err != nil {
		return glpiConfig{}, err
	}
	if c.GroupID, err = intField(cfg, "default_assignment_group"); err != nil {
		return glpiConfig{}, err
	}
	if c.Severity, err = severityTable(cfg["severity_map"]); err != nil {
		return glpiConfig{}, err
	}
	return c, nil
}
