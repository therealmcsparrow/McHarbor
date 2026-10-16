// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const (
	testAlertName   = "McHarborConnectionTest"
	notifyAlertName = "McHarborNotification"
)

// Test proves DNS, reachability, the TLS chain and the SSRF policy without posting anything,
// because a default test that pages the on-call engineer is worse than a weaker one.
func (c *Client) Test(ctx context.Context, config map[string]string) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	if err := c.probe(ctx, cfg); err != nil {
		return err
	}
	if !cfg.TestSendsAlert {
		return nil
	}
	ev := incident.Event{
		DedupKey: "mcharbor-connection-test", State: incident.StateFiring, Severity: incident.SeverityInfo,
		Title: "McHarbor connection test", Message: "Test alert from McHarbor; it resolves immediately.",
		AlertName: testAlertName, SubjectName: "mcharbor",
	}
	return c.fireAndResolve(ctx, cfg, ev)
}

// probe dials through the same guarded dialer the transport uses, so Test refuses exactly the
// addresses a real delivery would refuse.
func (c *Client) probe(ctx context.Context, cfg amConfig) error {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return operatorErr("receiver URL must be an absolute https:// URL")
	}
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	conn, err := c.dialer.DialContext(ctx, "tcp", net.JoinHostPort(u.Hostname(), port))
	if err != nil {
		if errors.Is(err, errBlockedAddress) {
			return blockedErr(cfg.Origin)
		}
		return operatorErr("cannot reach receiver %s: %w", cfg.Origin, err)
	}
	defer conn.Close()
	if u.Scheme != "https" {
		return nil
	}
	tc := tls.Client(conn, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
	if err := tc.HandshakeContext(ctx); err != nil {
		return operatorErr("TLS handshake with receiver %s failed: %w", cfg.Origin, err)
	}
	return nil
}

// Send posts a firing alert and its resolve back to back: a plain notification has no
// lifecycle, and leaving it firing would keep an incident open at the receiver forever.
func (c *Client) Send(ctx context.Context, config map[string]string, msg plugin.NotificationMessage) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	if msg.Title == "" && msg.Message == "" {
		return operatorErr("notification has neither a title nor a message")
	}
	sum := sha256.Sum256([]byte(msg.Title + "\n" + msg.Message + "\n" + strconv.FormatInt(c.now().UnixNano(), 10)))
	ev := incident.Event{
		DedupKey: "mcharbor-notify:" + hex.EncodeToString(sum[:])[:32], State: incident.StateFiring,
		Severity: levelSeverity(msg.Level), Title: firstNonEmpty(msg.Title, msg.Message), Message: msg.Message,
		AlertName: notifyAlertName, SubjectName: "mcharbor",
	}
	return c.fireAndResolve(ctx, cfg, ev)
}

func (c *Client) fireAndResolve(ctx context.Context, cfg amConfig, ev incident.Event) error {
	fp, startsAt := fingerprint(ev.DedupKey), c.now().UTC().Truncate(time.Second).Format(time.RFC3339)
	if err := c.send(ctx, cfg, buildPayload(cfg, ev, fp, startsAt, statusFiring, time.Time{})); err != nil {
		return err
	}
	ev.State = incident.StateResolved
	if err := c.send(ctx, cfg, buildPayload(cfg, ev, fp, startsAt, statusResolved, c.now())); err != nil {
		return fmt.Errorf("comm-alertmanager-webhook: resolve after firing: %w", err)
	}
	return nil
}

func levelSeverity(level string) incident.Severity {
	switch level {
	case "error", "critical":
		return incident.SeverityCritical
	case "warning":
		return incident.SeverityWarning
	}
	return incident.SeverityInfo
}
