// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

type fakeEmitter struct {
	mu     sync.Mutex
	types  map[string]string
	events []incident.Event
}

func (f *fakeEmitter) ChannelType(_ context.Context, id string) (string, error) {
	return f.types[id], nil
}
func (f *fakeEmitter) Supports(t string) bool { return t == "servicenow" }
func (f *fakeEmitter) Emit(_ context.Context, _ string, ev incident.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, ev)
	return nil
}

// TestDeliverRoutesIncidentChannels checks an ITSM channel receives a
// lifecycle event keyed on the alert state, while a chat channel does not,
// and a test send never opens a ticket.
func TestDeliverRoutesIncidentChannels(t *testing.T) {
	em := &fakeEmitter{types: map[string]string{"itsm": "servicenow", "chat": "slack"}}
	SetIncidentEmitter(em)
	t.Cleanup(func() { SetIncidentEmitter(nil) })
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	n := &notifier{dispatcher: corenotify.NewDispatcher(database, nil), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rule := Alert{ID: "a1", Name: "CPU", Severity: "critical", Destinations: []AlertDestination{{ChannelID: "itsm"}, {ChannelID: "chat"}}}
	got := n.deliver(context.Background(), rule, deliveryInput{
		Title: "t", Message: "m", State: EventStateFiring, StateKey: "a1:e1:c1",
		EnvironmentID: "e1", EnvironmentName: "prod", SubjectID: "c1", SubjectName: "web",
	})
	if len(em.events) != 1 || em.events[0].DedupKey != "a1:e1:c1" || em.events[0].Severity != incident.SeverityCritical ||
		em.events[0].SubjectName != "web" || em.events[0].State != incident.StateFiring {
		t.Fatalf("incident channel not emitted correctly: %+v", em.events)
	}
	if got[0].Mode != "incident" || !got[0].OK || got[1].Mode != "" {
		t.Fatalf("deliveries: %+v", got)
	}
	n.deliver(context.Background(), rule, deliveryInput{Title: "t", Message: "m", State: EventStateFiring})
	if len(em.events) != 1 {
		t.Fatalf("a test send opened a ticket: %+v", em.events)
	}
}

// TestAcknowledgedConditionDoesNotRenotify checks the cooldown repeat is
// withheld once an ITSM reported someone is working on it.
func TestAcknowledgedConditionDoesNotRenotify(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	prev := alertState{Active: true, ActiveSince: now.Add(-2 * time.Hour), LastNotifiedAt: now.Add(-time.Hour)}
	if d := decide(prev, true, time.Time{}, now, 0, 30*time.Minute, true); !d.Notify {
		t.Fatal("an unacknowledged condition past its cooldown must repeat")
	}
	prev.Acknowledged = true
	if d := decide(prev, true, time.Time{}, now, 0, 30*time.Minute, true); d.Notify {
		t.Fatal("an acknowledged condition repeated")
	}
	if d := decide(prev, false, time.Time{}, now, 0, 30*time.Minute, true); !d.Notify || d.State != EventStateResolved {
		t.Fatal("an acknowledged condition must still resolve")
	}
}
