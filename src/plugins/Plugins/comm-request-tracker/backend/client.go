// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const (
	maxSubjectRunes = 200
	maxContentRunes = 16000
	subjectPrefix   = "[McHarbor] "
	reopenStatus    = "open"
)

// Client is stateless apart from its transport and clock, which tests replace.
type Client struct {
	hc  *http.Client
	now func() time.Time
}

// NewClient copies hc (nil means a fresh client) and refuses redirects on the copy, because a
// redirect is a misconfigured instance_url and following it could carry the token elsewhere.
func NewClient(hc *http.Client) *Client {
	cp := http.Client{Timeout: 35 * time.Second}
	if hc != nil {
		cp = *hc
		if cp.Timeout == 0 {
			cp.Timeout = 35 * time.Second
		}
	}
	cp.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{hc: &cp, now: time.Now}
}

var (
	_ plugin.NotificationAdapterClient = (*Client)(nil)
	_ incident.LifecycleClient         = (*Client)(nil)
	_ incident.WebhookParser           = (*Client)(nil)
	_ incident.Poller                  = (*Client)(nil)
)

// Test proves the queue is visible, the dedup custom field is applied to it and TicketSQL
// search works, and creates nothing, because a stray test ticket would page a queue's watchers.
func (c *Client) Test(ctx context.Context, raw map[string]string) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	var q struct {
		ID   rtID   `json:"id"`
		Name string `json:"Name"`
	}
	if _, err := c.do(ctx, cfg, http.MethodGet, "/queue/"+url.PathEscape(cfg.queue), nil, &q); err != nil {
		if errors.Is(err, incident.ErrNotFound) {
			return operatorf("queue %s was not found, or the token's user lacks SeeQueue on it", cfg.queue)
		}
		return err
	}
	queueID, err := ticketID(string(q.ID))
	if err != nil {
		return operatorf("RT did not return an id for queue %s", cfg.queue)
	}
	if err := c.checkCFApplied(ctx, cfg, queueID); err != nil {
		return err
	}
	var probe collection
	path := "/tickets?" + url.Values{"query": {"Queue = " + sqlQuote(cfg.queue)}, "per_page": {"1"}}.Encode()
	_, err = c.do(ctx, cfg, http.MethodGet, path, nil, &probe)
	return err
}

// checkCFApplied is Review Focus 1 caught before any alert: RT silently drops a CF value on
// create when the field is not applied to the queue, which would defeat Open's dedup search.
func (c *Client) checkCFApplied(ctx context.Context, cfg *config, queueID string) error {
	for page := 1; page <= 10; page++ {
		var cfs collection
		path := "/queue/" + queueID + "/customfields?" + url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}, "fields": {"Name"}}.Encode()
		_, err := c.do(ctx, cfg, http.MethodGet, path, nil, &cfs)
		if errors.Is(err, incident.ErrNotFound) && page == 1 {
			return c.checkCFExists(ctx, cfg)
		}
		if err != nil {
			return err
		}
		for _, it := range cfs.Items {
			if strings.EqualFold(strings.TrimSpace(it.Name), cfg.cfName) {
				return nil
			}
		}
		if cfs.Pages <= page {
			break
		}
	}
	return operatorf("custom field %s is not applied to queue %s", cfg.cfName, cfg.queue)
}

// checkCFExists is the fallback for RT builds without the queue customfields sub-collection:
// it can only prove the field exists, so Open's post-create check stays the backstop.
func (c *Client) checkCFExists(ctx context.Context, cfg *config) error {
	var cfs collection
	search := []map[string]string{{"field": "Name", "value": cfg.cfName}}
	if _, err := c.do(ctx, cfg, http.MethodPost, "/customfields?fields=Name", search, &cfs); err != nil {
		return err
	}
	for _, it := range cfs.Items {
		if strings.EqualFold(strings.TrimSpace(it.Name), cfg.cfName) {
			return nil
		}
	}
	return operatorf("custom field %s does not exist, or is not applied to queue %s", cfg.cfName, cfg.queue)
}

// Send is the plain-notification fallback: it creates a ticket without a dedup value and
// without searching, because a one-off notification has no lifecycle to correlate.
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
	_, err = c.create(ctx, cfg, msg.Title, clip(msg.Message, maxContentRunes), sev, "")
	return err
}

