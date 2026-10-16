// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// config is the validated channel configuration; every lifecycle call parses it afresh because
// the host passes the raw map and an operator may have edited the channel in between.
type config struct {
	base          string
	token         string
	group         string
	customer      string
	onBehalfOf    string
	owner         string
	resolveState  string
	suppress      bool
	pollInterval  int
	reopenWindow  int
	severity      map[incident.Severity]string
	status        map[string]incident.RemoteState
	webhookSecret string
}

// parseConfig validates every field a vendor call relies on and returns operator messages,
// so a misconfigured channel is parked with a readable reason instead of retried.
func parseConfig(raw map[string]string) (config, error) {
	base, err := baseURL(raw)
	if err != nil {
		return config{}, err
	}
	cfg := config{
		base:          base,
		token:         strings.TrimSpace(raw["access_token"]),
		group:         strings.TrimSpace(raw["group"]),
		customer:      strings.TrimSpace(raw["customer"]),
		onBehalfOf:    strings.TrimSpace(raw["on_behalf_of"]),
		owner:         strings.TrimSpace(raw["default_assignment_group"]),
		resolveState:  strings.TrimSpace(raw["resolve_state"]),
		suppress:      raw["suppress_notifications"] == "true",
		webhookSecret: raw["_webhook_secret"],
	}
	if cfg.resolveState == "" {
		cfg.resolveState = "closed"
	}
	for _, req := range []struct{ key, val string }{
		{"access_token", cfg.token}, {"group", cfg.group}, {"customer", cfg.customer},
	} {
		if req.val == "" {
			return config{}, fmt.Errorf("%w: %s is required", incident.ErrOperatorMessage, req.key)
		}
	}
	if strings.ContainsAny(cfg.token+cfg.onBehalfOf, "\r\n") {
		return config{}, fmt.Errorf("%w: access_token and on_behalf_of must be a single line", incident.ErrOperatorMessage)
	}
	if cfg.pollInterval, err = intField(raw, "poll_interval_minutes", 5, 0, 60); err != nil {
		return config{}, err
	}
	if cfg.reopenWindow, err = intField(raw, "reopen_window_minutes", 0, 0, 1440); err != nil {
		return config{}, err
	}
	if cfg.severity, err = severityTable(raw["severity_map"]); err != nil {
		return config{}, err
	}
	if cfg.status, err = statusTable(raw["status_map"]); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func intField(raw map[string]string, key string, def, lo, hi int) (int, error) {
	v := strings.TrimSpace(raw[key])
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("%w: %s must be a whole number from %d to %d", incident.ErrOperatorMessage, key, lo, hi)
	}
	return n, nil
}

// baseURL accepts only an absolute https URL without credentials, query or fragment, because
// the token travels in a header and SHA-1 webhook signing is only acceptable over TLS.
func baseURL(raw map[string]string) (string, error) {
	s := strings.TrimRight(strings.TrimSpace(raw["instance_url"]), "/")
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" {
		return "", fmt.Errorf("%w: the Zammad URL must be an absolute https:// URL", incident.ErrOperatorMessage)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("%w: the Zammad URL must not carry credentials, a query or a fragment", incident.ErrOperatorMessage)
	}
	return s, nil
}
