// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func firing() incident.Event {
	return incident.Event{DedupKey: "a1:e1:c1", State: incident.StateFiring, Severity: incident.SeverityCritical,
		Title: "CPU high on web", Message: "92% for 5m", Link: "https://mcharbor.example/alerts/history?event=x"}
}

func TestOpenIsIdempotent(t *testing.T) {
	f, srv := newFakeGI(t)
	created := map[string]string{}
	f.handlers["POST /TicketSearch"] = func(b map[string]any) any {
		df, _ := b["DynamicField_McHarborDedupKey"].(map[string]any)
		for id, key := range created {
			if df["Equals"] == key {
				return map[string]any{"TicketID": []string{id}}
			}
		}
		return map[string]any{}
	}
	f.handlers["POST /TicketGet"] = func(b map[string]any) any {
		return map[string]any{"Ticket": []any{map[string]any{"TicketID": "42", "TicketNumber": "2026092510000017", "DynamicField_McHarborDedupKey": created["42"]}}}
	}
	f.handlers["POST /Ticket"] = func(b map[string]any) any {
		tk := b["Ticket"].(map[string]any)
		if tk["Priority"] != "5 very high" || tk["Queue"] != "Raw" {
			t.Errorf("ticket body %v", tk)
		}
		dfs, _ := b["DynamicField"].([]any)
		if len(dfs) != 1 || dfs[0].(map[string]any)["Value"] != "a1:e1:c1" {
			t.Errorf("dedup field not set on create: %v", b["DynamicField"])
		}
		created["42"] = "a1:e1:c1"
		return map[string]any{"TicketID": "42", "TicketNumber": "2026092510000017", "ArticleID": "1"}
	}
	c := NewClient()
	ctx := context.Background()
	first, err := c.Open(ctx, testConfig(srv.URL), firing())
	if err != nil {
		t.Fatal(err)
	}
	again, err := c.Open(ctx, testConfig(srv.URL), firing())
	if err != nil {
		t.Fatal(err)
	}
	if first.ExternalID != "42" || again.ExternalID != "42" || first.Number != "2026092510000017" {
		t.Fatalf("refs %+v %+v", first, again)
	}
	if n := f.count("POST /Ticket"); n != 1 {
		t.Fatalf("TicketCreate called %d times", n)
	}
	if first.URL != srv.URL+"/otrs/index.pl?Action=AgentTicketZoom;TicketID=42" {
		t.Fatalf("url %s", first.URL)
	}
}

func TestOpenVerifiesDedupFieldOnHit(t *testing.T) {
	f, srv := newFakeGI(t)
	f.handlers["POST /TicketSearch"] = func(map[string]any) any { return map[string]any{"TicketID": []string{"7"}} }
	f.handlers["POST /TicketGet"] = func(map[string]any) any {
		return map[string]any{"Ticket": []any{map[string]any{"TicketID": "7", "DynamicField_McHarborDedupKey": ""}}}
	}
	f.handlers["POST /Ticket"] = func(map[string]any) any { return map[string]any{"TicketID": "8", "TicketNumber": "N8"} }
	ref, err := NewClient().Open(context.Background(), testConfig(srv.URL), firing())
	if err != nil {
		t.Fatal(err)
	}
	if ref.ExternalID != "8" {
		t.Fatalf("reused unrelated ticket 7: %+v", ref)
	}
	if n := f.count("POST /TicketGet"); n != 1 {
		t.Fatalf("the search hit was not re-read: TicketGet calls = %d", n)
	}
}

func TestOpenReusesHitFromDynamicFieldArray(t *testing.T) {
	f, srv := newFakeGI(t)
	f.handlers["POST /TicketSearch"] = func(map[string]any) any { return map[string]any{"TicketID": "9"} }
	f.handlers["POST /TicketGet"] = func(map[string]any) any {
		return map[string]any{"Ticket": []any{map[string]any{"TicketID": "9", "TicketNumber": "N9",
			"DynamicField": []any{map[string]any{"Name": "Other", "Value": "a1:e1:c1"}, map[string]any{"Name": "McHarborDedupKey", "Value": "a1:e1:c1"}}}}}
	}
	ref, err := NewClient().Open(context.Background(), testConfig(srv.URL), firing())
	if err != nil || ref.ExternalID != "9" || ref.Number != "N9" {
		t.Fatalf("%+v %v", ref, err)
	}
	if n := f.count("POST /Ticket"); n != 0 {
		t.Fatalf("created a duplicate: %d", n)
	}
}

func TestResolveSetsCloseStateAndInternalArticle(t *testing.T) {
	f, srv := newFakeGI(t)
	var got map[string]any
	f.handlers["PATCH /Ticket/:id"] = func(b map[string]any) any { got = b; return map[string]any{"TicketID": "42", "ArticleID": "9"} }
	ev := firing()
	ev.State = incident.StateResolved
	if err := NewClient().Resolve(context.Background(), testConfig(srv.URL), incident.Ref{ExternalID: "42"}, ev); err != nil {
		t.Fatal(err)
	}
	f.locked(func() {
		tk := got["Ticket"].(map[string]any)
		art := got["Article"].(map[string]any)
		if tk["State"] != "closed successful" || art["CommunicationChannel"] != "Internal" || art["IsVisibleForCustomer"] != float64(0) {
			t.Fatalf("resolve body %v", got)
		}
	})
}

func TestUpdateIsNoteOnly(t *testing.T) {
	f, srv := newFakeGI(t)
	var got map[string]any
	f.handlers["PATCH /Ticket/:id"] = func(b map[string]any) any { got = b; return map[string]any{"TicketID": "42"} }
	ev := firing()
	ev.State = incident.StateUpdate
	if err := NewClient().Update(context.Background(), testConfig(srv.URL), incident.Ref{ExternalID: "42"}, ev); err != nil {
		t.Fatal(err)
	}
	f.locked(func() {
		if _, hasTicket := got["Ticket"]; hasTicket {
			t.Fatalf("update must not touch ticket attributes: %v", got)
		}
		art, _ := got["Article"].(map[string]any)
		if !strings.Contains(art["Body"].(string), "92% for 5m") {
			t.Fatalf("note body %v", art)
		}
	})
}

func TestUpdateDeletedTicketIsNotFound(t *testing.T) {
	f, srv := newFakeGI(t)
	f.handlers["PATCH /Ticket/:id"] = func(map[string]any) any {
		return map[string]any{"Error": map[string]any{"ErrorCode": "TicketUpdate.NotValidTicketID", "ErrorMessage": "gone"}}
	}
	err := NewClient().Update(context.Background(), testConfig(srv.URL), incident.Ref{ExternalID: "42"}, firing())
	if !errors.Is(err, incident.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
