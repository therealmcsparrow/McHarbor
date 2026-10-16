// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// Client is stateless apart from its transport and clock, which tests replace.
type Client struct {
	hc  *http.Client
	now func() time.Time
}

// NewClient returns a Client on hc, or on a transport that refuses redirects when hc is nil,
// because following one would carry the access token to whatever host Zammad names.
func NewClient(hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{
			Timeout:       35 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return errRedirect },
		}
	}
	return &Client{hc: hc, now: time.Now}
}

var (
	_ plugin.NotificationAdapterClient = (*Client)(nil)
	_ incident.LifecycleClient         = (*Client)(nil)
	_ incident.WebhookParser           = (*Client)(nil)
	_ incident.Poller                  = (*Client)(nil)
)

// flexString decodes a JSON string, number, or an expanded association object into its name,
// because Zammad returns the same field as an id, a name, or an object depending on expand.
type flexString string

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if json.Unmarshal(b, &n) == nil {
		*f = flexString(n.String())
		return nil
	}
	var obj struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if json.Unmarshal(b, &obj) == nil {
		if obj.Login != "" {
			*f = flexString(obj.Login)
		} else {
			*f = flexString(obj.Name)
		}
		return nil
	}
	*f = ""
	return nil
}

// ticket is the subset of a Zammad ticket every call reads.
type ticket struct {
	ID        flexString `json:"id"`
	Number    flexString `json:"number"`
	Title     string     `json:"title"`
	State     flexString `json:"state"`
	Owner     flexString `json:"owner"`
	UpdatedAt string     `json:"updated_at"`
	CloseAt   string     `json:"close_at"`
}

func (c *Client) ref(cfg config, t ticket) incident.Ref {
	id := string(t.ID)
	return incident.Ref{ExternalID: id, Number: string(t.Number), URL: cfg.base + "/#ticket/zoom/" + id}
}

func article(subject, body string) map[string]any {
	return map[string]any{
		"subject":      subject,
		"body":         body,
		"type":         "note",
		"internal":     true,
		"content_type": "text/plain",
	}
}

func ticketID(ref incident.Ref) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(ref.ExternalID), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: ticket reference %q is not a Zammad ticket id", incident.ErrOperatorMessage, ref.ExternalID)
	}
	return id, nil
}

// Test proves the token, the group and the customer with read-only calls, so pressing Test
// connection never creates a ticket or notifies an agent.
func (c *Client) Test(ctx context.Context, raw map[string]string) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	var me struct {
		ID flexString `json:"id"`
	}
	if err := c.do(ctx, cfg, http.MethodGet, "/api/v1/users/me", nil, &me); err != nil {
		return err
	}
	if err := c.checkGroup(ctx, cfg); err != nil {
		return err
	}
	return c.checkCustomer(ctx, cfg)
}

func (c *Client) checkGroup(ctx context.Context, cfg config) error {
	const perPage = 100
	for page := 1; page <= 20; page++ {
		var groups []struct {
			Name   string `json:"name"`
			Active bool   `json:"active"`
		}
		path := fmt.Sprintf("/api/v1/groups?expand=true&page=%d&per_page=%d", page, perPage)
		if err := c.do(ctx, cfg, http.MethodGet, path, nil, &groups); err != nil {
			return err
		}
		for _, g := range groups {
			if g.Name == cfg.group && g.Active {
				return nil
			}
		}
		if len(groups) < perPage {
			break
		}
	}
	return fmt.Errorf("%w: group %s not found or inactive", incident.ErrOperatorMessage, sanitize(cfg.group))
}

func (c *Client) checkCustomer(ctx context.Context, cfg config) error {
	if strings.HasPrefix(cfg.customer, "guess:") {
		return nil
	}
	var users []struct {
		Login string `json:"login"`
		Email string `json:"email"`
	}
	path := "/api/v1/users/search?query=" + url.QueryEscape(cfg.customer) + "&limit=10"
	if err := c.do(ctx, cfg, http.MethodGet, path, nil, &users); err != nil {
		return err
	}
	for _, u := range users {
		if strings.EqualFold(u.Login, cfg.customer) || strings.EqualFold(u.Email, cfg.customer) {
			return nil
		}
	}
	return fmt.Errorf("%w: customer %s not found; create the user or use guess:<email>", incident.ErrOperatorMessage, sanitize(cfg.customer))
}

