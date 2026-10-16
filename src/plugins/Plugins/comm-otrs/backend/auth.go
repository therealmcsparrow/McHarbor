// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

var errAuthFail = errors.New("comm-otrs: session rejected")

func (c *Client) sessionKey(cfg config) string {
	sum := sha256.Sum256([]byte(cfg.Endpoint + "\x00" + cfg.UserLogin + "\x00" + cfg.Password))
	return hex.EncodeToString(sum[:])
}

func (c *Client) dropSession(cfg config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, c.sessionKey(cfg))
}

func (c *Client) dropSessionIf(cfg config, stale string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.sessionKey(cfg)
	if c.sessions[key] == stale {
		delete(c.sessions, key)
	}
}

func (c *Client) session(ctx context.Context, cfg config) (string, error) {
	key := c.sessionKey(cfg)
	c.mu.Lock()
	id, ok := c.sessions[key]
	c.mu.Unlock()
	if ok {
		return id, nil
	}
	var out struct {
		SessionID   string `json:"SessionID"`
		AccessToken string `json:"AccessToken"`
	}
	body := map[string]any{"UserLogin": cfg.UserLogin, "Password": cfg.Password}
	if err := c.post(ctx, cfg, "POST", "/Session", body, &out); err != nil {
		if errors.Is(err, errAuthFail) {
			return "", fmt.Errorf("%w: OTRS rejected the configured agent login", incident.ErrOperatorMessage)
		}
		return "", err
	}
	id = out.SessionID
	if id == "" {
		id = out.AccessToken
	}
	if id == "" {
		return "", fmt.Errorf("%w: the session operation returned no SessionID", incident.ErrOperatorMessage)
	}
	c.mu.Lock()
	c.sessions[key] = id
	c.mu.Unlock()
	return id, nil
}

func (c *Client) call(ctx context.Context, cfg config, route, method string, body map[string]any, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		sid, err := c.session(ctx, cfg)
		if err != nil {
			return routeMissing(cfg, err)
		}
		body["SessionID"] = sid
		err = c.post(ctx, cfg, method, route, body, out)
		if !errors.Is(err, errAuthFail) {
			return routeMissing(cfg, err)
		}
		c.dropSessionIf(cfg, sid)
	}
	return fmt.Errorf("%w: OTRS rejected the session twice; check that the agent is valid", incident.ErrOperatorMessage)
}

func routeMissing(cfg config, err error) error {
	if errors.Is(err, errRouteMissing) {
		return fmt.Errorf("%w: web service %q not found; import McHarborConnectorREST.yml and check the product and script path", incident.ErrOperatorMessage, sanitize(cfg.Webservice))
	}
	return err
}
