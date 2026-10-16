// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const maxSummary = 1024

type pdLink struct {
	Href string `json:"href"`
	Text string `json:"text"`
}

type pdPayload struct {
	Summary       string            `json:"summary"`
	Source        string            `json:"source"`
	Severity      string            `json:"severity"`
	Timestamp     string            `json:"timestamp,omitempty"`
	Component     string            `json:"component,omitempty"`
	Group         string            `json:"group,omitempty"`
	Class         string            `json:"class,omitempty"`
	CustomDetails map[string]string `json:"custom_details,omitempty"`
}

type pdEvent struct {
	RoutingKey  string     `json:"routing_key"`
	EventAction string     `json:"event_action"`
	DedupKey    string     `json:"dedup_key"`
	Payload     *pdPayload `json:"payload,omitempty"`
	Links       []pdLink   `json:"links,omitempty"`
	Client      string     `json:"client,omitempty"`
	ClientURL   string     `json:"client_url,omitempty"`
}

type pdEnqueueResponse struct {
	Status   string `json:"status"`
	DedupKey string `json:"dedup_key"`
}

var (
	errDedupRewritten = errors.New("comm-pagerduty: PagerDuty returned a different dedup_key")
	errNoRef          = fmt.Errorf("%w: comm-pagerduty: link has no dedup key", incident.ErrOperatorMessage)
)

// Open triggers on ev.DedupKey; PagerDuty folds a repeat trigger into the open alert, which is
// what makes a retry after a lost 202 safe without any lookup first.
func (c *Client) Open(ctx context.Context, config map[string]string, ev incident.Event) (incident.Ref, error) {
	cfg, err := parseConfig(config)
	if err != nil {
		return incident.Ref{}, err
	}
	if err := ev.Validate(); err != nil {
		return incident.Ref{}, fmt.Errorf("comm-pagerduty: %w", err)
	}
	if err := c.enqueue(ctx, cfg, c.triggerEvent(cfg, ev)); err != nil {
		return incident.Ref{}, err
	}
	return incident.Ref{ExternalID: ev.DedupKey}, nil
}

// Update re-sends a trigger with the new summary, because the Events API has no note verb and a
// REST note needs a user email McHarbor does not own.
func (c *Client) Update(ctx context.Context, config map[string]string, ref incident.Ref, ev incident.Event) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	if ref.ExternalID == "" {
		return errNoRef
	}
	ev.DedupKey = ref.ExternalID
	return c.enqueue(ctx, cfg, c.triggerEvent(cfg, ev))
}

// Resolve sends resolve on the link's key; PagerDuty drops it silently if the alert is already
// resolved, so a duplicate resolve from the outbox is harmless.
func (c *Client) Resolve(ctx context.Context, config map[string]string, ref incident.Ref, _ incident.Event) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	if ref.ExternalID == "" {
		return errNoRef
	}
	return c.enqueue(ctx, cfg, pdEvent{RoutingKey: cfg.RoutingKey, EventAction: "resolve", DedupKey: ref.ExternalID})
}

func (c *Client) triggerEvent(cfg pdConfig, ev incident.Event) pdEvent {
	details := map[string]string{"message": ev.Message, "environment": ev.EnvironmentName, "subject": ev.SubjectName, "alert": ev.AlertName}
	for k, v := range ev.Labels {
		details["label."+k] = v
	}
	ts := ev.OccurredAt
	if ts.IsZero() {
		ts = c.now()
	}
	sev := cfg.SeverityMap[ev.Severity]
	if sev == "" {
		sev = "error"
	}
	out := pdEvent{
		RoutingKey:  cfg.RoutingKey,
		EventAction: "trigger",
		DedupKey:    ev.DedupKey,
		Client:      "McHarbor",
		ClientURL:   ev.Link,
		Payload: &pdPayload{
			Summary:       truncateRunes(firstNonEmpty(ev.Title, ev.Message, ev.AlertName, "McHarbor alert"), maxSummary),
			Source:        firstNonEmpty(ev.Source, ev.SubjectName, "mcharbor"),
			Severity:      sev,
			Timestamp:     ts.UTC().Format(time.RFC3339),
			Component:     ev.SubjectName,
			Group:         ev.EnvironmentName,
			Class:         ev.AlertName,
			CustomDetails: details,
		},
	}
	if ev.Link != "" {
		out.Links = []pdLink{{Href: ev.Link, Text: "Open in McHarbor"}}
	}
	return out
}

func (c *Client) enqueue(ctx context.Context, cfg pdConfig, body pdEvent) error {
	var resp pdEnqueueResponse
	if err := c.doJSON(ctx, http.MethodPost, c.eventsURL(cfg)+"/v2/enqueue", nil, body, &resp, cfg.RoutingKey, cfg.RESTKey); err != nil {
		return err
	}
	if resp.DedupKey != "" && resp.DedupKey != body.DedupKey {
		return errDedupRewritten
	}
	return nil
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
