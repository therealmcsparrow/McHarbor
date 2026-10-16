// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// Client is the stateless PagerDuty adapter; the base URLs are empty in production and set
// only by tests, so a channel's config can never redirect traffic to another host.
type Client struct {
	hc         *http.Client
	now        func() time.Time
	eventsBase string
	restBase   string
}

// NewClient returns a Client whose transport timeout backs up the per-call 30s context,
// because a plugin must not rely on http.DefaultClient having any timeout at all.
func NewClient() *Client {
	return &Client{hc: &http.Client{Timeout: 35 * time.Second}, now: time.Now}
}

var (
	_ plugin.NotificationAdapterClient = (*Client)(nil)
	_ incident.LifecycleClient         = (*Client)(nil)
	_ incident.WebhookParser           = (*Client)(nil)
	_ incident.Poller                  = (*Client)(nil)
)

func (c *Client) eventsURL(cfg pdConfig) string {
	if c.eventsBase != "" {
		return c.eventsBase
	}
	if cfg.Region == "eu" {
		return "https://events.eu.pagerduty.com"
	}
	return "https://events.pagerduty.com"
}

func (c *Client) restURL(cfg pdConfig) string {
	if c.restBase != "" {
		return c.restBase
	}
	if cfg.Region == "eu" {
		return "https://api.eu.pagerduty.com"
	}
	return "https://api.pagerduty.com"
}
