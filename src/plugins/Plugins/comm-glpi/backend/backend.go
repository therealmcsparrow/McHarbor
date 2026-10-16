// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// ticket is the subset of a GLPI ticket both APIs report the same way.
type ticket struct {
	ID         int
	Name       string
	Status     int
	ExternalID string
	Deleted    bool
	Modified   string
}

// ticketInput is one create request, rendered per API by v1Map and v2Map.
type ticketInput struct {
	Name       string
	Content    string
	Urgency    int
	Impact     int
	Priority   int
	ExternalID string
	EntityID   int
	CategoryID int
	GroupID    int
}

const ticketTypeIncident = 1

func (t ticketInput) v1Map() map[string]any {
	m := map[string]any{
		"name":     t.Name,
		"content":  t.Content,
		"type":     ticketTypeIncident,
		"urgency":  t.Urgency,
		"impact":   t.Impact,
		"priority": t.Priority,
	}
	if t.ExternalID != "" {
		m["externalid"] = t.ExternalID
	}
	if t.EntityID > 0 {
		m["entities_id"] = t.EntityID
	}
	if t.CategoryID > 0 {
		m["itilcategories_id"] = t.CategoryID
	}
	if t.GroupID > 0 {
		m["_groups_id_assign"] = t.GroupID
	}
	return m
}

func (t ticketInput) v2Map() map[string]any {
	m := map[string]any{
		"name":     t.Name,
		"content":  t.Content,
		"type":     ticketTypeIncident,
		"urgency":  t.Urgency,
		"impact":   t.Impact,
		"priority": t.Priority,
	}
	if t.ExternalID != "" {
		m["external_id"] = t.ExternalID
	}
	if t.EntityID > 0 {
		m["entity"] = map[string]int{"id": t.EntityID}
	}
	if t.CategoryID > 0 {
		m["category"] = map[string]int{"id": t.CategoryID}
	}
	return m
}

// glpiBackend hides the v1 and v2 API surfaces so Open, Update and Resolve are written once.
type glpiBackend interface {
	findOpenByExternalID(ctx context.Context, externalID string) (ticket, bool, error)
	create(ctx context.Context, t ticketInput) (int, error)
	get(ctx context.Context, id int) (ticket, error)
	followup(ctx context.Context, id int, content string) error
	solve(ctx context.Context, id int, content string) error
	setStatus(ctx context.Context, id, status int) error
	close() error
}

// cachedToken is a v2 bearer token and the moment it stops being reused.
type cachedToken struct {
	token   string
	expires time.Time
}

// Client is shared by every GLPI channel and goroutine; its only state is the v2 token cache,
// keyed by a hash of the credentials and guarded by mu.
type Client struct {
	hc     *http.Client
	now    func() time.Time
	mu     sync.Mutex
	tokens map[string]cachedToken
}

// NewClient returns a Client over hc (nil means a fresh client with a 35s backstop timeout) that
// never follows redirects, because App-Token and Session-Token would travel to the new host.
func NewClient(hc *http.Client) *Client {
	base := http.Client{Timeout: 35 * time.Second}
	if hc != nil {
		base = *hc
	}
	base.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{hc: &base, now: time.Now, tokens: map[string]cachedToken{}}
}

func (c *Client) open(ctx context.Context, cfg glpiConfig) (glpiBackend, error) {
	if cfg.API == apiV2 {
		return c.openV2(ctx, cfg)
	}
	return c.openV1(ctx, cfg)
}

// flexInt decodes GLPI numbers that some versions send as strings.
type flexInt int

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexInt) UnmarshalJSON(b []byte) error {
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		var s string
		if json.Unmarshal(b, &s) != nil || s == "" {
			*f = 0
			return nil
		}
		n = json.Number(s)
	}
	v, err := n.Int64()
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt(v)
	return nil
}

// flexBool decodes is_deleted, which v1 sends as 0/1 and v2 as a boolean.
type flexBool bool

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexBool) UnmarshalJSON(b []byte) error {
	var v bool
	if json.Unmarshal(b, &v) == nil {
		*f = flexBool(v)
		return nil
	}
	var n flexInt
	if err := n.UnmarshalJSON(b); err != nil {
		return err
	}
	*f = n != 0
	return nil
}

// statusField decodes the v2 status object {"id":N,"name":…} and also a bare number.
type statusField int

// UnmarshalJSON implements json.Unmarshaler.
func (s *statusField) UnmarshalJSON(b []byte) error {
	var obj struct {
		ID flexInt `json:"id"`
	}
	if json.Unmarshal(b, &obj) == nil {
		*s = statusField(obj.ID)
		return nil
	}
	var n flexInt
	if err := n.UnmarshalJSON(b); err != nil {
		return err
	}
	*s = statusField(n)
	return nil
}
