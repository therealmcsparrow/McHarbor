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

const (
	headerTimestamp = "X-McHarbor-Timestamp"
	headerSignature = "X-McHarbor-Signature"
	maxSkew         = 300 * time.Second
)

var (
	// errStale rejects a Scrip callout whose timestamp is outside ±300s, which is what makes a
	// captured request useless for replay.
	errStale = errors.New("request_tracker: webhook timestamp outside the 300s window")
	// errBadSignature rejects a callout whose HMAC does not match; it says nothing about why.
	errBadSignature = errors.New("request_tracker: webhook signature mismatch")
	// errNoSecret fails closed when the channel has no inbound secret yet.
	errNoSecret = errors.New("request_tracker: no inbound webhook secret configured")
)

// VerifyWebhook checks McHarbor's own Scrip signing scheme, HMAC-SHA256 over ts + "." + body,
// because RT has no built-in webhook and therefore no vendor signature to verify.
func (c *Client) VerifyWebhook(raw map[string]string, header http.Header, body []byte) error {
	secret := raw["_webhook_secret"]
	if secret == "" {
		return errNoSecret
	}
	tsRaw := strings.TrimSpace(header.Get(headerTimestamp))
	ts, err := strconv.ParseInt(tsRaw, 10, 64)
	if err != nil {
		return errStale
	}
	skew := c.now().Sub(time.Unix(ts, 0))
	if skew > maxSkew || skew < -maxSkew {
		return errStale
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsRaw))
	mac.Write([]byte("."))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(strings.TrimSpace(header.Get(headerSignature)))) {
		return errBadSignature
	}
	return nil
}

// scripPayload is the JSON the documented Scrip posts on a status change.
type scripPayload struct {
	ID        rtID   `json:"id"`
	Status    string `json:"status"`
	OldStatus string `json:"old_status"`
	Owner     rtUser `json:"owner"`
	Queue     string `json:"queue"`
	DedupKey  string `json:"dedup_key"`
	Updated   string `json:"updated"`
}

// ParseWebhook maps a verified Scrip callout to one remote status; a ticket without the dedup
// custom field is not McHarbor's and is ignored rather than reported as an error.
func (c *Client) ParseWebhook(_ context.Context, raw map[string]string, _ http.Header, body []byte) ([]incident.RemoteStatus, error) {
	var p scripPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("request_tracker: decode scrip payload: %w", err)
	}
	if strings.TrimSpace(p.DedupKey) == "" {
		return nil, nil
	}
	id, err := ticketID(string(p.ID))
	if err != nil {
		return nil, err
	}
	statuses, err := statusTable(raw["status_map"])
	if err != nil {
		return nil, err
	}
	st, ok := statuses[strings.ToLower(strings.TrimSpace(p.Status))]
	if !ok {
		return nil, nil
	}
	ref := incident.Ref{ExternalID: id, Number: id}
	if base, err := baseURL(raw["instance_url"]); err == nil {
		ref.URL = base + "/Ticket/Display.html?id=" + id
	}
	note := ""
	if p.OldStatus != "" {
		note = sanitize(fmt.Sprintf("RT status changed from %s to %s", p.OldStatus, p.Status))
	}
	return []incident.RemoteStatus{{
		Ref:       ref,
		State:     st,
		Assignee:  sanitize(p.Owner.assignee()),
		Note:      note,
		UpdatedAt: parseRTTime(p.Updated),
	}}, nil
}

// parseRTTime reads REST2's and the Scrip's W3CDTF timestamps; a zero time lets the host stamp
// its own clock rather than reject the status.
func parseRTTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
