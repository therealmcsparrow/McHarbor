// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// amConfig is the validated channel config. URL is a secret because receivers embed their
// token in it, so only Origin (scheme://host) may appear in any error.
type amConfig struct {
	URL            string
	Origin         string
	Profile        string
	Receiver       string
	AuthMode       string
	Bearer         string
	BasicUser      string
	BasicPass      string
	HMACSecret     string
	AllowHTTP      bool
	TestSendsAlert bool
	SeverityMap    map[incident.Severity]string
}

// defaultSeverity carries the Prometheus convention values that receivers' label rules expect;
// severity_map renames entries for receivers configured on other values.
var defaultSeverity = map[incident.Severity]string{
	incident.SeverityCritical: "critical",
	incident.SeverityWarning:  "warning",
	incident.SeverityInfo:     "info",
}

var knownProfiles = map[string]bool{"generic": true, "grafana_irm": true, "squadcast": true, "zenduty": true, "keep": true}

func operatorErr(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{incident.ErrOperatorMessage}, args...)...)
}

// parseConfig validates the channel on every call, so a bad edit fails the first delivery with
// an operator-readable reason instead of a request the receiver silently drops.
func parseConfig(m map[string]string) (amConfig, error) {
	cfg := amConfig{
		Profile:        strings.TrimSpace(m["profile"]),
		Receiver:       strings.TrimSpace(m["receiver_name"]),
		AuthMode:       strings.TrimSpace(m["auth_mode"]),
		Bearer:         m["bearer_token"],
		BasicUser:      strings.TrimSpace(m["basic_username"]),
		BasicPass:      m["basic_password"],
		HMACSecret:     m["hmac_secret"],
		AllowHTTP:      m["allow_http"] == "true",
		TestSendsAlert: m["test_sends_alert"] == "true",
	}
	if cfg.Profile == "" {
		cfg.Profile = "generic"
	}
	if !knownProfiles[cfg.Profile] {
		return amConfig{}, operatorErr("unknown receiver profile %q", cfg.Profile)
	}
	if cfg.Receiver == "" {
		cfg.Receiver = "mcharbor"
	}
	if cfg.AuthMode == "" {
		cfg.AuthMode = "none"
		if cfg.Profile == "keep" {
			cfg.AuthMode = "basic"
		}
	}
	if cfg.Profile == "keep" && cfg.BasicUser == "" {
		cfg.BasicUser = "api_key"
	}
	switch cfg.AuthMode {
	case "none":
	case "bearer":
		if cfg.Bearer == "" {
			return amConfig{}, operatorErr("bearer authentication needs a bearer token")
		}
	case "basic":
		if cfg.BasicUser == "" {
			return amConfig{}, operatorErr("basic authentication needs a username")
		}
	default:
		return amConfig{}, operatorErr("unknown authentication mode %q", cfg.AuthMode)
	}

	raw := strings.TrimSpace(m["webhook_url"])
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return amConfig{}, operatorErr("receiver URL must be an absolute https:// URL")
	}
	cfg.Origin = u.Scheme + "://" + u.Host
	if u.User != nil {
		return amConfig{}, operatorErr("receiver URL for %s carries credentials; use the authentication setting instead", cfg.Origin)
	}
	if u.Scheme == "http" && !cfg.AllowHTTP {
		return amConfig{}, operatorErr("receiver URL for %s uses http://; turn on plain http to allow it", cfg.Origin)
	}
	if cfg.Profile == "grafana_irm" && !strings.HasSuffix(u.Path, "/") {
		return amConfig{}, operatorErr("Grafana IRM integration URLs end in /")
	}
	cfg.URL = raw

	sev, err := severityTable(m["severity_map"])
	if err != nil {
		return amConfig{}, err
	}
	cfg.SeverityMap = sev
	return cfg, nil
}

// severityTable merges the operator's severity_map over the default and refuses unknown keys
// and empty values, because Squadcast rejects an alert whose severity label is empty.
func severityTable(raw string) (map[incident.Severity]string, error) {
	out := make(map[incident.Severity]string, len(defaultSeverity))
	for k, v := range defaultSeverity {
		out[k] = v
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	var override map[string]string
	if err := json.Unmarshal([]byte(raw), &override); err != nil {
		return nil, operatorErr("severity_map is not a JSON object of strings")
	}
	for k, v := range override {
		sev := incident.Severity(k)
		if _, ok := defaultSeverity[sev]; !ok {
			return nil, operatorErr("severity_map key %q is not critical, warning or info", k)
		}
		if strings.TrimSpace(v) == "" {
			return nil, operatorErr("severity_map value for %q is empty", k)
		}
		out[sev] = strings.TrimSpace(v)
	}
	return out, nil
}