// Open searches for the active ticket carrying the hashed dedup key first and creates only on a
// miss, then re-reads the new ticket to prove RT stored the key, so a retry cannot duplicate.
func (c *Client) Open(ctx context.Context, raw map[string]string, ev incident.Event) (incident.Ref, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return incident.Ref{}, err
	}
	key := hashKey(ev.DedupKey, 64)
	hit, err := c.search(ctx, cfg, key, "__Active__", "Subject,Status")
	if err != nil {
		return incident.Ref{}, err
	}
	if hit != nil {
		return c.ref(cfg, string(hit.ID))
	}
	if cfg.reopenMinutes > 0 {
		if ref, ok, err := c.reopen(ctx, cfg, key, ev); err != nil || ok {
			return ref, err
		}
	}
	id, err := c.create(ctx, cfg, ev.Title, describe(ev), ev.Severity, key)
	if err != nil {
		return incident.Ref{}, err
	}
	if err := c.verifyCF(ctx, cfg, id, key); err != nil {
		return incident.Ref{}, err
	}
	return c.ref(cfg, id)
}

// search runs the dedup TicketSQL; every spliced value passed safeName or is hex, and is still
// quoted, so neither configuration nor an alert can widen the query.
func (c *Client) search(ctx context.Context, cfg *config, key, status, fields string) (*ticketItem, error) {
	query := "Queue = " + sqlQuote(cfg.queue) + " AND 'CF.{" + cfg.cfName + "}' = " + sqlQuote(key) + " AND Status = " + sqlQuote(status)
	orderBy := "Created"
	if status == "__Inactive__" {
		orderBy = "Resolved"
	}
	path := "/tickets?" + url.Values{
		"query": {query}, "fields": {fields}, "per_page": {"5"}, "orderby": {orderBy}, "order": {"DESC"},
	}.Encode()
	var res collection
	if _, err := c.do(ctx, cfg, http.MethodGet, path, nil, &res); err != nil {
		return nil, err
	}
	for i := range res.Items {
		if _, err := ticketID(string(res.Items[i].ID)); err == nil {
			return &res.Items[i], nil
		}
	}
	return nil, nil
}

// reopen revives the ticket this condition resolved within reopen_window_minutes, so a flapping
// alert keeps one history instead of a trail of tickets; a rejected ticket is never revived.
func (c *Client) reopen(ctx context.Context, cfg *config, key string, ev incident.Event) (incident.Ref, bool, error) {
	hit, err := c.search(ctx, cfg, key, "__Inactive__", "Status,Resolved")
	if err != nil || hit == nil {
		return incident.Ref{}, false, err
	}
	if st, ok := cfg.state(hit.Status); !ok || (st != incident.RemoteResolved && st != incident.RemoteClosed) {
		return incident.Ref{}, false, nil
	}
	resolved, err := time.Parse(time.RFC3339, strings.TrimSpace(hit.Resolved))
	if err != nil || c.now().Sub(resolved) > time.Duration(cfg.reopenMinutes)*time.Minute {
		return incident.Ref{}, false, nil
	}
	id := string(hit.ID)
	body := map[string]any{"Content": "Reopened: the alert fired again.\n\n" + describe(ev), "ContentType": "text/plain", "Status": reopenStatus}
	if _, err := c.do(ctx, cfg, http.MethodPost, "/ticket/"+id+"/comment", body, nil); err != nil {
		return incident.Ref{}, false, err
	}
	ref, err := c.ref(cfg, id)
	return ref, err == nil, err
}

func (c *Client) create(ctx context.Context, cfg *config, title, content string, sev incident.Severity, key string) (string, error) {
	body := map[string]any{
		"Queue":       cfg.queue,
		"Subject":     clip(subjectPrefix+oneLine(title), maxSubjectRunes),
		"Content":     content,
		"ContentType": "text/plain",
		"Priority":    cfg.priority(sev),
	}
	if key != "" {
		body["CustomFields"] = map[string]string{cfg.cfName: key}
	}
	if cfg.owner != "" {
		body["Owner"] = cfg.owner
	}
	if cfg.requestor != "" {
		body["Requestor"] = cfg.requestor
	}
	var created struct {
		ID rtID `json:"id"`
	}
	if _, err := c.do(ctx, cfg, http.MethodPost, "/ticket", body, &created); err != nil {
		return "", err
	}
	id, err := ticketID(string(created.ID))
	if err != nil {
		return "", operatorf("RT accepted the ticket but returned no ticket id")
	}
	return id, nil
}

