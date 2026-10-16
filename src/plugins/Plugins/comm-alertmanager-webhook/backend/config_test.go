// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestParseConfigRejects(t *testing.T) {
	const token = "tok-SECRET-9"
	cases := map[string]map[string]string{
		"empty":             {},
		"file scheme":       {"webhook_url": "file:///etc/passwd"},
		"plain http":        {"webhook_url": "http://receiver.example/" + token},
		"userinfo":          {"webhook_url": "https://user:" + token + "@receiver.example/hook"},
		"unknown profile":   {"webhook_url": "https://receiver.example/", "profile": "opsgenie"},
		"unknown auth":      {"webhook_url": "https://receiver.example/", "auth_mode": "digest"},
		"bearer no token":   {"webhook_url": "https://receiver.example/", "auth_mode": "bearer"},
		"basic no user":     {"webhook_url": "https://receiver.example/", "auth_mode": "basic"},
		"grafana no slash":  {"webhook_url": "https://oncall.example/integrations/v1/alertmanager/" + token, "profile": "grafana_irm"},
		"severity typo":     {"webhook_url": "https://receiver.example/", "severity_map": `{"crit":"P1"}`},
		"severity empty":    {"webhook_url": "https://receiver.example/", "severity_map": `{"critical":""}`},
		"severity not json": {"webhook_url": "https://receiver.example/", "severity_map": `P1`},
	}
	for name, m := range cases {
		_, err := parseConfig(m)
		if !errors.Is(err, incident.ErrOperatorMessage) {
			t.Errorf("%s: accepted or unmarked: %v", name, err)
			continue
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("%s: secret leaked: %v", name, err)
		}
	}
}

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := parseConfig(map[string]string{"webhook_url": " https://oncall.example/integrations/v1/alertmanager/abc/ ", "profile": "grafana_irm"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Origin != "https://oncall.example" || cfg.Receiver != "mcharbor" || cfg.AuthMode != "none" || cfg.SeverityMap[incident.SeverityWarning] != "warning" {
		t.Fatalf("defaults %+v", cfg)
	}
	keep, err := parseConfig(map[string]string{"webhook_url": "http://keep.lan:8080/alerts/event/prometheus", "profile": "keep", "allow_http": "true"})
	if err != nil || keep.AuthMode != "basic" || keep.BasicUser != "api_key" {
		t.Fatalf("keep defaults %+v %v", keep, err)
	}
	explicit, err := parseConfig(map[string]string{"webhook_url": "https://keep.example/alerts/event/prometheus", "profile": "keep", "auth_mode": "none"})
	if err != nil || explicit.AuthMode != "none" {
		t.Fatalf("explicit auth_mode ignored %+v %v", explicit, err)
	}
}