// Send opens a plain ticket for callers outside the incident lifecycle; it carries no marker
// and no mcharbor tag, so the lifecycle's dedup and the status trigger both ignore it.
func (c *Client) Send(ctx context.Context, raw map[string]string, msg plugin.NotificationMessage) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	sev := incident.SeverityInfo
	switch strings.ToLower(msg.Level) {
	case "error", "critical":
		sev = incident.SeverityCritical
	case "warning":
		sev = incident.SeverityWarning
	}
	title := strings.Join(strings.Fields(msg.Title), " ")
	if title == "" {
		title = "McHarbor notification"
	}
	if r := []rune(title); len(r) > maxTitleRunes {
		title = string(r[:maxTitleRunes])
	}
	body := msg.Message
	if strings.TrimSpace(body) == "" {
		body = title
	}
	return c.do(ctx, cfg, http.MethodPost, "/api/v1/tickets", c.createBody(cfg, title, sev, body), nil)
}

func (c *Client) createBody(cfg config, title string, sev incident.Severity, body string) map[string]any {
	req := map[string]any{
		"title":    title,
		"group":    cfg.group,
		"customer": cfg.customer,
		"priority": cfg.severity[sev],
		"article":  article(title, body),
	}
	if cfg.owner != "" {
		req["owner"] = cfg.owner
	}
	return req
}

// Open returns the live ticket already carrying this firing's marker, or creates one, and
// tags it either way: a retry after a lost response or a failed tag call never duplicates.
func (c *Client) Open(ctx context.Context, raw map[string]string, ev incident.Event) (incident.Ref, error) {
	if err := ev.Validate(); err != nil {
		return incident.Ref{}, err
	}
	cfg, err := parseConfig(raw)
	if err != nil {
		return incident.Ref{}, err
	}
	m := marker(ev)
	existing, found, err := c.findLive(ctx, cfg, m)
	if err != nil {
		return incident.Ref{}, err
	}
	if !found && cfg.reopenWindow > 0 {
		reopened, ok, err := c.reopen(ctx, cfg, ev)
		if err != nil {
			return incident.Ref{}, err
		}
		existing, found = reopened, ok
	}
	if !found {
		title := ticketTitle(ev)
		if err := c.do(ctx, cfg, http.MethodPost, "/api/v1/tickets", c.createBody(cfg, title, ev.Severity, describe(ev)), &existing); err != nil {
			return incident.Ref{}, err
		}
		if existing.ID == "" {
			return incident.Ref{}, fmt.Errorf("%w: Zammad created the ticket but returned no id", incident.ErrRetryable)
		}
	}
	if err := c.tag(ctx, cfg, existing.ID); err != nil {
		return incident.Ref{}, err
	}
	return c.ref(cfg, existing), nil
}

// reopen sets the ticket a previous firing of this condition closed back to open when it closed
// within reopen_window_minutes, so a flapping alert keeps one history; a merged ticket never returns.
func (c *Client) reopen(ctx context.Context, cfg config, ev incident.Event) (ticket, bool, error) {
	cm := conditionMarker(ev)
	query := `"` + strings.TrimSuffix(strings.TrimPrefix(cm, "["), "]") + `"`
	var hits []ticket
	path := "/api/v1/tickets/search?query=" + url.QueryEscape(query) + "&limit=10&expand=true"
	if err := c.do(ctx, cfg, http.MethodGet, path, nil, &hits); err != nil {
		return ticket{}, false, err
	}
	window := time.Duration(cfg.reopenWindow) * time.Minute
	var best ticket
	var bestAt time.Time
	for _, t := range hits {
		if t.ID == "" || !strings.Contains(t.Title, cm) || !strings.EqualFold(strings.TrimSpace(string(t.State)), "closed") {
			continue
		}
		closed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(t.CloseAt))
		if err != nil || c.now().Sub(closed) > window || !closed.After(bestAt) {
			continue
		}
		best, bestAt = t, closed
	}
	if best.ID == "" {
		return ticket{}, false, nil
	}
	id, err := strconv.ParseInt(string(best.ID), 10, 64)
	if err != nil {
		return ticket{}, false, fmt.Errorf("%w: Zammad returned a non-numeric ticket id", incident.ErrOperatorMessage)
	}
	body := map[string]any{
		"state":   reopenState,
		"article": article(ev.Title, "Reopened: the alert fired again.\n\n"+describe(ev)),
	}
	if err := c.do(ctx, cfg, http.MethodPut, "/api/v1/tickets/"+strconv.FormatInt(id, 10), body, nil); err != nil {
		return ticket{}, false, err
	}
	return best, true, nil
}

