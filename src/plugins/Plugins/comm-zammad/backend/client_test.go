// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

type fakeTickets struct {
	mu        sync.Mutex
	created   int
	tagged    int
	tagFails  int
	title     string
	tagBodies []map[string]any
}

func (f *fakeTickets) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/tickets/search":
			if r.URL.Query().Get("expand") != "true" || !strings.HasPrefix(r.URL.Query().Get("query"), `"MH-`) {
				t.Errorf("search query = %s", r.URL.RawQuery)
			}
			if f.title == "" {
				write(t, w, `[{"id":9,"number":"31009","title":"[MH-0000000000] other","state":"open"}]`)
				return
			}
			q := strconv.Quote(f.title)
			write(t, w, `[{"id":11,"number":"31011","title":`+q+`,"state":"closed"},`+
				`{"id":13,"number":"31013","title":`+q+`,"state":"merged"},`+
				`{"id":12,"number":"31012","title":`+strconv.Quote("Fwd: "+f.title)+`,"state":"open"},`+
				`{"id":10,"number":"31010","title":`+q+`,"state":"new"}]`)
		case r.URL.Path == "/api/v1/tickets" && r.Method == http.MethodPost:
			f.created++
			var body struct {
				Title    string         `json:"title"`
				Group    string         `json:"group"`
				Customer string         `json:"customer"`
				Priority string         `json:"priority"`
				Article  map[string]any `json:"article"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Group != "Operations" || body.Customer != "mcharbor@example.de" || body.Priority != "3 high" ||
				body.Article["internal"] != true || body.Article["type"] != "note" || body.Article["content_type"] != "text/plain" {
				t.Errorf("body = %+v", body)
			}
			f.title = body.Title
			w.WriteHeader(http.StatusCreated)
			write(t, w, `{"id":10,"number":"31010","title":`+strconv.Quote(f.title)+`,"state_id":1}`)
		case r.URL.Path == "/api/v1/tags/add":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			f.tagBodies = append(f.tagBodies, body)
			if f.tagFails > 0 {
				f.tagFails--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			f.tagged++
			write(t, w, `true`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}
}

func firingEvent() incident.Event {
	return incident.Event{DedupKey: "a:e:s", State: incident.StateFiring, Severity: incident.SeverityCritical,
		Title: "Festplatte voll", Message: "disk at 97%", OccurredAt: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
}

func TestOpenMarkerExactAndLive(t *testing.T) {
	f := &fakeTickets{}
	srv, cfg := fakeZammad(t, f.handler(t))
	c := NewClient(srv.Client())
	ev := firingEvent()
	a, err := c.Open(context.Background(), cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Open(context.Background(), cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	if f.created != 1 || a.ExternalID != "10" || b.ExternalID != "10" || a.Number != "31010" || b.Number != "31010" || f.tagged != 2 {
		t.Fatalf("created=%d tagged=%d a=%+v b=%+v", f.created, f.tagged, a, b)
	}
	if a.URL != srv.URL+"/#ticket/zoom/10" {
		t.Fatalf("url = %s", a.URL)
	}
	if !strings.HasPrefix(f.title, marker(ev)+" "+conditionMarker(ev)+" Festplatte voll") {
		t.Fatalf("title = %q", f.title)
	}
	for _, body := range f.tagBodies {
		if body["object"] != "Ticket" || body["o_id"] != float64(10) || body["item"] != "mcharbor" {
			t.Fatalf("tag body = %v", body)
		}
	}
}

func TestOpenRetagsExisting(t *testing.T) {
	f := &fakeTickets{tagFails: 1}
	srv, cfg := fakeZammad(t, f.handler(t))
	c := NewClient(srv.Client())
	ev := firingEvent()
	if _, err := c.Open(context.Background(), cfg, ev); !errors.Is(err, incident.ErrRetryable) {
		t.Fatalf("first open: %v", err)
	}
	ref, err := c.Open(context.Background(), cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	if f.created != 1 || f.tagged != 1 || ref.ExternalID != "10" {
		t.Fatalf("created=%d tagged=%d ref=%+v", f.created, f.tagged, ref)
	}
}

func TestMarkerIsPerOccurrence(t *testing.T) {
	ev := firingEvent()
	m := marker(ev)
	if len(m) != len("[MH-]")+10 || m != marker(ev) {
		t.Fatalf("marker %q is not a stable 10-hex marker", m)
	}
	later := ev
	later.OccurredAt = ev.OccurredAt.Add(time.Minute)
	if marker(later) == m {
		t.Fatal("a new firing must get a new marker")
	}
	ev.Title = strings.Repeat("x", 400)
	if title := ticketTitle(ev); len([]rune(title)) != 250 || !strings.HasPrefix(title, m) {
		t.Fatalf("title %q", title)
	}
}

func TestResolveClosesWithNote(t *testing.T) {
	var body map[string]any
	srv, cfg := fakeZammad(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/tickets/10" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Zammad-Suppress-Notifications") != "true" {
			t.Errorf("suppress header missing")
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		write(t, w, `{"id":10,"state":"closed"}`)
	})
	cfg["suppress_notifications"] = "true"
	err := NewClient(srv.Client()).Resolve(context.Background(), cfg, incident.Ref{ExternalID: "10"},
		incident.Event{State: incident.StateResolved, Title: "ok", Message: "recovered"})
	if err != nil {
		t.Fatal(err)
	}
	art, ok := body["article"].(map[string]any)
	if !ok || body["state"] != "closed" || art["internal"] != true || !strings.Contains(art["body"].(string), "recovered") {
		t.Fatalf("body = %v", body)
	}
}

func TestUpdateAddsInternalNote(t *testing.T) {
	var body map[string]any
	srv, cfg := fakeZammad(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/ticket_articles" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Zammad-Suppress-Notifications") != "" {
			t.Errorf("suppress header sent without the option")
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusCreated)
		write(t, w, `{"id":99}`)
	})
	err := NewClient(srv.Client()).Update(context.Background(), cfg, incident.Ref{ExternalID: "10"},
		incident.Event{State: incident.StateFiring, Title: "again", Message: "still full"})
	if err != nil {
		t.Fatal(err)
	}
	if body["ticket_id"] != float64(10) || body["internal"] != true || body["type"] != "note" || !strings.Contains(body["body"].(string), "still full") {
		t.Fatalf("body = %v", body)
	}
	if err := NewClient(srv.Client()).Update(context.Background(), cfg, incident.Ref{ExternalID: "../users"}, incident.Event{}); !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("non-numeric ref accepted: %v", err)
	}
}

func testConnectionFake(t *testing.T, groups string, posts *int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			*posts++
		}
		switch r.URL.Path {
		case "/api/v1/users/me":
			write(t, w, `{"id":3,"login":"mcharbor-agent"}`)
		case "/api/v1/groups":
			write(t, w, groups)
		case "/api/v1/users/search":
			write(t, w, `[{"login":"someone-else","email":"x@example.de"},{"login":"mh","email":"McHarbor@example.de"}]`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}
}

func TestTestChecksGroup(t *testing.T) {
	for _, groups := range []string{`[{"name":"Users","active":true}]`, `[{"name":"Operations","active":false}]`} {
		var posts int
		srv, cfg := fakeZammad(t, testConnectionFake(t, groups, &posts))
		err := NewClient(srv.Client()).Test(context.Background(), cfg)
		if !errors.Is(err, incident.ErrOperatorMessage) || !strings.Contains(err.Error(), "group Operations not found or inactive") {
			t.Fatalf("groups %s: err = %v", groups, err)
		}
	}
}

func TestTestNoCreate(t *testing.T) {
	var posts int
	srv, cfg := fakeZammad(t, testConnectionFake(t, `[{"name":"Users","active":true},{"name":"Operations","active":true}]`, &posts))
	if err := NewClient(srv.Client()).Test(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if posts != 0 {
		t.Fatalf("Test issued %d non-GET requests", posts)
	}
	cfg["customer"] = "nobody@example.de"
	if err := NewClient(srv.Client()).Test(context.Background(), cfg); !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("unknown customer accepted: %v", err)
	}
}

func TestSendCreatesUnmarkedTicket(t *testing.T) {
	var body map[string]any
	srv, cfg := fakeZammad(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tickets" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusCreated)
		write(t, w, `{"id":20}`)
	})
	err := NewClient(srv.Client()).Send(context.Background(), cfg, plugin.NotificationMessage{Title: "Backup failed", Message: "x", Level: "warning"})
	if err != nil {
		t.Fatal(err)
	}
	if body["title"] != "Backup failed" || body["priority"] != "2 normal" {
		t.Fatalf("body = %v", body)
	}
}
