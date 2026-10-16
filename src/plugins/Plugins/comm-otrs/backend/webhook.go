// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const secretHeader = "X-McHarbor-Webhook-Secret"

var errWebhookRejected = errors.New("comm-otrs: webhook rejected")

// VerifyWebhook compares the static header in constant time and fails closed without a secret;
// GenericInterface requesters cannot sign bodies, so this header is the only proof available.
func (c *Client) VerifyWebhook(m map[string]string, h http.Header, _ []byte) error {
	want := m["_webhook_secret"]
	if want == "" {
		return errWebhookRejected
	}
	got := h.Get(secretHeader)
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return errWebhookRejected
	}
	return nil
}

// ParseWebhook accepts both the Znuny Ticket::Generic and the OTOBO Ticket::TicketUpdate shapes,
// because the admin's product decides which invoker exists and both reach the same route.
func (c *Client) ParseWebhook(_ context.Context, m map[string]string, _ http.Header, body []byte) ([]incident.RemoteStatus, error) {
	cfg, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	var p map[string]any
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("comm-otrs: webhook body: %w", err)
	}
	ticket, _ := p["Ticket"].(map[string]any)
	event, _ := p["Event"].(map[string]any)
	id := firstNonEmpty(str(p["TicketID"]), str(ticket["TicketID"]), str(event["TicketID"]))
	if id == "" || ticket == nil {
		return nil, nil
	}
	st, ok := cfg.status(id, firstNonEmpty(str(p["TicketNumber"]), str(ticket["TicketNumber"])), ticket)
	if !ok {
		return nil, nil
	}
	return []incident.RemoteStatus{st}, nil
}
