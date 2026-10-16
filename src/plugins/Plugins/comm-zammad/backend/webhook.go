// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// errBadSignature is the single error every verification failure returns, so a caller probing
// the public route learns nothing about which check failed.
var errBadSignature = errors.New("comm-zammad: webhook signature rejected")

// VerifyWebhook checks Zammad's X-Hub-Signature, an HMAC-SHA1 of the raw body keyed with the
// channel's inbound secret; it fails closed when no secret is set and compares in constant time.
func (c *Client) VerifyWebhook(raw map[string]string, header http.Header, body []byte) error {
	secret := raw["_webhook_secret"]
	if secret == "" {
		return errBadSignature
	}
	sig := strings.TrimSpace(header.Get("X-Hub-Signature"))
	if !strings.HasPrefix(sig, "sha1=") {
		return errBadSignature
	}
	got, err := hex.DecodeString(sig[len("sha1="):])
	if err != nil {
		return errBadSignature
	}
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return errBadSignature
	}
	return nil
}

// webhookPayload is the part of Zammad's default trigger payload the plugin reads; a custom
// payload must carry at least these ticket fields.
type webhookPayload struct {
	Ticket *ticket `json:"ticket"`
}

// ParseWebhook turns a verified trigger payload into a status; tickets without the McHarbor
// marker are dropped silently because a broad trigger is the admin's choice, not an error.
func (c *Client) ParseWebhook(_ context.Context, raw map[string]string, _ http.Header, body []byte) ([]incident.RemoteStatus, error) {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("%w: webhook payload is not JSON", incident.ErrOperatorMessage)
	}
	if p.Ticket == nil || p.Ticket.ID == "" {
		return nil, fmt.Errorf("%w: webhook payload has no ticket id", incident.ErrOperatorMessage)
	}
	if !strings.HasPrefix(p.Ticket.Title, markerPrefix) {
		return nil, nil
	}
	table, err := statusTable(raw["status_map"])
	if err != nil {
		return nil, err
	}
	base, err := baseURL(raw)
	if err != nil {
		base = ""
	}
	return []incident.RemoteStatus{c.status(config{status: table}, base, *p.Ticket)}, nil
}
