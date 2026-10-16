// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

var errForeignRef = fmt.Errorf("%w: link reference was not created by the Alertmanager webhook plugin", incident.ErrOperatorMessage)

// Open sends one firing alert in its own group and returns fingerprint@startsAt, so later calls
// resend the same startsAt and receivers never mistake an update for a new alert.
func (c *Client) Open(ctx context.Context, config map[string]string, ev incident.Event) (incident.Ref, error) {
	cfg, err := parseConfig(config)
	if err != nil {
		return incident.Ref{}, err
	}
	if err := ev.Validate(); err != nil {
		return incident.Ref{}, fmt.Errorf("comm-alertmanager-webhook: %w", err)
	}
	at := ev.OccurredAt
	if at.IsZero() {
		at = c.now()
	}
	fp, startsAt := fingerprint(ev.DedupKey), at.UTC().Truncate(time.Second).Format(time.RFC3339)
	if err := c.send(ctx, cfg, buildPayload(cfg, ev, fp, startsAt, statusFiring, time.Time{})); err != nil {
		return incident.Ref{}, err
	}
	return incident.Ref{ExternalID: fp + "@" + startsAt}, nil
}

// Update re-sends the firing alert unchanged in identity, which receivers already treat as an
// Alertmanager repeat notification.
func (c *Client) Update(ctx context.Context, config map[string]string, ref incident.Ref, ev incident.Event) error {
	return c.lifecycle(ctx, config, ref, ev, statusFiring)
}

// Resolve sends the same alert with status resolved and endsAt now; the receiver's own
// autoresolve closes whatever it opened for that group.
func (c *Client) Resolve(ctx context.Context, config map[string]string, ref incident.Ref, ev incident.Event) error {
	return c.lifecycle(ctx, config, ref, ev, statusResolved)
}

// lifecycle takes the fingerprint from the Ref rather than ev.DedupKey, so Resolve still
// matches the open alert even if the host's key format changed between firing and recovery.
func (c *Client) lifecycle(ctx context.Context, config map[string]string, ref incident.Ref, ev incident.Event, status string) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	if err := ev.Validate(); err != nil {
		return fmt.Errorf("comm-alertmanager-webhook: %w", err)
	}
	fp, startsAt, err := parseRef(ref)
	if err != nil {
		return err
	}
	return c.send(ctx, cfg, buildPayload(cfg, ev, fp, startsAt, status, c.now()))
}

func parseRef(ref incident.Ref) (string, string, error) {
	fp, startsAt, ok := strings.Cut(ref.ExternalID, "@")
	if !ok || len(fp) != 16 {
		return "", "", errForeignRef
	}
	if _, err := hex.DecodeString(fp); err != nil {
		return "", "", errForeignRef
	}
	if _, err := time.Parse(time.RFC3339, startsAt); err != nil {
		return "", "", errForeignRef
	}
	return fp, startsAt, nil
}

func (c *Client) send(ctx context.Context, cfg amConfig, p amPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("comm-alertmanager-webhook: encode payload: %w", err)
	}
	return c.post(ctx, cfg, body)
}
