// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func sign(secret string, body []byte, ts string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(append(append([]byte{}, body...), ts...))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhookHMACAndSkew(t *testing.T) {
	c := NewClient(nil)
	now := time.Unix(1_790_000_000, 0)
	c.now = func() time.Time { return now }
	cfg := map[string]string{"webhook_signing_secret": "glpisecret"}
	body := []byte(`{"event":"update","item":{"id":42,"status":{"id":5,"name":"Solved"},"external_id":"mcharbor:ab"}}`)
	ts := strconv.FormatInt(now.Unix(), 10)
	good := sign("glpisecret", body, ts)

	h := http.Header{"X-Glpi-Signature": {good}, "X-Glpi-Timestamp": {ts}}
	if err := c.VerifyWebhook(cfg, h, body); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := c.VerifyWebhook(cfg, http.Header{"X-Glpi-Signature": {strings.ToUpper(good)}, "X-Glpi-Timestamp": {ts}}, body); err != nil {
		t.Fatalf("upper-case hex rejected: %v", err)
	}
	if err := c.VerifyWebhook(cfg, h, append(body, ' ')); err == nil {
		t.Fatal("tampered body accepted")
	}
	if err := c.VerifyWebhook(map[string]string{"webhook_signing_secret": "other"}, h, body); err == nil {
		t.Fatal("wrong secret accepted")
	}
	if err := c.VerifyWebhook(map[string]string{"_webhook_secret": "glpisecret"}, h, body); err != nil {
		t.Fatalf("host secret fallback rejected: %v", err)
	}
	if err := c.VerifyWebhook(map[string]string{"webhook_signing_secret": "glpisecret", "_webhook_secret": "hostsecret"}, h, body); err != nil {
		t.Fatalf("operator secret must override the host one: %v", err)
	}
	emptyKey := http.Header{"X-Glpi-Signature": {sign("", body, ts)}, "X-Glpi-Timestamp": {ts}}
	if err := c.VerifyWebhook(map[string]string{"_webhook_secret": " "}, emptyKey, body); err == nil {
		t.Fatal("no secret configured but a signature with an empty key was accepted")
	}
	if err := c.VerifyWebhook(cfg, http.Header{"X-Glpi-Signature": {good}}, body); err == nil {
		t.Fatal("missing timestamp accepted")
	}
	shifted := strconv.FormatInt(now.Add(time.Minute).Unix(), 10)
	if err := c.VerifyWebhook(cfg, http.Header{"X-Glpi-Signature": {good}, "X-Glpi-Timestamp": {shifted}}, body); err == nil {
		t.Fatal("signature not bound to the timestamp")
	}
	h.Set("X-Glpi-Timestamp", strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10))
	if err := c.VerifyWebhook(cfg, h, body); !errors.Is(err, errStale) {
		t.Fatalf("stale timestamp: %v", err)
	}
	future := strconv.FormatInt(now.Add(6*time.Minute).Unix(), 10)
	h = http.Header{"X-Glpi-Signature": {sign("glpisecret", body, future)}, "X-Glpi-Timestamp": {future}}
	if err := c.VerifyWebhook(cfg, h, body); !errors.Is(err, errStale) {
		t.Fatalf("future timestamp: %v", err)
	}
}

func TestParseWebhookIgnoresForeignTickets(t *testing.T) {
	c := NewClient(nil)
	got, err := c.ParseWebhook(context.Background(), map[string]string{"instance_url": "https://glpi"}, nil,
		[]byte(`{"event":"update","item":{"id":7,"status":{"id":2},"external_id":""}}`))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v err %v", got, err)
	}
	got, err = c.ParseWebhook(context.Background(), map[string]string{"instance_url": "https://glpi"}, nil,
		[]byte(`{"event":"update","item":{"id":42,"status":{"id":2},"external_id":"mcharbor:ab"}}`))
	if err != nil || len(got) != 1 || got[0].State != incident.RemoteAcknowledged || got[0].Ref.ExternalID != "42" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if got[0].Ref.URL != "https://glpi/front/ticket.form.php?id=42" || got[0].DedupKey != "" {
		t.Fatalf("ref %+v", got[0])
	}
}

func TestParseWebhookStates(t *testing.T) {
	c := NewClient(nil)
	cases := map[string]incident.RemoteState{
		`{"event":"update","item":{"id":1,"status":{"id":1},"external_id":"mcharbor:a"}}`:                   incident.RemoteOpen,
		`{"event":"update","item":{"id":1,"status":{"id":4},"external_id":"mcharbor:a"}}`:                   incident.RemoteAcknowledged,
		`{"event":"update","item":{"id":1,"status":{"id":5},"external_id":"mcharbor:a"}}`:                   incident.RemoteResolved,
		`{"event":"update","item":{"id":1,"status":{"id":6},"external_id":"mcharbor:a"}}`:                   incident.RemoteClosed,
		`{"event":"update","item":{"id":1,"status":{"id":2},"external_id":"mcharbor:a","is_deleted":true}}`: incident.RemoteCancelled,
		`{"event":"purge","item":{"id":1,"status":{"id":2},"external_id":"mcharbor:a"}}`:                    incident.RemoteCancelled,
	}
	for body, want := range cases {
		got, err := c.ParseWebhook(context.Background(), map[string]string{"instance_url": "https://glpi"}, nil, []byte(body))
		if err != nil || len(got) != 1 || got[0].State != want {
			t.Errorf("%s: got %+v err %v", body, got, err)
		}
	}
	if _, err := c.ParseWebhook(context.Background(), map[string]string{"instance_url": "https://glpi"}, nil, []byte(`{`)); err == nil {
		t.Error("malformed body accepted")
	}
}
