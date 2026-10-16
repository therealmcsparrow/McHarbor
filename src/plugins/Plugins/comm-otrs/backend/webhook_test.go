// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestParseWebhookShapes(t *testing.T) {
	cfg := testConfig("https://x")
	for file, want := range map[string]incident.RemoteStatus{
		"testdata/znuny_ticket_generic.json": {Ref: incident.Ref{ExternalID: "42", Number: "2026092510000017"}, DedupKey: "a1:e1:c1", State: incident.RemoteAcknowledged, Assignee: "jdoe"},
		"testdata/otobo_ticket_update.json":  {Ref: incident.Ref{ExternalID: "43", Number: "2026092510000018"}, DedupKey: "a1:e1:c2", State: incident.RemoteResolved, Assignee: "jdoe"},
	} {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		got, err := NewClient().ParseWebhook(context.Background(), cfg, http.Header{}, body)
		if err != nil || len(got) != 1 {
			t.Fatalf("%s: %v %v", file, got, err)
		}
		g := got[0]
		if g.Ref.ExternalID != want.Ref.ExternalID || g.Ref.Number != want.Ref.Number || g.DedupKey != want.DedupKey || g.State != want.State || g.Assignee != want.Assignee {
			t.Errorf("%s: got %+v want %+v", file, g, want)
		}
	}
	renamed := []byte(`{"Event":{"TicketID":"45"},"Ticket":{"TicketID":"45","State":"In Bearbeitung","StateType":"pending reminder"}}`)
	got, err := NewClient().ParseWebhook(context.Background(), cfg, http.Header{}, renamed)
	if err != nil || len(got) != 1 || got[0].State != incident.RemoteAcknowledged {
		t.Fatalf("a renamed state must map through StateType: %+v %v", got, err)
	}
}

func TestParseWebhookIgnoresStatelessPayload(t *testing.T) {
	got, err := NewClient().ParseWebhook(context.Background(), testConfig("https://x"), http.Header{}, []byte(`{"TicketID":"44","Ticket":{"Owner":"jdoe"}}`))
	if err != nil || len(got) != 0 {
		t.Fatalf("a payload without a state must not report one: %+v %v", got, err)
	}
}

func TestVerifyWebhookSecretHeader(t *testing.T) {
	cfg := testConfig("https://x")
	cfg["_webhook_secret"] = "s3cr3t-value"
	c := NewClient()
	ok := http.Header{"X-Mcharbor-Webhook-Secret": {"s3cr3t-value"}}
	if err := c.VerifyWebhook(cfg, ok, nil); err != nil {
		t.Fatal(err)
	}
	for _, h := range []http.Header{{}, {"X-Mcharbor-Webhook-Secret": {"s3cr3t-valuX"}}, {"X-Mcharbor-Webhook-Secret": {""}}, {"X-Mcharbor-Webhook-Secret": {"s3cr3t-value "}}} {
		if err := c.VerifyWebhook(cfg, h, nil); err == nil {
			t.Errorf("accepted %v", h)
		}
	}
	delete(cfg, "_webhook_secret")
	if err := c.VerifyWebhook(cfg, ok, nil); err == nil {
		t.Fatal("no secret configured must fail closed")
	}
	if err := c.VerifyWebhook(cfg, http.Header{}, nil); err == nil {
		t.Fatal("no secret and no header must fail closed")
	}
}
