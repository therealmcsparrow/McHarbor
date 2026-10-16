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
	"strconv"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// v1 is one legacy REST API session; it lives for a single call chain and is killed in close,
// because GLPI sessions are PHP sessions and a leaked one lingers in the session store.
type v1 struct {
	c       *Client
	cfg     glpiConfig
	mu      sync.Mutex
	session string
}

func (c *Client) initSession(ctx context.Context, cfg glpiConfig) (string, error) {
	var out struct {
		SessionToken string `json:"session_token"`
	}
	hdr := http.Header{"Authorization": {"user_token " + cfg.UserToken}}
	if cfg.AppToken != "" {
		hdr.Set("App-Token", cfg.AppToken)
	}
	if _, err := c.do(ctx, http.MethodGet, cfg.BaseURL+"/apirest.php/initSession", hdr, nil, &out, cfg.secrets()...); err != nil {
		if errors.Is(err, incident.ErrNotFound) {
			return "", operatorError("GLPI has no REST API at /apirest.php; check the GLPI URL and that the REST API is enabled")
		}
		return "", fmt.Errorf("comm-glpi: initSession: %w", err)
	}
	if out.SessionToken == "" {
		return "", operatorError("GLPI returned no session token; check that the REST API is enabled and the API client admits McHarbor's IP")
	}
	return out.SessionToken, nil
}

func (c *Client) openV1(ctx context.Context, cfg glpiConfig) (*v1, error) {
	session, err := c.initSession(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &v1{c: c, cfg: cfg, session: session}, nil
}

func (b *v1) current() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.session
}

func (b *v1) headersFor(session string) http.Header {
	h := http.Header{"Session-Token": {session}}
	if b.cfg.AppToken != "" {
		h.Set("App-Token", b.cfg.AppToken)
	}
	return h
}

func (b *v1) secretsFor(session string) []string {
	return append(b.cfg.secrets(), session)
}

// renew replaces a session GLPI reported gone; concurrent poll workers that saw the same stale
// session share one new session instead of each opening their own.
func (b *v1) renew(ctx context.Context, stale string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.session != stale {
		return b.session, nil
	}
	session, err := b.c.initSession(ctx, b.cfg)
	if err != nil {
		return "", err
	}
	b.session = session
	return session, nil
}

// call sends one request in the session and, when GLPI reports the session gone, opens a new
// one and replays once; the replay is safe because GLPI rejected the first request unprocessed.
func (b *v1) call(ctx context.Context, method, path string, body, out any) (result, error) {
	target := b.cfg.BaseURL + "/apirest.php/" + path
	session := b.current()
	res, err := b.c.do(ctx, method, target, b.headersFor(session), body, out, b.secretsFor(session)...)
	if !errors.Is(err, errSessionInvalid) {
		return res, err
	}
	session, err = b.renew(ctx, session)
	if err != nil {
		return result{}, err
	}
	res, err = b.c.do(ctx, method, target, b.headersFor(session), body, out, b.secretsFor(session)...)
	if errors.Is(err, errSessionInvalid) {
		return res, fmt.Errorf("%w: comm-glpi: GLPI rejected a fresh session: %w", incident.ErrRetryable, err)
	}
	return res, err
}

// close kills the PHP session with its own short context, because the caller's context may
// already be cancelled and a leaked session lingers until GLPI's gc runs.
func (b *v1) close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session := b.current()
	if _, err := b.c.do(ctx, http.MethodGet, b.cfg.BaseURL+"/apirest.php/killSession", b.headersFor(session), nil, nil, b.secretsFor(session)...); err != nil {
		return fmt.Errorf("comm-glpi: killSession: %w", err)
	}
	return nil
}

type v1Ticket struct {
	ID         flexInt  `json:"id"`
	Name       string   `json:"name"`
	Status     flexInt  `json:"status"`
	ExternalID string   `json:"externalid"`
	IsDeleted  flexBool `json:"is_deleted"`
	DateMod    string   `json:"date_mod"`
}

func (r v1Ticket) ticket() ticket {
	return ticket{ID: int(r.ID), Name: r.Name, Status: int(r.Status), ExternalID: r.ExternalID, Deleted: bool(r.IsDeleted), Modified: r.DateMod}
}