// verifyCF is Review Focus 1 at run time: a created ticket without the key means the CF is not
// applied, and returning it permanently stops the outbox from creating one ticket per retry.
func (c *Client) verifyCF(ctx context.Context, cfg *config, id, key string) error {
	var t struct {
		CustomFields json.RawMessage `json:"CustomFields"`
	}
	if _, err := c.do(ctx, cfg, http.MethodGet, "/ticket/"+id, nil, &t); err != nil {
		return err
	}
	for _, v := range cfValues(t.CustomFields, cfg.cfName) {
		if strings.TrimSpace(v) == key {
			return nil
		}
	}
	return operatorf("custom field %s is not applied to queue %s: ticket #%s was created without the dedup value; apply the field to the queue and resolve that ticket by hand", cfg.cfName, cfg.queue, id)
}

// cfValues reads a custom field from REST2's array shape [{"name","values"}] (RT 5+) and from
// the object shape {"Name":["v"]} some RT 4.4 REST2 builds render.
func cfValues(raw json.RawMessage, name string) []string {
	var arr []struct {
		Name   string   `json:"name"`
		Values []string `json:"values"`
	}
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, f := range arr {
			if strings.EqualFold(strings.TrimSpace(f.Name), name) {
				return f.Values
			}
		}
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	for k, v := range obj {
		if !strings.EqualFold(strings.TrimSpace(k), name) {
			continue
		}
		var many []string
		if err := json.Unmarshal(v, &many); err == nil {
			return many
		}
		var one string
		if err := json.Unmarshal(v, &one); err == nil {
			return []string{one}
		}
	}
	return nil
}

// Update adds an internal comment; /correspond is never used because it mails requestors.
func (c *Client) Update(ctx context.Context, raw map[string]string, ref incident.Ref, ev incident.Event) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	id, err := ticketID(ref.ExternalID)
	if err != nil {
		return err
	}
	body := map[string]any{"Content": describe(ev), "ContentType": "text/plain"}
	_, err = c.do(ctx, cfg, http.MethodPost, "/ticket/"+id+"/comment", body, nil)
	return err
}

// Resolve comments and sets resolve_status in one call, so the closing note and the status
// change cannot drift apart; an answer without a status change is confirmed with a read.
func (c *Client) Resolve(ctx context.Context, raw map[string]string, ref incident.Ref, ev incident.Event) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	id, err := ticketID(ref.ExternalID)
	if err != nil {
		return err
	}
	body := map[string]any{"Content": "Recovered.\n\n" + describe(ev), "ContentType": "text/plain", "Status": cfg.resolveStatus}
	var lines []string
	if _, err := c.do(ctx, cfg, http.MethodPost, "/ticket/"+id+"/comment", body, &lines); err != nil {
		if errors.Is(err, errNotJSON) {
			lines = nil
		} else {
			return err
		}
	}
	for _, l := range lines {
		low := strings.ToLower(l)
		if strings.Contains(low, "valid status") {
			return invalidStatus(cfg, sanitize(l, cfg.token))
		}
		if strings.Contains(low, "status changed") {
			return nil
		}
	}
	var t ticketItem
	if _, err := c.do(ctx, cfg, http.MethodGet, "/ticket/"+id, nil, &t); err != nil {
		return err
	}
	if strings.EqualFold(t.Status, cfg.resolveStatus) {
		return nil
	}
	if st, ok := cfg.state(t.Status); ok && st.Terminal() {
		return nil
	}
	return operatorf("RT kept ticket #%s in status %q instead of %q; check resolve_status against queue %s's lifecycle", id, sanitize(t.Status), cfg.resolveStatus, cfg.queue)
}

func (c *Client) ref(cfg *config, id string) (incident.Ref, error) {
	id, err := ticketID(id)
	if err != nil {
		return incident.Ref{}, err
	}
	return incident.Ref{ExternalID: id, Number: id, URL: cfg.ticketURL(id)}, nil
}

// describe renders the event as the plain-text ticket body; labels are sorted so repeated
// comments for the same condition read identically.
func describe(ev incident.Event) string {
	var b strings.Builder
	if ev.Message != "" {
		b.WriteString(ev.Message)
		b.WriteString("\n\n")
	}
	line := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s: %s\n", k, oneLine(v))
		}
	}
	line("Alert", ev.Title)
	line("Severity", string(ev.Severity))
	line("Rule", ev.AlertName)
	line("Environment", ev.EnvironmentName)
	line("Subject", ev.SubjectName)
	line("Source", ev.Source)
	if !ev.OccurredAt.IsZero() {
		line("Occurred", ev.OccurredAt.UTC().Format(time.RFC3339))
	}
	line("Link", ev.Link)
	keys := make([]string, 0, len(ev.Labels))
	for k := range ev.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		line("Label "+k, ev.Labels[k])
	}
	return clip(strings.TrimRight(b.String(), "\n"), maxContentRunes)
}

func oneLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
