// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const ackBody = `{"event":{"id":"01DEN","event_type":"incident.acknowledged","resource_type":"incident","occurred_at":"2026-09-25T11:58:00.000Z","agent":{"summary":"Jane Doe"},"data":{"id":"PGR0VU2","type":"incident","number":42,"status":"acknowledged","incident_key":"a1:e1:c1","html_url":"https://acme.pagerduty.com/incidents/PGR0VU2","assignees":[{"summary":"Jane Doe"}]}}}`

func sign(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "v1=" + hex.EncodeToString(m.Sum(nil))
}

func TestVerifyWebhookMultipleSignatures(t *testing.T) {
	c := NewClient()
	cfg := baseCfg()
	cfg["webhook_signing_secret"] = "new-secret"
	h := http.Header{}
	h.Set("X-PagerDuty-Signature", sign("old-secret", ackBody)+","+sign("new-secret", ackBody))
	if err := c.VerifyWebhook(cfg, h, []byte(ackBody)); err != nil {
		t.Fatalf("second valid signature rejected: %v", err)
	}
	h.Set("X-PagerDuty-Signature", "v2="+sign("new-secret", ackBody)[3:])
	if err := c.VerifyWebhook(cfg, h, []byte(ackBody)); err == nil {
		t.Fatal("v2 signature must not be accepted")
	}
	h.Set("X-PagerDuty-Signature", sign("new-secret", ackBody))
	if err := c.VerifyWebhook(cfg, h, []byte(ackBody+" ")); err == nil {
		t.Fatal("signature over a different body accepted")
	}
	delete(cfg, "webhook_signing_secret")
	h.Set("X-PagerDuty-Signature", sign("", ackBody))
	if err := c.VerifyWebhook(cfg, h, []byte(ackBody)); err == nil {
		t.Fatal("empty secret must fail closed")
	}
}

func TestVerifyWebhookSecretPrecedence(t *testing.T) {
	c := NewClient()
	cfg := baseCfg()
	cfg["_webhook_secret"] = "host-secret"
	h := http.Header{}
	h.Set("X-PagerDuty-Signature", sign("host-secret", ackBody))
	if err := c.VerifyWebhook(cfg, h, []byte(ackBody)); err != nil {
		t.Fatalf("host secret fallback rejected: %v", err)
	}
	cfg["webhook_signing_secret"] = "vendor-secret"
	if err := c.VerifyWebhook(cfg, h, []byte(ackBody)); err == nil {
		t.Fatal("vendor secret must take precedence over the host secret")
	}
}

func TestParseWebhookCarriesOccurredAt(t *testing.T) {
	st, err := NewClient().ParseWebhook(context.Background(), baseCfg(), http.Header{}, []byte(ackBody))
	if err != nil || len(st) != 1 {
		t.Fatalf("parse: %v %v", st, err)
	}
	s := st[0]
	if s.State != incident.RemoteAcknowledged || s.DedupKey != "a1:e1:c1" || s.Ref.ExternalID != "a1:e1:c1" || s.Ref.Number != "#42" || s.Assignee != "Jane Doe" {
		t.Fatalf("bad status %+v", s)
	}
	if !s.UpdatedAt.Equal(time.Date(2026, 9, 25, 11, 58, 0, 0, time.UTC)) {
		t.Fatalf("UpdatedAt = %v", s.UpdatedAt)
	}
}

func TestParseWebhookIgnoresNonIncidentAndUnknownStatus(t *testing.T) {
	bodies := []string{
		`{"event":{"event_type":"pagey.ping","resource_type":"pagey","data":{"type":"ping"}}}`,
		`{"event":{"event_type":"incident.priority_updated","resource_type":"incident","data":{"status":"weird","incident_key":"k"}}}`,
		`{"event":{"event_type":"incident.triggered","resource_type":"incident","data":{"status":"triggered"}}}`,
	}
	for _, b := range bodies {
		st, err := NewClient().ParseWebhook(context.Background(), baseCfg(), http.Header{}, []byte(b))
		if err != nil || len(st) != 0 {
			t.Errorf("body %s: %v %v", b, st, err)
		}
	}
	st, err := NewClient().ParseWebhook(context.Background(), baseCfg(), http.Header{},
		[]byte(`{"event":{"event_type":"incident.reopened","resource_type":"incident","data":{"status":"triggered","incident_key":"k"}}}`))
	if err != nil || len(st) != 1 || st[0].State != incident.RemoteOpen {
		t.Fatalf("reopened: %v %v", st, err)
	}
}
