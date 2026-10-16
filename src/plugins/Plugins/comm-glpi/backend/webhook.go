// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// maxSkew is the replay window: GLPI stamps every delivery, and a signed body older than this
// is refused even though its signature is valid.
const maxSkew = 300 * time.Second

// errStale marks a correctly formed delivery whose X-GLPI-timestamp is outside maxSkew.
var errStale = errors.New("comm-glpi: webhook timestamp outside the 5 minute window")

// errBadSignature covers a missing, malformed or wrong signature, and a missing secret.
var errBadSignature = errors.New("comm-glpi: webhook signature rejected")

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// VerifyWebhook checks GLPI 11's X-GLPI-signature, hex HMAC-SHA256 over body followed by the
// X-GLPI-timestamp string, in constant time; it fails closed when no secret is configured.
func (c *Client) VerifyWebhook(config map[string]string, header http.Header, body []byte) error {
	secret := firstNonEmpty(config["webhook_signing_secret"], config["_webhook_secret"])
	if secret == "" {
		return fmt.Errorf("%w: no secret configured", errBadSignature)
	}
	tsRaw := strings.TrimSpace(header.Get("X-GLPI-timestamp"))
	ts, err := strconv.ParseInt(tsRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: missing or malformed timestamp", errBadSignature)
	}
	skew := c.now().Sub(time.Unix(ts, 0))
	if skew > maxSkew || skew < -maxSkew {
		return errStale
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	mac.Write([]byte(tsRaw))
	want := hex.EncodeToString(mac.Sum(nil))
	got := strings.ToLower(strings.TrimSpace(header.Get("X-GLPI-signature")))
	if !hmac.Equal([]byte(want), []byte(got)) {
		return errBadSignature
	}
	return nil
}

type webhookPayload struct {
	Event string `json:"event"`
	Item  struct {
		ID         flexInt     `json:"id"`
		Status     statusField `json:"status"`
		ExternalID string      `json:"external_id"`
		IsDeleted  flexBool    `json:"is_deleted"`
		DateMod    string      `json:"date_mod"`
	} `json:"item"`
}

// ParseWebhook reads GLPI's default {"event","item"} payload. Tickets without the mcharbor:
// prefix are ignored without error, because an admin's webhook filter may be broader than ours.
func (c *Client) ParseWebhook(_ context.Context, config map[string]string, _ http.Header, body []byte) ([]incident.RemoteStatus, error) {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("comm-glpi: decode webhook: %w", err)
	}
	it := p.Item
	if !strings.HasPrefix(it.ExternalID, externalIDPrefix) || it.ID <= 0 {
		return nil, nil
	}
	deleted := bool(it.IsDeleted) || p.Event == "delete" || p.Event == "purge"
	state, ok := remoteState(int(it.Status), deleted)
	if !ok {
		return nil, nil
	}
	base, err := baseURL(config)
	if err != nil {
		return nil, err
	}
	updated := c.now()
	if t, err := time.Parse(time.RFC3339, it.DateMod); err == nil {
		updated = t
	}
	return []incident.RemoteStatus{{
		Ref:       refFor(base, int(it.ID)),
		State:     state,
		UpdatedAt: updated,
	}}, nil
}
