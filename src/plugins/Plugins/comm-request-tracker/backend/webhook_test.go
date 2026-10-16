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

func sign(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(body)))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyScripSignature(t *testing.T) {
	c := NewClient(nil)
	now := time.Unix(1_790_000_000, 0)
	c.now = func() time.Time { return now }
	cfg := map[string]string{"_webhook_secret": "rtsecret"}
	body := []byte(`{"id":77,"status":"resolved","owner":"jdoe","dedup_key":"ab"}`)
	ts := strconv.FormatInt(now.Unix(), 10)
	h := http.Header{}
	h.Set("X-McHarbor-Timestamp", ts)
	h.Set("X-McHarbor-Signature", sign("rtsecret", ts, body))
	if err := c.VerifyWebhook(cfg, h, body); err != nil {
		t.Fatal(err)
	}
	if err := c.VerifyWebhook(cfg, h, []byte(`{"id":78}`)); err == nil {
		t.Fatal("tampered body accepted")
	}
	if err := c.VerifyWebhook(map[string]string{"_webhook_secret": "other"}, h, body); err == nil {
		t.Fatal("wrong secret accepted")
	}
	unkeyed := http.Header{}
	unkeyed.Set("X-McHarbor-Timestamp", ts)
	unkeyed.Set("X-McHarbor-Signature", sign("", ts, body))
	if err := c.VerifyWebhook(map[string]string{}, unkeyed, body); !errors.Is(err, errNoSecret) {
		t.Fatalf("a channel without a secret must fail closed, even for an empty-key HMAC: %v", err)
	}
	bodyOnly := http.Header{}
	bodyOnly.Set("X-McHarbor-Timestamp", ts)
	mac := hmac.New(sha256.New, []byte("rtsecret"))
	mac.Write(body)
	bodyOnly.Set("X-McHarbor-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	if err := c.VerifyWebhook(cfg, bodyOnly, body); err == nil {
		t.Fatal("a signature over the body alone must not verify; it is over ts + \".\" + body")
	}
	later := strconv.FormatInt(now.Add(time.Minute).Unix(), 10)
	h.Set("X-McHarbor-Timestamp", later)
	if err := c.VerifyWebhook(cfg, h, body); !errors.Is(err, errBadSignature) {
		t.Fatalf("a moved timestamp must break the signature: %v", err)
	}
	h.Set("X-McHarbor-Timestamp", strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10))
	if err := c.VerifyWebhook(cfg, h, body); !errors.Is(err, errStale) {
		t.Fatalf("stale: %v", err)
	}
	future := strconv.FormatInt(now.Add(10*time.Minute).Unix(), 10)
	h.Set("X-McHarbor-Timestamp", future)
	h.Set("X-McHarbor-Signature", sign("rtsecret", future, body))
	if err := c.VerifyWebhook(cfg, h, body); !errors.Is(err, errStale) {
		t.Fatalf("future: %v", err)
	}
	edge := strconv.FormatInt(now.Add(-299*time.Second).Unix(), 10)
	h.Set("X-McHarbor-Timestamp", edge)
	h.Set("X-McHarbor-Signature", sign("rtsecret", edge, body))
	if err := c.VerifyWebhook(cfg, h, body); err != nil {
		t.Fatalf("inside the window: %v", err)
	}
}

func TestParseWebhook(t *testing.T) {
	c := NewClient(nil)
	cfg := map[string]string{"instance_url": "https://rt.example.edu/rt"}
	body := []byte(`{"id":77,"status":"stalled","old_status":"open","owner":"jdoe","queue":"Operations","dedup_key":"ab","updated":"2026-09-25T10:00:00Z"}`)
	got, err := c.ParseWebhook(context.Background(), cfg, nil, body)
	if err != nil || len(got) != 1 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	st := got[0]
	if st.Ref.ExternalID != "77" || st.State != incident.RemoteAcknowledged || st.Assignee != "jdoe" ||
		st.Ref.URL != "https://rt.example.edu/rt/Ticket/Display.html?id=77" || !st.UpdatedAt.Equal(time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("status = %+v", st)
	}
	if got, err := c.ParseWebhook(context.Background(), cfg, nil, []byte(`{"id":77,"status":"resolved"}`)); err != nil || len(got) != 0 {
		t.Fatalf("a ticket without dedup_key must be ignored: %v %v", got, err)
	}
	if got, err := c.ParseWebhook(context.Background(), cfg, nil, []byte(`{"id":77,"status":"weird","dedup_key":"ab"}`)); err != nil || len(got) != 0 {
		t.Fatalf("an unknown status must be dropped: %v %v", got, err)
	}
	nobody := []byte(`{"id":"78","status":"rejected","owner":"Nobody","dedup_key":"ab"}`)
	if got, err := c.ParseWebhook(context.Background(), cfg, nil, nobody); err != nil || len(got) != 1 || got[0].Assignee != "" || got[0].State != incident.RemoteCancelled {
		t.Fatalf("nobody: %v %v", got, err)
	}
}

func TestPollOneQuery(t *testing.T) {
	var hits int
	srv, cfg := fakeRT(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if q := r.URL.Query().Get("query"); q != "id = 77 OR id = 78" {
			t.Errorf("query = %q", q)
		}
		w.Write([]byte(`{"total":2,"count":2,"page":1,"pages":1,"per_page":100,"items":[{"id":"77","Status":"stalled","Owner":{"id":"jdoe"}},{"id":"78","Status":"rejected","Owner":{"id":"Nobody"}}]}`))
	})
	got, err := NewClient(srv.Client()).Poll(context.Background(), cfg, []incident.Ref{{ExternalID: "77"}, {ExternalID: "78"}, {ExternalID: "1 OR id > 0"}})
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 || len(got) != 2 || got[0].State != incident.RemoteAcknowledged || got[0].Assignee != "jdoe" || got[1].State != incident.RemoteCancelled || got[1].Assignee != "" {
		t.Fatalf("hits=%d got=%+v", hits, got)
	}
}

func TestPollBatchesOf50(t *testing.T) {
	var queries []string
	srv, cfg := fakeRT(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("query"))
		w.Write([]byte(`{"items":[]}`))
	})
	refs := make([]incident.Ref, 120)
	for i := range refs {
		refs[i] = incident.Ref{ExternalID: strconv.Itoa(i + 1)}
	}
	if _, err := NewClient(srv.Client()).Poll(context.Background(), cfg, refs); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 || strings.Count(queries[0], " OR ") != 49 || strings.Count(queries[2], " OR ") != 19 {
		t.Fatalf("queries = %d", len(queries))
	}
}
