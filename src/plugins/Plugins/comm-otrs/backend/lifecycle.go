// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

type createResp struct {
	TicketID     json.RawMessage `json:"TicketID"`
	TicketNumber json.RawMessage `json:"TicketNumber"`
}

type searchResp struct {
	TicketID json.RawMessage `json:"TicketID"`
}

type getResp struct {
	Ticket json.RawMessage `json:"Ticket"`
}

var openStateTypes = []string{"new", "open", "pending reminder", "pending auto"}

func rawString(raw json.RawMessage) string {
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return str(v)
}

func (r searchResp) ids() []string {
	var v any
	if len(r.TicketID) == 0 || json.Unmarshal(r.TicketID, &v) != nil {
		return nil
	}
	if list, ok := v.([]any); ok {
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s := str(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	if s := str(v); s != "" {
		return []string{s}
	}
	return nil
}

func (r getResp) tickets() []map[string]any {
	var list []map[string]any
	if json.Unmarshal(r.Ticket, &list) == nil {
		return list
	}
	var one map[string]any
	if json.Unmarshal(r.Ticket, &one) == nil && one != nil {
		return []map[string]any{one}
	}
	return nil
}

func (c *Client) getOne(ctx context.Context, cfg config, id string) (map[string]any, error) {
	var out getResp
	if err := c.call(ctx, cfg, "/TicketGet", "POST", map[string]any{"TicketID": id, "DynamicFields": 1}, &out); err != nil {
		return nil, err
	}
	for _, t := range out.tickets() {
		if str(t["TicketID"]) == id {
			return t, nil
		}
	}
	return nil, fmt.Errorf("comm-otrs: ticket %s: %w", id, incident.ErrNotFound)
}

// Open searches the dedup dynamic field before creating, and re-reads any hit, because a
// mistyped field name makes TicketSearch ignore the filter and match unrelated tickets.
func (c *Client) Open(ctx context.Context, m map[string]string, ev incident.Event) (incident.Ref, error) {
	cfg, err := parseConfig(m)
	if err != nil {
		return incident.Ref{}, err
	}
	var hits searchResp
	search := map[string]any{
		"DynamicField_" + cfg.DedupField: map[string]any{"Equals": ev.DedupKey},
		"StateType":                      openStateTypes,
		"Queues":                         []string{cfg.Queue},
		"Limit":                          2,
	}
	if err := c.call(ctx, cfg, "/TicketSearch", "POST", search, &hits); err != nil {
		return incident.Ref{}, err
	}
	for _, id := range hits.ids() {
		t, err := c.getOne(ctx, cfg, id)
		if errors.Is(err, incident.ErrNotFound) {
			continue
		}
		if err != nil {
			return incident.Ref{}, err
		}
		if cfg.dedupValue(t) == ev.DedupKey {
			return cfg.ref(id, str(t["TicketNumber"])), nil
		}
	}
	return c.create(ctx, cfg, ev.Severity, ev.Title, renderBody(ev), ev.DedupKey)
}

func (c *Client) create(ctx context.Context, cfg config, sev incident.Severity, title, text, dedupKey string) (incident.Ref, error) {
	priority := cfg.Severity[sev]
	if priority == "" {
		priority = cfg.Severity[incident.SeverityInfo]
	}
	if strings.TrimSpace(title) == "" {
		title = "McHarbor notification"
	}
	ticket := map[string]any{
		"Title":        truncate(title, 255),
		"Queue":        cfg.Queue,
		"State":        "new",
		"Priority":     priority,
		"CustomerUser": cfg.CustomerUser,
	}
	if cfg.TicketType != "" {
		ticket["Type"] = cfg.TicketType
	}
	body := map[string]any{
		"Ticket":  ticket,
		"Article": article(title, text),
	}
	if dedupKey != "" {
		body["DynamicField"] = []map[string]any{{"Name": cfg.DedupField, "Value": dedupKey}}
	}
	var out createResp
	if err := c.call(ctx, cfg, "/Ticket", "POST", body, &out); err != nil {
		return incident.Ref{}, err
	}
	id := rawString(out.TicketID)
	if id == "" {
		return incident.Ref{}, fmt.Errorf("%w: TicketCreate returned no TicketID", incident.ErrOperatorMessage)
	}
	return cfg.ref(id, rawString(out.TicketNumber)), nil
}

// Update appends an internal article only, so repeats never reset state or priority an agent set.
func (c *Client) Update(ctx context.Context, m map[string]string, ref incident.Ref, ev incident.Event) error {
	cfg, err := parseConfig(m)
	if err != nil {
		return err
	}
	return c.call(ctx, cfg, "/Ticket/"+url.PathEscape(ref.ExternalID), "PATCH",
		map[string]any{"Article": article("Update: "+ev.Title, renderBody(ev))}, nil)
}

// Resolve sets the configured close state and records why in the same request, so the ticket
// never shows closed without an explanation.
func (c *Client) Resolve(ctx context.Context, m map[string]string, ref incident.Ref, ev incident.Event) error {
	cfg, err := parseConfig(m)
	if err != nil {
		return err
	}
	return c.call(ctx, cfg, "/Ticket/"+url.PathEscape(ref.ExternalID), "PATCH", map[string]any{
		"Ticket":  map[string]any{"State": cfg.CloseState},
		"Article": article("Recovered: "+ev.Title, renderBody(ev)),
	}, nil)
}

// Send files a plain notification as a ticket without the dedup field, because a message that is
// not part of an alert lifecycle must never be matched and reused by a later Open.
func (c *Client) Send(ctx context.Context, m map[string]string, msg plugin.NotificationMessage) error {
	cfg, err := parseConfig(m)
	if err != nil {
		return err
	}
	sev := incident.SeverityInfo
	switch strings.ToLower(strings.TrimSpace(msg.Level)) {
	case "error":
		sev = incident.SeverityCritical
	case "warning":
		sev = incident.SeverityWarning
	}
	_, err = c.create(ctx, cfg, sev, msg.Title, msg.Message, "")
	return err
}

func article(subject, text string) map[string]any {
	if strings.TrimSpace(text) == "" {
		text = subject
	}
	return map[string]any{
		"CommunicationChannel": "Internal",
		"SenderType":           "system",
		"IsVisibleForCustomer": 0,
		"Subject":              truncate(subject, 255),
		"Body":                 text,
		"ContentType":          "text/plain; charset=utf8",
	}
}