// findOpenByExternalID compares externalid exactly, because searchText is a LIKE match and
// would also return a longer key that merely contains this one.
func (b *v1) findOpenByExternalID(ctx context.Context, ext string) (ticket, bool, error) {
	q := url.Values{}
	q.Set("searchText[externalid]", ext)
	q.Set("range", "0-49")
	q.Set("expand_dropdowns", "false")
	var rows []v1Ticket
	if _, err := b.call(ctx, http.MethodGet, "Ticket/?"+q.Encode(), nil, &rows); err != nil {
		if errors.Is(err, incident.ErrNotFound) {
			return ticket{}, false, operatorError("GLPI answered 404 to the ticket search; check the API user's rights on tickets")
		}
		return ticket{}, false, err
	}
	for _, r := range rows {
		if r.ExternalID == ext && r.Status < 5 && !r.IsDeleted {
			return r.ticket(), true, nil
		}
	}
	return ticket{}, false, nil
}

// create treats 207 Multi-Status as a refusal: GLPI then answers [{"id":false,"message"}],
// and reading that as success would link the alert to ticket 0.
func (b *v1) create(ctx context.Context, t ticketInput) (int, error) {
	var out json.RawMessage
	res, err := b.call(ctx, http.MethodPost, "Ticket/", map[string]any{"input": t.v1Map()}, &out)
	if err != nil {
		if errors.Is(err, incident.ErrNotFound) {
			return 0, operatorError("GLPI answered 404 to the ticket create; check the API user's rights on tickets")
		}
		return 0, err
	}
	if res.status == http.StatusMultiStatus {
		return 0, operatorError("GLPI refused the ticket: %s", sanitize(firstMessage(out), b.secretsFor(b.current())...))
	}
	var created struct {
		ID json.Number `json:"id"`
	}
	if err := json.Unmarshal(out, &created); err != nil {
		return 0, fmt.Errorf("comm-glpi: decode create: %w", err)
	}
	id, err := created.ID.Int64()
	if err != nil || id <= 0 {
		return 0, operatorError("GLPI returned no ticket id")
	}
	return int(id), nil
}

func (b *v1) get(ctx context.Context, id int) (ticket, error) {
	var r v1Ticket
	if _, err := b.call(ctx, http.MethodGet, "Ticket/"+strconv.Itoa(id)+"?expand_dropdowns=false", nil, &r); err != nil {
		return ticket{}, err
	}
	return r.ticket(), nil
}

func (b *v1) followup(ctx context.Context, id int, content string) error {
	in := map[string]any{"itemtype": "Ticket", "items_id": id, "content": content, "is_private": 1}
	_, err := b.call(ctx, http.MethodPost, "ITILFollowup/", map[string]any{"input": in}, nil)
	return err
}

func (b *v1) solve(ctx context.Context, id int, content string) error {
	in := map[string]any{"itemtype": "Ticket", "items_id": id, "content": content}
	_, err := b.call(ctx, http.MethodPost, "ITILSolution/", map[string]any{"input": in}, nil)
	return err
}

func (b *v1) setStatus(ctx context.Context, id, status int) error {
	var out json.RawMessage
	res, err := b.call(ctx, http.MethodPut, "Ticket/"+strconv.Itoa(id), map[string]any{"input": map[string]int{"status": status}}, &out)
	if err != nil {
		return err
	}
	if res.status == http.StatusMultiStatus {
		return operatorError("GLPI refused the status change: %s", sanitize(firstMessage(out), b.secretsFor(b.current())...))
	}
	return nil
}

// activeEntities lists the entity IDs the API user's session can act in; GLPI nests them under
// active_entity, while older docs show them at the top level, so both are read.
func (b *v1) activeEntities(ctx context.Context) ([]int, error) {
	type ent struct {
		ID flexInt `json:"id"`
	}
	var out struct {
		ActiveEntity struct {
			ID       flexInt `json:"id"`
			Entities []ent   `json:"active_entities"`
		} `json:"active_entity"`
		Entities []ent `json:"active_entities"`
	}
	if _, err := b.call(ctx, http.MethodGet, "getActiveEntities", nil, &out); err != nil {
		return nil, err
	}
	var ids []int
	for _, e := range append(out.ActiveEntity.Entities, out.Entities...) {
		ids = append(ids, int(e.ID))
	}
	if len(ids) == 0 {
		ids = append(ids, int(out.ActiveEntity.ID))
	}
	return ids, nil
}
