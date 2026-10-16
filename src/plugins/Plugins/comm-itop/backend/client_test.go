// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const objStatusNew = `{"code":0,"message":"","objects":{"Incident::42":{"code":0,"class":"Incident","key":"42","fields":{"id":"42","ref":"I-000042","status":"new"}}}}`

func TestResolveFromNewAssignsFirst(t *testing.T) {
	srv, cfg, calls := fakeITop(t, func(c restCall) string {
		switch c.Operation {
		case "core/get":
			return objStatusNew
		default:
			return `{"code":0,"message":"","objects":{"Incident::42":{"code":0,"class":"Incident","key":"42","fields":{"status":"x"}}}}`
		}
	})
	err := NewClient(srv.Client()).Resolve(context.Background(), cfg, incident.Ref{ExternalID: "42"},
		incident.Event{State: incident.StateResolved, Title: "ok", Message: "recovered"})
	if err != nil {
		t.Fatal(err)
	}
	got := *calls
	if len(got) != 3 || got[1].Stimulus != "ev_assign" || got[2].Stimulus != "ev_resolve" {
		t.Fatalf("calls = %+v", got)
	}
	if got[1].Fields["team_id"] != float64(18) || got[1].Fields["agent_id"] != float64(57) {
		t.Fatalf("assign fields = %v", got[1].Fields)
	}
	f := got[2].Fields
	solution, _ := f["solution"].(string)
	if f["service_id"] != float64(3) || f["resolution_code"] != "other" || !strings.Contains(solution, "recovered") {
		t.Fatalf("resolve fields = %v", f)
	}
}

func TestResolveFromAssignedSkipsAssignAndCloses(t *testing.T) {
	srv, cfg, calls := fakeITop(t, func(c restCall) string {
		if c.Operation == "core/get" {
			return strings.Replace(objStatusNew, `"new"`, `"assigned"`, 1)
		}
		return `{"code":0,"objects":{"Incident::42":{"code":0,"class":"Incident","key":"42","fields":{}}}}`
	})
	cfg["close_on_resolve"] = "true"
	if err := NewClient(srv.Client()).Resolve(context.Background(), cfg, incident.Ref{ExternalID: "42"}, incident.Event{Title: "ok"}); err != nil {
		t.Fatal(err)
	}
	got := *calls
	if len(got) != 3 || got[1].Stimulus != "ev_resolve" || got[2].Stimulus != "ev_close" {
		t.Fatalf("calls = %+v", got)
	}
}

func TestResolveIdempotent(t *testing.T) {
	for _, status := range []string{"resolved", "closed"} {
		srv, cfg, calls := fakeITop(t, func(restCall) string {
			return `{"code":0,"message":"","objects":{"Incident::42":{"code":0,"class":"Incident","key":"42","fields":{"id":"42","status":"` + status + `"}}}}`
		})
		cfg["close_on_resolve"] = "true"
		if err := NewClient(srv.Client()).Resolve(context.Background(), cfg, incident.Ref{ExternalID: "42"}, incident.Event{State: incident.StateResolved, Title: "ok"}); err != nil {
			t.Fatal(err)
		}
		if len(*calls) != 1 || (*calls)[0].Operation != "core/get" {
			t.Fatalf("%s: a stimulus was applied to a finished ticket: %+v", status, *calls)
		}
	}
}

