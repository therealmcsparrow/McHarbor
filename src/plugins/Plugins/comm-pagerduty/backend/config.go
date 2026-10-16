// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// pdConfig is a channel config after validation; it is rebuilt per call because the host
// hands the plugin a fresh decrypted map every time.
type pdConfig struct {
	Region        string
	RoutingKey    string
	RESTKey       string
	WebhookSecret string
	SeverityMap   map[incident.Severity]string
}

var errConfig = errors.New("comm-pagerduty: invalid configuration")

var validPDSeverity = map[string]bool{"critical": true, "error": true, "warning": true, "info": true}

// defaultSeverity is the one table a reviewer reads to know how McHarbor's levels land in
// PagerDuty; severity_map overrides entries and never replaces the table.
var defaultSeverity = map[incident.Severity]string{
	incident.SeverityCritical: "critical",
	incident.SeverityWarning:  "warning",
	incident.SeverityInfo:     "info",
}

// parseConfig validates everything that can be checked offline, so a bad region, key length
// or severity map fails at Test time instead of as a parked ticket later.
func parseConfig(cfg map[string]string) (pdConfig, error) {
	out := pdConfig{
		Region:        strings.ToLower(strings.TrimSpace(cfg["region"])),
		RoutingKey:    strings.TrimSpace(cfg["routing_key"]),
		RESTKey:       strings.TrimSpace(cfg["rest_api_key"]),
		WebhookSecret: strings.TrimSpace(cfg["webhook_signing_secret"]),
		SeverityMap:   make(map[incident.Severity]string, len(defaultSeverity)),
	}
	for k, v := range defaultSeverity {
		out.SeverityMap[k] = v
	}
	if out.Region == "" {
		out.Region = "us"
	}
	if out.Region != "us" && out.Region != "eu" {
		return pdConfig{}, fmt.Errorf("%w: %w: region must be us or eu", errConfig, incident.ErrOperatorMessage)
	}
	if len(out.RoutingKey) != 32 {
		return pdConfig{}, fmt.Errorf("%w: %w: routing key must be 32 characters", errConfig, incident.ErrOperatorMessage)
	}
	if raw := strings.TrimSpace(cfg["severity_map"]); raw != "" {
		var override map[string]string
		if err := json.Unmarshal([]byte(raw), &override); err != nil {
			return pdConfig{}, fmt.Errorf("%w: %w: severity_map is not a JSON object", errConfig, incident.ErrOperatorMessage)
		}
		for k, v := range override {
			sev := incident.Severity(k)
			if _, ok := defaultSeverity[sev]; !ok {
				return pdConfig{}, fmt.Errorf("%w: %w: severity_map key %q is not critical, warning or info", errConfig, incident.ErrOperatorMessage, k)
			}
			if !validPDSeverity[v] {
				return pdConfig{}, fmt.Errorf("%w: %w: severity_map value %q is not critical, error, warning or info", errConfig, incident.ErrOperatorMessage, v)
			}
			out.SeverityMap[sev] = v
		}
	}
	return out, nil
}
