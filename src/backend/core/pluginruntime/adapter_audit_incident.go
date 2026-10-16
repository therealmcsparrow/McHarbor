// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"net/http"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const (
	actIncidentOpen    = "notification.incident.open"
	actIncidentUpdate  = "notification.incident.update"
	actIncidentResolve = "notification.incident.resolve"
)

// auditedIncidentClient is returned instead of auditedNotificationClient when
// the plugin implements incident.LifecycleClient, so the host's type assertion
// on the wrapped client still succeeds.
type auditedIncidentClient struct {
	*auditedNotificationClient
	lc incident.LifecycleClient
}

// Open records a shape-only audit row, never ticket content.
func (c *auditedIncidentClient) Open(ctx context.Context, cfg map[string]string, ev incident.Event) (incident.Ref, error) {
	start := time.Now()
	ref, err := c.lc.Open(ctx, cfg, ev)
	c.aud.record(ctx, c.slug, actIncidentOpen, c.key, start, err, auditAlways, map[string]any{"severity": string(ev.Severity)})
	return ref, err
}

// Update records a shape-only audit row.
func (c *auditedIncidentClient) Update(ctx context.Context, cfg map[string]string, ref incident.Ref, ev incident.Event) error {
	start := time.Now()
	err := c.lc.Update(ctx, cfg, ref, ev)
	c.aud.record(ctx, c.slug, actIncidentUpdate, c.key, start, err, auditAlways, map[string]any{"severity": string(ev.Severity)})
	return err
}

// Resolve records a shape-only audit row.
func (c *auditedIncidentClient) Resolve(ctx context.Context, cfg map[string]string, ref incident.Ref, ev incident.Event) error {
	start := time.Now()
	err := c.lc.Resolve(ctx, cfg, ref, ev)
	c.aud.record(ctx, c.slug, actIncidentResolve, c.key, start, err, auditAlways, nil)
	return err
}

type auditedIncidentWebhookClient struct {
	*auditedIncidentClient
	wp incident.WebhookParser
}

func (c *auditedIncidentWebhookClient) VerifyWebhook(cfg map[string]string, h http.Header, body []byte) error {
	return c.wp.VerifyWebhook(cfg, h, body)
}

func (c *auditedIncidentWebhookClient) ParseWebhook(ctx context.Context, cfg map[string]string, h http.Header, body []byte) ([]incident.RemoteStatus, error) {
	return c.wp.ParseWebhook(ctx, cfg, h, body)
}

type auditedIncidentPollClient struct {
	*auditedIncidentClient
	p incident.Poller
}

func (c *auditedIncidentPollClient) Poll(ctx context.Context, cfg map[string]string, refs []incident.Ref) ([]incident.RemoteStatus, error) {
	return c.p.Poll(ctx, cfg, refs)
}

type auditedIncidentFullClient struct {
	*auditedIncidentWebhookClient
	p incident.Poller
}

func (c *auditedIncidentFullClient) Poll(ctx context.Context, cfg map[string]string, refs []incident.Ref) ([]incident.RemoteStatus, error) {
	return c.p.Poll(ctx, cfg, refs)
}

// wrapIncidentCapabilities returns a wrapper exposing exactly the optional
// incident interfaces inner implements. Go's structural typing only forwards
// what the concrete type declares, so each combination is its own type.
func wrapIncidentCapabilities(base *auditedNotificationClient, inner plugin.NotificationAdapterClient) plugin.NotificationAdapterClient {
	lc, ok := inner.(incident.LifecycleClient)
	if !ok {
		return base
	}
	ic := &auditedIncidentClient{auditedNotificationClient: base, lc: lc}
	wp, hasWebhook := inner.(incident.WebhookParser)
	p, hasPoll := inner.(incident.Poller)
	switch {
	case hasWebhook && hasPoll:
		return &auditedIncidentFullClient{auditedIncidentWebhookClient: &auditedIncidentWebhookClient{auditedIncidentClient: ic, wp: wp}, p: p}
	case hasWebhook:
		return &auditedIncidentWebhookClient{auditedIncidentClient: ic, wp: wp}
	case hasPoll:
		return &auditedIncidentPollClient{auditedIncidentClient: ic, p: p}
	}
	return ic
}