func TestResolveDeletedTicketIsNotFound(t *testing.T) {
	srv, cfg, _ := fakeITop(t, func(restCall) string { return `{"code":0,"message":"Found: 0","objects":null}` })
	err := NewClient(srv.Client()).Resolve(context.Background(), cfg, incident.Ref{ExternalID: "42"}, incident.Event{Title: "ok"})
	if !errors.Is(err, incident.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveRejectsNonNumericRef(t *testing.T) {
	srv, cfg, calls := fakeITop(t, func(restCall) string { return objStatusNew })
	err := NewClient(srv.Client()).Resolve(context.Background(), cfg, incident.Ref{ExternalID: "42 OR 1=1"}, incident.Event{Title: "ok"})
	if !errors.Is(err, incident.ErrOperatorMessage) || len(*calls) != 0 {
		t.Fatalf("err = %v calls = %d", err, len(*calls))
	}
}

func TestOpenReusesMarkerTicket(t *testing.T) {
	created := false
	srv, cfg, calls := fakeITop(t, func(c restCall) string {
		switch c.Operation {
		case "core/get":
			key := c.Key.(string)
			if !strings.Contains(key, "title LIKE '%[MH-") || !strings.Contains(key, "status NOT IN ('resolved','closed'") {
				t.Errorf("oql = %s", key)
			}
			if !created {
				return `{"code":0,"message":"Found: 0","objects":null}`
			}
			m := key[strings.Index(key, "[MH-") : strings.Index(key, "]%")+1]
			return `{"code":0,"message":"","objects":{"Incident::43":{"code":0,"class":"Incident","key":"43","fields":{"id":"43","ref":"I-000043","status":"new","title":"` + m + ` Disque plein"}}}}`
		case "core/create":
			created = true
			if c.Fields["impact"] != "1" || c.Fields["urgency"] != "1" || c.Fields["priority"] != nil {
				t.Errorf("fields = %v", c.Fields)
			}
			if c.Fields["org_id"] != "SELECT Organization WHERE name = 'Demo'" || !strings.HasPrefix(c.Fields["title"].(string), "[MH-") {
				t.Errorf("fields = %v", c.Fields)
			}
			return `{"code":0,"message":"","objects":{"Incident::43":{"code":0,"class":"Incident","key":"43","fields":{"id":"43","ref":"I-000043","status":"new"}}}}`
		}
		return `{"code":0}`
	})
	c := NewClient(srv.Client())
	ev := incident.Event{DedupKey: "a:e:s", State: incident.StateFiring, Severity: incident.SeverityCritical, Title: "Disque plein", Message: "<script>x</script>", OccurredAt: time.Now()}
	a, err := c.Open(context.Background(), cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Open(context.Background(), cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	creates := 0
	for _, call := range *calls {
		if call.Operation == "core/create" {
			creates++
			if strings.Contains(call.Fields["description"].(string), "<script>") {
				t.Errorf("description not escaped: %v", call.Fields["description"])
			}
		}
	}
	if creates != 1 || a.ExternalID != "43" || b.ExternalID != "43" || a.Number != "I-000043" {
		t.Fatalf("creates=%d a=%+v b=%+v", creates, a, b)
	}
	if a.URL != srv.URL+"/pages/UI.php?operation=details&class=Incident&id=43" {
		t.Fatalf("url = %s", a.URL)
	}
}

func TestOpenReopensWithinWindow(t *testing.T) {
	srv, cfg, calls := fakeITop(t, func(c restCall) string {
		switch {
		case c.Operation == "core/get" && strings.Contains(c.Key.(string), "status = 'resolved'"):
			key := c.Key.(string)
			if !strings.Contains(key, "INTERVAL 30 MINUTE") {
				t.Errorf("oql = %s", key)
			}
			m := key[strings.Index(key, "[MH-") : strings.Index(key, "]%")+1]
			return `{"code":0,"objects":{"Incident::9":{"code":0,"class":"Incident","key":"9","fields":{"id":"9","ref":"I-9","status":"resolved","title":"` + m + ` x"}}}}`
		case c.Operation == "core/get":
			return `{"code":0,"objects":null}`
		}
		return `{"code":0,"objects":{"Incident::9":{"code":0,"class":"Incident","key":"9","fields":{}}}}`
	})
	cfg["reopen_window_minutes"] = "30"
	ref, err := NewClient(srv.Client()).Open(context.Background(), cfg, incident.Event{DedupKey: "k", State: incident.StateFiring, Severity: incident.SeverityInfo, Title: "x"})
	if err != nil || ref.ExternalID != "9" {
		t.Fatalf("ref=%+v err=%v", ref, err)
	}
	last := (*calls)[len(*calls)-1]
	if last.Stimulus != "ev_reopen" {
		t.Fatalf("calls = %+v", *calls)
	}
}

func TestUpdateDeletedTicketIsNotFound(t *testing.T) {
	srv, cfg, calls := fakeITop(t, func(c restCall) string {
		if c.Operation == "core/update" {
			return `{"code":100,"message":"Error: Invalid object Incident::42"}`
		}
		return `{"code":0,"objects":null}`
	})
	err := NewClient(srv.Client()).Update(context.Background(), cfg, incident.Ref{ExternalID: "42"}, incident.Event{Title: "again", Message: "m"})
	if !errors.Is(err, incident.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	log, _ := (*calls)[0].Fields["private_log"].(map[string]any)
	item, _ := log["add_item"].(map[string]any)
	if item["format"] != "text" || !strings.Contains(item["message"].(string), "again") {
		t.Fatalf("private_log = %v", (*calls)[0].Fields)
	}
}

func TestTestValidatesAssignmentPair(t *testing.T) {
	srv, cfg, _ := fakeITop(t, func(c restCall) string {
		if c.Class == "lnkPersonToTeam" {
			return `{"code":0,"message":"Found: 0","objects":null}`
		}
		return `{"code":0,"objects":{"X::1":{"code":0,"class":"X","key":"1","fields":{"id":"1"}}}}`
	})
	err := NewClient(srv.Client()).Test(context.Background(), cfg)
	if !errors.Is(err, incident.ErrOperatorMessage) || !strings.Contains(err.Error(), "team 18") {
		t.Fatalf("err = %v", err)
	}
}

func TestTestNoCreate(t *testing.T) {
	srv, cfg, calls := fakeITop(t, func(restCall) string {
		return `{"code":0,"objects":{"X::1":{"code":0,"class":"X","key":"1","fields":{"id":"1"}}}}`
	})
	cfg["caller_email"] = "ops@example.fr"
	if err := NewClient(srv.Client()).Test(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	classes := map[string]bool{}
	for _, c := range *calls {
		if c.Operation != "core/get" && c.Operation != "list_operations" {
			t.Fatalf("Test issued %s", c.Operation)
		}
		classes[c.Class] = true
	}
	for _, want := range []string{"Organization", "Person", "Team", "Service", "lnkPersonToTeam"} {
		if !classes[want] {
			t.Errorf("Test never checked %s", want)
		}
	}
}

func TestTestOrganizationMustBeUnique(t *testing.T) {
	srv, cfg, _ := fakeITop(t, func(c restCall) string {
		if c.Class == "Organization" {
			return `{"code":0,"objects":{"Organization::1":{"code":0,"key":"1"},"Organization::2":{"code":0,"key":"2"}}}`
		}
		return `{"code":0}`
	})
	if err := NewClient(srv.Client()).Test(context.Background(), cfg); !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("err = %v", err)
	}
}

func TestSendCreatesWithoutMarker(t *testing.T) {
	srv, cfg, calls := fakeITop(t, func(restCall) string {
		return `{"code":0,"objects":{"Incident::5":{"code":0,"class":"Incident","key":"5","fields":{}}}}`
	})
	if err := NewClient(srv.Client()).Send(context.Background(), cfg, plugin.NotificationMessage{Title: "Backup failed", Message: "a & b", Level: "warning"}); err != nil {
		t.Fatal(err)
	}
	f := (*calls)[0].Fields
	if (*calls)[0].Operation != "core/create" || f["title"] != "Backup failed" || f["impact"] != "2" || !strings.Contains(f["description"].(string), "a &amp; b") {
		t.Fatalf("create = %+v", (*calls)[0])
	}
}
