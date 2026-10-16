// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// Test validates the config offline and probes the REST key when one is set; it never sends a
// trigger, because a connection test that pages the on-call engineer is worse than no test.
func (c *Client) Test(ctx context.Context, config map[string]string) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	if cfg.RESTKey == "" {
		return nil
	}
	err = c.doJSON(ctx, http.MethodGet, c.restURL(cfg)+"/abilities", restHeaders(cfg), nil, nil, cfg.RESTKey, cfg.RoutingKey)
	var se *statusError
	if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
		return fmt.Errorf("%w: the REST API key was rejected by the %s service region (HTTP %d); check that the key belongs to an account in that region",
			incident.ErrOperatorMessage, strings.ToUpper(cfg.Region), se.Code)
	}
	return err
}

// Send maps a plain notification to a trigger with a content-derived key, so a notification
// repeated while the first alert is still open does not page a second time.
func (c *Client) Send(ctx context.Context, config map[string]string, msg plugin.NotificationMessage) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(msg.Title + "\x00" + msg.Message))
	sev := incident.SeverityInfo
	switch msg.Level {
	case "error":
		sev = incident.SeverityCritical
	case "warning":
		sev = incident.SeverityWarning
	}
	ev := incident.Event{DedupKey: "mcharbor-notify:" + hex.EncodeToString(sum[:])[:32], State: incident.StateFiring,
		Severity: sev, Title: firstNonEmpty(msg.Title, msg.Message), Message: msg.Message, Source: "mcharbor"}
	return c.enqueue(ctx, cfg, c.triggerEvent(cfg, ev))
}
