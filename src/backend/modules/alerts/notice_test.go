// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

func TestDispatchHandsEveryFiringToTheAlertNotifier(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := &Engine{
		logger:   logger,
		notifier: &notifier{logger: logger},
		history:  newHistoryStore(openAlertsTestDB(t)),
	}
	var got []AlertNotice
	e.SetAlertNotifier(func(n AlertNotice) { got = append(got, n) })

	rule := Alert{ID: "a1", Name: "web down", Severity: "critical", SourceType: SourceContainer}
	obs := alerting.AlertObservation{SubjectID: "abc123", SubjectName: "web"}
	e.dispatch(context.Background(), rule, alerting.AlertConditionDecl{}, environmentRef{ID: "env-1", Name: "prod"},
		obs, alertState{}, EventStateFiring, time.Now())

	if len(got) != 1 {
		t.Fatalf("the notifier saw %d notices, want 1", len(got))
	}
	n := got[0]
	if n.AlertID != "a1" || n.EnvironmentID != "env-1" || n.SubjectID != "abc123" || n.SourceType != SourceContainer || n.State != EventStateFiring {
		t.Fatalf("notice lost its routing fields: %+v", n)
	}
	if n.Title == "" || n.Severity != "critical" {
		t.Fatalf("notice title/severity = %q/%q", n.Title, n.Severity)
	}

	e.SetAlertNotifier(nil)
	e.dispatch(context.Background(), rule, alerting.AlertConditionDecl{}, environmentRef{ID: "env-1"}, obs, alertState{}, EventStateResolved, time.Now())
	if len(got) != 1 {
		t.Fatal("a cleared notifier was still called")
	}
}
