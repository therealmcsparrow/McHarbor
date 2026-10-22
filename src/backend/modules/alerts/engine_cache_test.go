// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/inapp"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

type stubClient struct{ obs alerting.AlertObservation }

func (c stubClient) Evaluate(context.Context, alerting.EvaluateRequest) ([]alerting.AlertObservation, error) {
	return []alerting.AlertObservation{c.obs}, nil
}

// brokenEngine is an engine whose database has no tables at all, which is what every
// write looks like on a full disk: state, history and in-app all fail.
func brokenEngine(t *testing.T, clock *time.Time) (*Engine, *fakeEmitter, *atomic.Int32) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/empty.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	em := &fakeEmitter{types: map[string]string{"itsm": "servicenow"}}
	SetIncidentEmitter(em)
	t.Cleanup(func() { SetIncidentEmitter(nil) })
	var inAppTries atomic.Int32
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Engine{
		db: db, logger: logger, service: NewService(db),
		notifier: &notifier{logger: logger, sendInApp: func(inapp.Record) error {
			inAppTries.Add(1)
			return errors.New("database or disk is full")
		}},
		states: newStateStore(db), history: newHistoryStore(db),
		cache: newStateCache(), lastRun: map[string]time.Time{},
		clock: func() time.Time { return *clock },
	}, em, &inAppTries
}

func diskRule(forSeconds int) Alert {
	return Alert{
		ID: "r1", Name: "Host disk usage critical", Severity: "critical", SourceType: sourceHost,
		ConditionKey: conditionHostFSPercent, SendInApp: true, NotifyOnRecovery: true,
		Condition:    ConditionConfig{Operator: ">=", Threshold: 95, Unit: "%", ForSeconds: forSeconds},
		Destinations: []AlertDestination{{ChannelID: "itsm"}},
	}
}

func evaluate(e *Engine, rule Alert, value float64) {
	decl := hostFSPercentDecl()
	client := stubClient{obs: alerting.AlertObservation{SubjectID: "env1", SubjectName: "Prod", Value: value, ValueLabel: "98%"}}
	e.evaluateRule(context.Background(), rule, decl, client, []environmentRef{{ID: "env1", Name: "Prod"}})
}

func TestCriticalAlertFiresExactlyOnceWhenEveryWriteFails(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	e, em, inApp := brokenEngine(t, &now)
	rule := diskRule(0)
	for i := 0; i < 3; i++ {
		evaluate(e, rule, 98)
		now = now.Add(30 * time.Second)
	}
	if len(em.events) != 1 {
		t.Fatalf("channel received %d alerts over 3 ticks, want exactly 1 (a lost write must not re-send)", len(em.events))
	}
	if inApp.Load() != 1 {
		t.Fatalf("in-app attempted %d times, want 1", inApp.Load())
	}
}

func TestSustainedAlertFiresAfterItsWindowWhenWritesFail(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	e, em, _ := brokenEngine(t, &now)
	rule := diskRule(300)
	for i := 0; i < 12; i++ {
		evaluate(e, rule, 98)
		now = now.Add(30 * time.Second)
	}
	if len(em.events) != 1 {
		t.Fatalf("a 300s rule delivered %d alerts over 6 minutes, want 1 (state must survive without SQLite)", len(em.events))
	}
}

func TestRecoveryIsDeliveredFromCachedState(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	e, em, _ := brokenEngine(t, &now)
	rule := diskRule(0)
	evaluate(e, rule, 98)
	now = now.Add(30 * time.Second)
	evaluate(e, rule, 10)
	if len(em.events) != 2 {
		t.Fatalf("events = %d, want firing then resolved", len(em.events))
	}
}

type fakeCoordinator struct{ leader bool }

func (f *fakeCoordinator) LeaderOf(context.Context, string) bool { return f.leader }
func (f *fakeCoordinator) NodeID() string                        { return "n1" }

func TestCacheIsDroppedOnLeaderLoss(t *testing.T) {
	now := time.Now()
	e, _, _ := brokenEngine(t, &now)
	e.cache.put("r1", alertState{Key: "r1:env1:env1", AlertID: "r1", Active: true})
	e.coordinator, e.hasCoord = &fakeCoordinator{leader: false}, true
	e.tick(context.Background())
	if _, warm := e.cache.snapshot("r1"); warm {
		t.Fatal("a node that is not the leader kept its cached state")
	}
}

func TestDeletingARuleDropsItsCachedState(t *testing.T) {
	db := openAlertsTestDB(t)
	createAlertsSchema(t, db)
	sharedStateCache.put("r1", alertState{Key: "r1:env1:env1", AlertID: "r1", Active: true})
	t.Cleanup(sharedStateCache.dropAll)
	if err := newStateStore(db).deleteForAlert(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	if _, warm := sharedStateCache.snapshot("r1"); warm {
		t.Fatal("deleteForAlert left the cache warm")
	}
}

func TestAcknowledgementReachesTheCache(t *testing.T) {
	c := newStateCache()
	c.put("r1", alertState{Key: "r1:e:s", AlertID: "r1", Active: true})
	c.acknowledge("r1:e:s")
	snap, _ := c.snapshot("r1")
	if !snap["r1:e:s"].Acknowledged {
		t.Fatal("acknowledgement was not applied to the cached state")
	}
}

func TestAcknowledgementClearsWhenTheStateResolves(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	e, _, _ := brokenEngine(t, &now)
	rule := diskRule(0)
	evaluate(e, rule, 98)
	e.cache.acknowledge(stateKey(rule.ID, "env1", "env1"))
	now = now.Add(time.Minute)
	evaluate(e, rule, 10)
	snap, _ := e.cache.snapshot(rule.ID)
	if snap[stateKey(rule.ID, "env1", "env1")].Acknowledged {
		t.Fatal("a resolved state kept its acknowledgement and would silence the next episode")
	}
}
