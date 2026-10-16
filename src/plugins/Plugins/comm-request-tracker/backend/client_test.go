// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestOpenSearchesCFFirst(t *testing.T) {
	var creates int
	srv, cfg := fakeRT(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rt/REST/2.0/tickets":
			q := r.URL.Query().Get("query")
			if !strings.Contains(q, "'CF.{McHarbor Dedup Key}' = '"+hashKey("a:e:s", 64)+"'") || !strings.Contains(q, "Status = '__Active__'") || !strings.Contains(q, "Queue = 'Operations'") {
				t.Errorf("query = %s", q)
			}
			if creates == 0 {
				w.Write([]byte(`{"total":0,"count":0,"page":1,"pages":0,"per_page":5,"items":[]}`))
				return
			}
			w.Write([]byte(`{"total":1,"count":1,"page":1,"pages":1,"per_page":5,"items":[{"type":"ticket","id":"77","_url":"x"}]}`))
		case r.URL.Path == "/rt/REST/2.0/ticket" && r.Method == http.MethodPost:
			creates++
			var body struct {
				Queue        string            `json:"Queue"`
				Subject      string            `json:"Subject"`
				Priority     int               `json:"Priority"`
				ContentType  string            `json:"ContentType"`
				Requestor    *string           `json:"Requestor"`
				CustomFields map[string]string `json:"CustomFields"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Queue != "Operations" || body.Priority != 90 || body.ContentType != "text/plain" || len(body.CustomFields["McHarbor Dedup Key"]) != 64 || body.Subject != "[McHarbor] RAID degraded" || body.Requestor != nil {
				t.Errorf("body = %+v", body)
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"type":"ticket","id":77,"_url":"https://rt/REST/2.0/ticket/77"}`))
		case r.URL.Path == "/rt/REST/2.0/ticket/77":
			w.Write([]byte(`{"id":77,"Status":"new","CustomFields":[{"name":"McHarbor Dedup Key","values":["` + hashKey("a:e:s", 64) + `"]}]}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	c := NewClient(srv.Client())
	ev := incident.Event{DedupKey: "a:e:s", State: incident.StateFiring, Severity: incident.SeverityCritical, Title: "RAID degraded"}
	a, err := c.Open(context.Background(), cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Open(context.Background(), cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	if creates != 1 || a.ExternalID != "77" || b.ExternalID != "77" || a.URL != srv.URL+"/rt/Ticket/Display.html?id=77" {
		t.Fatalf("creates=%d a=%+v b=%+v", creates, a, b)
	}
}

func TestOpenDetectsUnappliedCF(t *testing.T) {
	srv, cfg := fakeRT(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rt/REST/2.0/tickets":
			w.Write([]byte(`{"total":0,"count":0,"page":1,"pages":0,"per_page":5,"items":[]}`))
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"type":"ticket","id":78}`))
		default:
			w.Write([]byte(`{"id":78,"Status":"new","CustomFields":[]}`))
		}
	})
	_, err := NewClient(srv.Client()).Open(context.Background(), cfg, incident.Event{DedupKey: "k", State: incident.StateFiring, Severity: incident.SeverityInfo, Title: "t"})
	if !errors.Is(err, incident.ErrOperatorMessage) || !strings.Contains(err.Error(), "not applied") || !strings.Contains(err.Error(), "#78") {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, incident.ErrRetryable) {
		t.Fatal("an unapplied CF must be permanent, or every retry creates a ticket")
	}
}

func TestCFValuesReadsBothShapes(t *testing.T) {
	arr := json.RawMessage(`[{"name":"Other","values":["x"]},{"name":"mcharbor dedup key","values":["v1"]}]`)
	if got := cfValues(arr, "McHarbor Dedup Key"); len(got) != 1 || got[0] != "v1" {
		t.Fatalf("array shape: %v", got)
	}
	obj := json.RawMessage(`{"McHarbor Dedup Key":["v2"]}`)
	if got := cfValues(obj, "McHarbor Dedup Key"); len(got) != 1 || got[0] != "v2" {
		t.Fatalf("object shape: %v", got)
	}
}

