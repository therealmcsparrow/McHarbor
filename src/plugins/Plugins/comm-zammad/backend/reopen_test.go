// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type reopenFake struct {
	mu       sync.Mutex
	state    string
	closeAt  time.Time
	created  int
	puts     []map[string]any
	putPaths []string
	tagged   int
}

func (f *reopenFake) handler(t *testing.T, cond string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/tickets/search":
			q, _ := url.QueryUnescape(r.URL.Query().Get("query"))
			if strings.HasPrefix(q, `"MC-`) {
				title := strconv.Quote("[MH-0000000000] " + cond + " Festplatte voll")
				write(t, w, `[{"id":11,"number":"31011","title":`+title+`,"state":"`+f.state+`","close_at":"`+f.closeAt.UTC().Format("2006-01-02T15:04:05.000Z")+`"}]`)
				return
			}
			write(t, w, `[]`)
		case r.Method == http.MethodPut:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			f.puts = append(f.puts, body)
			f.putPaths = append(f.putPaths, r.URL.Path)
			write(t, w, `{}`)
		case r.URL.Path == "/api/v1/tickets" && r.Method == http.MethodPost:
			f.created++
			w.WriteHeader(http.StatusCreated)
			write(t, w, `{"id":10,"number":"31010","title":"x","state_id":1}`)
		case r.URL.Path == "/api/v1/tags/add":
			f.tagged++
			write(t, w, `true`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}
}

func openWith(t *testing.T, f *reopenFake, window string, closedAgo time.Duration, state string) (id string) {
	t.Helper()
	ev := firingEvent()
	now := time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC)
	f.state, f.closeAt = state, now.Add(-closedAgo)
	srv, cfg := fakeZammad(t, f.handler(t, conditionMarker(ev)))
	cfg["reopen_window_minutes"] = window
	c := NewClient(srv.Client())
	c.now = func() time.Time { return now }
	ref, err := c.Open(context.Background(), cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	return ref.ExternalID
}

func TestOpenReopensTicketClosedInsideWindow(t *testing.T) {
	f := &reopenFake{}
	if id := openWith(t, f, "30", 5*time.Minute, "closed"); id != "11" {
		t.Fatalf("ref = %s, want the closed ticket 11", id)
	}
	if f.created != 0 || len(f.puts) != 1 || f.putPaths[0] != "/api/v1/tickets/11" || f.puts[0]["state"] != "open" {
		t.Fatalf("created=%d puts=%v paths=%v", f.created, f.puts, f.putPaths)
	}
	art, _ := f.puts[0]["article"].(map[string]any)
	if !strings.HasPrefix(art["body"].(string), "Reopened: the alert fired again.") || f.tagged != 1 {
		t.Fatalf("article=%v tagged=%d", art, f.tagged)
	}
}

func TestOpenCreatesNewTicketOutsideWindow(t *testing.T) {
	f := &reopenFake{}
	if id := openWith(t, f, "30", 45*time.Minute, "closed"); id != "10" || f.created != 1 || len(f.puts) != 0 {
		t.Fatalf("id=%s created=%d puts=%v", id, f.created, f.puts)
	}
}

func TestOpenNeverReopensMergedTicket(t *testing.T) {
	f := &reopenFake{}
	if id := openWith(t, f, "30", time.Minute, "merged"); id != "10" || len(f.puts) != 0 {
		t.Fatalf("id=%s puts=%v", id, f.puts)
	}
}

func TestOpenWithWindowZeroNeverReopens(t *testing.T) {
	f := &reopenFake{}
	if id := openWith(t, f, "0", time.Minute, "closed"); id != "10" || len(f.puts) != 0 {
		t.Fatalf("id=%s puts=%v", id, f.puts)
	}
}
