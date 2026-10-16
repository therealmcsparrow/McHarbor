// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestVerifyHubSignature(t *testing.T) {
	c := NewClient(nil)
	cfg := map[string]string{"_webhook_secret": "zsecret"}
	body := []byte(`{"ticket":{"id":10,"number":"31010","title":"[MH-abcdef0123] x","state":"closed","owner":{"login":"anna"}}}`)
	mac := hmac.New(sha1.New, []byte("zsecret"))
	mac.Write(body)
	sum := hex.EncodeToString(mac.Sum(nil))
	h := http.Header{"X-Hub-Signature": {"sha1=" + sum}}
	if err := c.VerifyWebhook(cfg, h, body); err != nil {
		t.Fatal(err)
	}
	reencoded := bytes.ReplaceAll(body, []byte(`":`), []byte(`": `))
	if err := c.VerifyWebhook(cfg, h, reencoded); err == nil {
		t.Fatal("signature must cover the raw bytes")
	}
	if err := c.VerifyWebhook(map[string]string{"_webhook_secret": "other"}, h, body); err == nil {
		t.Fatal("wrong secret accepted")
	}
	empty := hmac.New(sha1.New, nil)
	empty.Write(body)
	unkeyed := http.Header{"X-Hub-Signature": {"sha1=" + hex.EncodeToString(empty.Sum(nil))}}
	if err := c.VerifyWebhook(map[string]string{}, unkeyed, body); err == nil {
		t.Fatal("empty secret must fail closed")
	}
	h.Set("X-Hub-Signature", "sha256="+sum)
	if err := c.VerifyWebhook(cfg, h, body); err == nil {
		t.Fatal("wrong algorithm prefix accepted")
	}
	h.Set("X-Hub-Signature", "sha1="+sum[:len(sum)-2])
	if err := c.VerifyWebhook(cfg, h, body); err == nil {
		t.Fatal("truncated signature accepted")
	}
	h.Del("X-Hub-Signature")
	if err := c.VerifyWebhook(cfg, h, body); err == nil {
		t.Fatal("missing signature accepted")
	}
}

func TestParseWebhookMerged(t *testing.T) {
	got, err := NewClient(nil).ParseWebhook(context.Background(), map[string]string{"instance_url": "https://z"}, nil,
		[]byte(`{"ticket":{"id":10,"number":"31010","title":"[MH-abcdef0123] x","state":"merged","owner":{"login":"-"}}}`))
	if err != nil || len(got) != 1 || got[0].State != incident.RemoteClosed || got[0].Assignee != "" || got[0].Ref.URL != "https://z/#ticket/zoom/10" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if !got[0].State.Terminal() || got[0].Ref.ExternalID != "10" || got[0].Ref.Number != "31010" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestParseWebhookStatesAndFilter(t *testing.T) {
	c := NewClient(nil)
	cfg := map[string]string{"instance_url": "https://z", "status_map": `{"Waiting for vendor":"open"}`}
	cases := map[string]incident.RemoteState{
		"new": incident.RemoteOpen, "open": incident.RemoteAcknowledged, "pending reminder": incident.RemoteAcknowledged,
		"pending close": incident.RemoteResolved, "closed": incident.RemoteClosed, "custom thing": incident.RemoteAcknowledged,
		"waiting for vendor": incident.RemoteOpen,
	}
	for state, want := range cases {
		body := `{"ticket":{"id":"10","number":31010,"title":"[MH-abcdef0123] x","state":"` + state + `","owner":{"login":"anna"},"updated_at":"2026-09-25T12:00:00.123Z"}}`
		got, err := c.ParseWebhook(context.Background(), cfg, nil, []byte(body))
		if err != nil || len(got) != 1 || got[0].State != want || got[0].Assignee != "anna" || got[0].Ref.Number != "31010" || got[0].UpdatedAt.IsZero() {
			t.Errorf("%s: got %+v err %v", state, got, err)
		}
	}
	got, err := c.ParseWebhook(context.Background(), cfg, nil, []byte(`{"ticket":{"id":5,"title":"Printer jam","state":"closed"}}`))
	if err != nil || len(got) != 0 {
		t.Fatalf("unmarked ticket: %+v %v", got, err)
	}
	if _, err := c.ParseWebhook(context.Background(), cfg, nil, []byte(`{"article":{}}`)); err == nil {
		t.Fatal("payload without ticket accepted")
	}
}

func TestPollOmitsDeletedAndKeepsPartial(t *testing.T) {
	var inflight, peak int32
	srv, cfg := fakeZammad(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inflight, 1)
		defer atomic.AddInt32(&inflight, -1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		if r.URL.Query().Get("expand") != "true" {
			t.Errorf("expand missing: %s", r.URL)
		}
		switch r.URL.Path {
		case "/api/v1/tickets/404":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v1/tickets/500":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/tickets/")
			write(t, w, `{"id":`+id+`,"number":"3`+id+`","title":"[MH-abcdef0123] x","state":"closed","owner":"anna"}`)
		}
	})
	refs := []incident.Ref{{ExternalID: "404"}, {ExternalID: "500"}}
	for _, id := range []string{"1", "2", "3", "4", "5", "6"} {
		refs = append(refs, incident.Ref{ExternalID: id})
	}
	got, err := NewClient(srv.Client()).Poll(context.Background(), cfg, refs)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, st := range got {
		if st.State != incident.RemoteClosed || st.Assignee != "anna" {
			t.Errorf("status %+v", st)
		}
		ids = append(ids, st.Ref.ExternalID)
	}
	sort.Strings(ids)
	if strings.Join(ids, ",") != "1,2,3,4,5,6" {
		t.Fatalf("ids = %v", ids)
	}
	if peak > pollConcurrency {
		t.Fatalf("peak concurrency %d", peak)
	}
	if _, err := NewClient(srv.Client()).Poll(context.Background(), cfg, []incident.Ref{{ExternalID: "500"}}); err == nil {
		t.Fatal("an all-failed poll must return the error")
	}
}
