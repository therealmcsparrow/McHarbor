// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func seedRun(t *testing.T, database *sql.DB, incidentID, outcome string, at time.Time) string {
	t.Helper()
	runID := "run-" + incidentID + "-" + outcome
	if _, err := database.Exec(
		`INSERT OR IGNORE INTO runbooks (id, name, attach_kind, environment_id, created_at, updated_at)
		 VALUES ('rb-1', 'Restart loop', 'alert_condition', 'e1', ?, ?)`,
		at.UTC().Format(time.RFC3339), at.UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`INSERT INTO runbook_runs (id, runbook_id, runbook_name, environment_id, subject_kind,
		 subject_id, subject_name, incident_id, alert_event_id, started_by, started_by_name,
		 started_at, finished_at, outcome, note)
		 VALUES (?, 'rb-1', 'Restart loop', 'e1', 'container', '', 'web', ?, '', 'u-ops', 'ops', ?, ?, ?, ?)`,
		runID, incidentID, at.UTC().Format(time.RFC3339), at.UTC().Format(time.RFC3339),
		outcome, "restart did not clear it"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`INSERT INTO runbook_step_results (id, run_id, step_id, step_position, step_title, step_kind,
		 state, skip_reason, output, finding, acted_by, acted_by_name, at, last_error)
		 VALUES (?, ?, 'st-4', 4, 'Restart the container', 'action', 'failed', '', '',
		         'restart did not clear it', 'u-ops', 'ops', ?, 'exit event never arrived')`,
		"res-"+runID, runID, at.UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	return runID
}

func timelineFor(t *testing.T, database *sql.DB, in *Incident) []TimelineItem {
	t.Helper()
	asm := &Assembler{DB: database, Now: func() time.Time { return t0.Add(time.Hour) }}
	items, err := BuildTimeline(context.Background(), asm, in, t0.Add(time.Hour))
	if err != nil {
		t.Fatalf("build timeline: %v", err)
	}
	return items
}

func kinds(items []TimelineItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Kind)
	}
	return out
}

func TestTimelineNamesTheRunbookFollowedAndWhereItStopped(t *testing.T) {
	database := openDB(t)
	in := &Incident{
		ID: "in-1", EnvID: "e1", SubjectType: "container", SubjectName: "web",
		Signal: "restarting repeatedly", FirstSignalAt: t0.UTC().Format(time.RFC3339),
		LastSignalAt: t0.Add(10 * time.Minute).UTC().Format(time.RFC3339),
	}
	seedRun(t, database, "in-1", "abandoned", t0.Add(5*time.Minute))

	items := timelineFor(t, database, in)

	var runbook *TimelineItem
	for i := range items {
		if items[i].Kind == TimelineRunbook {
			runbook = &items[i]
		}
	}
	if runbook == nil {
		t.Fatalf("no runbook line in the timeline: %v", kinds(items))
	}
	if runbook.Who != "ops" {
		t.Fatalf("the runbook line must name who followed it, got %q", runbook.Who)
	}
	for _, want := range []string{"Restart loop", "was abandoned", "stopped at step 4", "restart did not clear it"} {
		if !strings.Contains(runbook.Detail, want) {
			t.Fatalf("the runbook line is missing %q: %q", want, runbook.Detail)
		}
	}
	// It is ordered with the rest, not appended at the end.
	for i := 1; i < len(items); i++ {
		if items[i].Kind == TimelineResolved {
			continue
		}
		if items[i].At < items[i-1].At && items[i-1].Kind != TimelineResolved {
			t.Fatalf("timeline is out of order at %d: %v", i, kinds(items))
		}
	}
	// A recorded run is the answer, so the lastChange guess stands down.
	for _, it := range items {
		if it.Kind == TimelineChange {
			t.Fatalf("lastChange still guessed alongside a recorded run: %q", it.Detail)
		}
	}
}

func TestTimelineWithNoRunKeepsTheLastChangeFallback(t *testing.T) {
	database := openDB(t)
	first := t0
	in := &Incident{
		ID: "in-2", EnvID: "e1", SubjectType: "container", SubjectName: "web",
		Signal: "restarting repeatedly", FirstSignalAt: first.UTC().Format(time.RFC3339),
		LastSignalAt: first.Add(5 * time.Minute).UTC().Format(time.RFC3339),
	}
	if _, err := database.Exec(
		`INSERT INTO audit_logs (id, user_id, username, action, entity_type, entity_name, timestamp, severity)
		 VALUES ('a1', 'u-ops', 'ops', 'restart', 'container', 'web', ?, 'info')`,
		first.Add(8*time.Minute).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	items := timelineFor(t, database, in)

	var change bool
	for _, it := range items {
		if it.Kind == TimelineRunbook {
			t.Fatalf("an incident with no run must produce no runbook line")
		}
		if it.Kind == TimelineChange {
			change = true
		}
	}
	if !change {
		t.Fatalf("the lastChange fallback must still fire with no run: %v", kinds(items))
	}
}