// findLive searches for the marker and keeps only a title that starts with it and a state that
// is not closed or merged, because the database fallback without Elasticsearch matches fuzzily.
func (c *Client) findLive(ctx context.Context, cfg config, m string) (ticket, bool, error) {
	query := `"` + strings.TrimSuffix(strings.TrimPrefix(m, "["), "]") + `"`
	var hits []ticket
	path := "/api/v1/tickets/search?query=" + url.QueryEscape(query) + "&limit=10&expand=true"
	if err := c.do(ctx, cfg, http.MethodGet, path, nil, &hits); err != nil {
		return ticket{}, false, err
	}
	for _, t := range hits {
		if strings.HasPrefix(t.Title, m) && isLiveState(string(t.State)) && t.ID != "" {
			return t, true, nil
		}
	}
	return ticket{}, false, nil
}

func (c *Client) tag(ctx context.Context, cfg config, id flexString) error {
	oid, err := strconv.ParseInt(string(id), 10, 64)
	if err != nil {
		return fmt.Errorf("%w: Zammad returned a non-numeric ticket id", incident.ErrOperatorMessage)
	}
	body := map[string]any{"object": "Ticket", "o_id": oid, "item": mcharborTag}
	return c.do(ctx, cfg, http.MethodPost, "/api/v1/tags/add", body, nil)
}

// Update appends the event as an internal note without touching the ticket's state.
func (c *Client) Update(ctx context.Context, raw map[string]string, ref incident.Ref, ev incident.Event) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	id, err := ticketID(ref)
	if err != nil {
		return err
	}
	body := article(ev.Title, describe(ev))
	body["ticket_id"] = id
	return c.doOpts(ctx, cfg, http.MethodPost, "/api/v1/ticket_articles", body, nil, reqOpts{suppress: true})
}

// Resolve sets resolve_state and adds the recovery note in one PUT, so a ticket is never
// closed without the note that explains why, nor noted without being closed.
func (c *Client) Resolve(ctx context.Context, raw map[string]string, ref incident.Ref, ev incident.Event) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	id, err := ticketID(ref)
	if err != nil {
		return err
	}
	body := map[string]any{
		"state":   cfg.resolveState,
		"article": article(ev.Title, describe(ev)),
	}
	return c.doOpts(ctx, cfg, http.MethodPut, "/api/v1/tickets/"+strconv.FormatInt(id, 10), body, nil, reqOpts{suppress: true})
}

func (c *Client) status(cfg config, base string, t ticket) incident.RemoteStatus {
	id := string(t.ID)
	st := incident.RemoteStatus{
		Ref:      incident.Ref{ExternalID: id, Number: string(t.Number)},
		State:    remoteState(cfg.status, string(t.State)),
		Assignee: string(t.Owner),
	}
	if base != "" {
		st.Ref.URL = base + "/#ticket/zoom/" + id
	}
	if st.Assignee == unassigned {
		st.Assignee = ""
	}
	if ts, err := time.Parse(time.RFC3339Nano, t.UpdatedAt); err == nil {
		st.UpdatedAt = ts
	}
	return st
}