func TestOpenReopensWithinWindow(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	var comment map[string]any
	var creates int
	srv, cfg := fakeRT(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rt/REST/2.0/tickets":
			if strings.Contains(r.URL.Query().Get("query"), "__Active__") {
				w.Write([]byte(`{"items":[]}`))
				return
			}
			w.Write([]byte(`{"items":[{"id":"55","Status":"resolved","Resolved":"2026-09-25T11:50:00Z"}]}`))
		case r.URL.Path == "/rt/REST/2.0/ticket/55/comment":
			if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
				t.Error(err)
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`["Comments added","Status changed from 'resolved' to 'open'"]`))
		case r.Method == http.MethodPost && r.URL.Path == "/rt/REST/2.0/ticket":
			creates++
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":99}`))
		default:
			w.Write([]byte(`{"id":99,"CustomFields":[{"name":"McHarbor Dedup Key","values":["` + hashKey("k", 64) + `"]}]}`))
		}
	})
	cfg["reopen_window_minutes"] = "30"
	c := NewClient(srv.Client())
	c.now = func() time.Time { return now }
	ref, err := c.Open(context.Background(), cfg, incident.Event{DedupKey: "k", State: incident.StateFiring, Severity: incident.SeverityInfo, Title: "t"})
	if err != nil || ref.ExternalID != "55" || creates != 0 || comment["Status"] != "open" {
		t.Fatalf("ref=%+v err=%v creates=%d comment=%v", ref, err, creates, comment)
	}
	c.now = func() time.Time { return now.Add(time.Hour) }
	if ref, err := c.Open(context.Background(), cfg, incident.Event{DedupKey: "k", State: incident.StateFiring, Severity: incident.SeverityInfo, Title: "t"}); err != nil || creates != 1 || ref.ExternalID != "99" {
		t.Fatalf("outside the window must create: creates=%d err=%v", creates, err)
	}
}

func TestResolveIsOneCommentCall(t *testing.T) {
	var paths []string
	var body map[string]any
	srv, cfg := fakeRT(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`["Comments added","Status changed from 'open' to 'resolved'"]`))
	})
	err := NewClient(srv.Client()).Resolve(context.Background(), cfg, incident.Ref{ExternalID: "77"},
		incident.Event{State: incident.StateResolved, Title: "ok", Message: "RAID rebuilt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "POST /rt/REST/2.0/ticket/77/comment" || body["Status"] != "resolved" || !strings.Contains(body["Content"].(string), "RAID rebuilt") {
		t.Fatalf("paths=%v body=%v", paths, body)
	}
}

func TestResolveConfirmsWithoutStatusLine(t *testing.T) {
	for _, tc := range []struct {
		status string
		ok     bool
	}{{"resolved", true}, {"open", false}} {
		srv, cfg := fakeRT(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`["Comments added"]`))
				return
			}
			w.Write([]byte(`{"id":77,"Status":"` + tc.status + `"}`))
		})
		err := NewClient(srv.Client()).Resolve(context.Background(), cfg, incident.Ref{ExternalID: "77"}, incident.Event{State: incident.StateResolved, Title: "ok"})
		if tc.ok != (err == nil) || (!tc.ok && !errors.Is(err, incident.ErrOperatorMessage)) {
			t.Fatalf("status %s: err = %v", tc.status, err)
		}
	}
}

func TestUpdateRejectsNonNumericID(t *testing.T) {
	srv, cfg := fakeRT(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("unexpected call %s", r.URL.Path) })
	err := NewClient(srv.Client()).Update(context.Background(), cfg, incident.Ref{ExternalID: "77/../../queue/1"}, incident.Event{Title: "t"})
	if !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("err = %v", err)
	}
}

func queueHandler(t *testing.T, cfNames string, calls *[]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/rt/REST/2.0/queue/Operations":
			w.Write([]byte(`{"id":3,"Name":"Operations"}`))
		case "/rt/REST/2.0/queue/3/customfields":
			w.Write([]byte(`{"total":1,"count":1,"page":1,"pages":1,"items":[` + cfNames + `]}`))
		case "/rt/REST/2.0/tickets":
			if q := r.URL.Query().Get("query"); q != "Queue = 'Operations'" {
				t.Errorf("probe query = %q", q)
			}
			w.Write([]byte(`{"total":0,"count":0,"page":1,"pages":0,"items":[]}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}
}

func TestTestRequiresCFOnQueue(t *testing.T) {
	var calls []string
	srv, cfg := fakeRT(t, queueHandler(t, `{"id":"9","Name":"Something Else"}`, &calls))
	err := NewClient(srv.Client()).Test(context.Background(), cfg)
	if !errors.Is(err, incident.ErrOperatorMessage) || !strings.Contains(err.Error(), "McHarbor Dedup Key is not applied to queue Operations") {
		t.Fatalf("err = %v", err)
	}
}

func TestTestNoCreate(t *testing.T) {
	var calls []string
	srv, cfg := fakeRT(t, queueHandler(t, `{"id":"5","Name":"McHarbor Dedup Key"}`, &calls))
	if err := NewClient(srv.Client()).Test(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 {
		t.Fatalf("calls = %v", calls)
	}
	for _, c := range calls {
		if !strings.HasPrefix(c, "GET ") {
			t.Fatalf("Test must only read: %v", calls)
		}
	}
}

func TestSendCreatesWithoutSearchOrCF(t *testing.T) {
	var calls []string
	var body map[string]any
	srv, cfg := fakeRT(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":80}`))
	})
	cfg["requestor"] = "ops@example.edu"
	err := NewClient(srv.Client()).Send(context.Background(), cfg, plugin.NotificationMessage{Title: "Backup done", Message: "ok", Level: "warning"})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "POST /rt/REST/2.0/ticket" || body["CustomFields"] != nil || body["Priority"] != float64(50) || body["Requestor"] != "ops@example.edu" {
		t.Fatalf("calls=%v body=%v", calls, body)
	}
}
