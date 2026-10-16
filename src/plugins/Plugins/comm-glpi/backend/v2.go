// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// tokenMargin is how long before expires_in a cached bearer token stops being reused, so a
// token never expires between the cache check and GLPI reading it.
const tokenMargin = 60 * time.Second

// v2 is the GLPI 11 high-level API behind a cached OAuth2 bearer token; close is a no-op.
type v2 struct {
	c     *Client
	cfg   glpiConfig
	mu    sync.Mutex
	token string
}

// tokenKey hashes every input of the password grant, so two channels share a token only when
// they would have received the same one, and no credential is held as a map key.
func tokenKey(cfg glpiConfig) string {
	h := sha256.New()
	for _, p := range []string{cfg.BaseURL, cfg.ClientID, cfg.ClientSecret, cfg.Username, cfg.Password} {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// bearer returns a cached token, or runs the password grant with scope api when there is none
// or force is set because GLPI rejected the cached one.
func (c *Client) bearer(ctx context.Context, cfg glpiConfig, force bool) (string, error) {
	key := tokenKey(cfg)
	now := c.now()
	c.mu.Lock()
	if t, ok := c.tokens[key]; ok && !force && now.Before(t.expires) {
		c.mu.Unlock()
		return t.token, nil
	}
	c.mu.Unlock()

	body := map[string]string{
		"grant_type":    "password",
		"client_id":     cfg.ClientID,
		"client_secret": cfg.ClientSecret,
		"username":      cfg.Username,
		"password":      cfg.Password,
		"scope":         "api",
	}
	var out struct {
		AccessToken string  `json:"access_token"`
		ExpiresIn   flexInt `json:"expires_in"`
	}
	if _, err := c.do(ctx, http.MethodPost, cfg.BaseURL+"/api.php/token", nil, body, &out, cfg.secrets()...); err != nil {
		if errors.Is(err, incident.ErrNotFound) {
			return "", operatorError("GLPI has no high-level API at /api.php; API v2 needs GLPI 11 or later")
		}
		return "", fmt.Errorf("comm-glpi: token: %w", err)
	}
	if out.AccessToken == "" {
		return "", operatorError("GLPI returned no access token; check the OAuth client's password grant and api scope")
	}
	expires := now.Add(time.Duration(out.ExpiresIn)*time.Second - tokenMargin)
	c.mu.Lock()
	for k, t := range c.tokens {
		if !now.Before(t.expires) {
			delete(c.tokens, k)
		}
	}
	if expires.After(now) {
		c.tokens[key] = cachedToken{token: out.AccessToken, expires: expires}
	} else {
		delete(c.tokens, key)
	}
	c.mu.Unlock()
	return out.AccessToken, nil
}

func (c *Client) openV2(ctx context.Context, cfg glpiConfig) (*v2, error) {
	token, err := c.bearer(ctx, cfg, false)
	if err != nil {
		return nil, err
	}
	return &v2{c: c, cfg: cfg, token: token}, nil
}

func (b *v2) close() error { return nil }

func (b *v2) current() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.token
}

// renew replaces a token GLPI rejected; concurrent poll workers that saw the same rejected
// token share one new grant.
func (b *v2) renew(ctx context.Context, stale string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.token != stale {
		return b.token, nil
	}
	token, err := b.c.bearer(ctx, b.cfg, true)
	if err != nil {
		return "", err
	}
	b.token = token
	return token, nil
}

func (b *v2) send(ctx context.Context, method, target, token string, body, out any) (result, error) {
	return b.c.do(ctx, method, target, http.Header{"Authorization": {"Bearer " + token}}, body, out, append(b.cfg.secrets(), token)...)
}

// call sends one HL API request and, on a 401, fetches a fresh token and replays once, because
// GLPI may revoke a token before the expires_in the cache trusted.
func (b *v2) call(ctx context.Context, method, rel string, body, out any) (result, error) {
	target := b.cfg.BaseURL + "/api.php/" + b.cfg.V2Path + rel
	token := b.current()
	res, err := b.send(ctx, method, target, token, body, out)
	if !errors.Is(err, errUnauthorized) {
		return res, err
	}
	token, err = b.renew(ctx, token)
	if err != nil {
		return result{}, err
	}
	return b.send(ctx, method, target, token, body, out)
}

type v2Ticket struct {
	ID         flexInt     `json:"id"`
	Name       string      `json:"name"`
	Status     statusField `json:"status"`
	ExternalID string      `json:"external_id"`
	IsDeleted  flexBool    `json:"is_deleted"`
	DateMod    string      `json:"date_mod"`
}

func (r v2Ticket) ticket() ticket {
	return ticket{ID: int(r.ID), Name: r.Name, Status: int(r.Status), ExternalID: r.ExternalID, Deleted: bool(r.IsDeleted), Modified: r.DateMod}
}

// findOpenByExternalID filters server-side with RSQL and still compares exactly, so a server
// that ignores or loosens the filter cannot make Open reuse the wrong ticket.
func (b *v2) findOpenByExternalID(ctx context.Context, ext string) (ticket, bool, error) {
	q := url.Values{}
	q.Set("filter", "external_id=="+ext+";status.id=lt=5")
	q.Set("limit", "10")
	var rows []v2Ticket
	if _, err := b.call(ctx, http.MethodGet, "/Assistance/Ticket?"+q.Encode(), nil, &rows); err != nil {
		if errors.Is(err, incident.ErrNotFound) {
			return ticket{}, false, operatorError("GLPI answered 404 to the ticket search; check the HL API version path")
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

func (b *v2) create(ctx context.Context, t ticketInput) (int, error) {
	var out struct {
		ID flexInt `json:"id"`
	}
	res, err := b.call(ctx, http.MethodPost, "/Assistance/Ticket", t.v2Map(), &out)
	if err != nil {
		if errors.Is(err, incident.ErrNotFound) {
			return 0, operatorError("GLPI answered 404 to the ticket create; check the HL API version path")
		}
		return 0, err
	}
	id := int(out.ID)
	if id <= 0 {
		if loc := res.header.Get("Location"); loc != "" {
			if n, err := strconv.Atoi(path.Base(loc)); err == nil {
				id = n
			}
		}
	}
	if id <= 0 {
		return 0, operatorError("GLPI returned no ticket id")
	}
	return id, nil
}

func (b *v2) get(ctx context.Context, id int) (ticket, error) {
	var r v2Ticket
	if _, err := b.call(ctx, http.MethodGet, "/Assistance/Ticket/"+strconv.Itoa(id), nil, &r); err != nil {
		return ticket{}, err
	}
	return r.ticket(), nil
}

func (b *v2) followup(ctx context.Context, id int, content string) error {
	_, err := b.call(ctx, http.MethodPost, "/Assistance/Ticket/"+strconv.Itoa(id)+"/Timeline/Followup",
		map[string]any{"content": content, "is_private": true}, nil)
	return err
}

func (b *v2) solve(ctx context.Context, id int, content string) error {
	_, err := b.call(ctx, http.MethodPost, "/Assistance/Ticket/"+strconv.Itoa(id)+"/Timeline/Solution",
		map[string]any{"content": content}, nil)
	return err
}

func (b *v2) setStatus(ctx context.Context, id, status int) error {
	_, err := b.call(ctx, http.MethodPatch, "/Assistance/Ticket/"+strconv.Itoa(id),
		map[string]any{"status": map[string]int{"id": status}}, nil)
	return err
}
