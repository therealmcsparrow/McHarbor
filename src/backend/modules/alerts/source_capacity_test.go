// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/capacity"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

func openCapacityAlertDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	_, err = db.Exec(`
		CREATE TABLE capacity_samples (
			id              TEXT PRIMARY KEY,
			environment_id  TEXT NOT NULL,
			subject_kind    TEXT NOT NULL,
			subject_id      TEXT NOT NULL,
			subject_name    TEXT NOT NULL DEFAULT '',
			sampled_at      TEXT NOT NULL,
			bucket_seconds  INTEGER NOT NULL DEFAULT 3600,
			used_bytes      INTEGER NOT NULL DEFAULT 0,
			total_bytes     INTEGER NOT NULL DEFAULT 0,
			created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
			updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		);
		CREATE UNIQUE INDEX idx_capacity_samples_unique
			ON capacity_samples(environment_id, subject_kind, subject_id, sampled_at);
		CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);`)
	if err != nil {
		t.Fatalf("creating tables: %v", err)
	}
	return db
}

// seedCapacity writes a daily series for one subject, rising by perDay percent.
func seedCapacity(t *testing.T, db *sql.DB, subjectID string, days int, startPercent, perDay float64) {
	t.Helper()
	const total int64 = 1_000_000
	now := time.Now().UTC()
	for i := 0; i < days; i++ {
		pct := startPercent + perDay*float64(i)
		at := now.AddDate(0, 0, -(days - 1 - i))
		reading := capacity.Reading{
			EnvironmentID: "env1", SubjectKind: capacity.KindRootFS,
			SubjectID: subjectID, SubjectName: "/" + subjectID,
			UsedBytes: int64(pct / 100 * float64(total)), TotalBytes: total,
		}
		if err := capacity.Write(context.Background(), db, reading, at); err != nil {
			t.Fatalf("seeding capacity sample: %v", err)
		}
	}
}

func capacityRequest() alerting.EvaluateRequest {
	return alerting.EvaluateRequest{
		ConditionKey:    conditionCapacityDays,
		EnvironmentID:   "env1",
		EnvironmentName: "Production",
	}
}

// The host compares; the source only measures. Days-to-exhaustion is the value and
// the operator's threshold never reaches here.
func TestCapacityConditionReportsDaysAsTheValue(t *testing.T) {
	db := openCapacityAlertDB(t)
	seedCapacity(t, db, "fast", 10, 70, 1)

	src := newHostSource(EngineDeps{DB: db})
	obs, err := src.Evaluate(context.Background(), capacityRequest())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("got %d observations, want 1", len(obs))
	}
	if obs[0].Value <= 0 || obs[0].Value > 30 {
		t.Fatalf("value = %v, want a day count inside the forecast horizon", obs[0].Value)
	}
	if obs[0].ValueLabel == "" {
		t.Fatal("an observation must carry a label the operator can read")
	}
	// Active is never set here: deciding whether "< 14" is met is the host's job.
	if obs[0].Active {
		t.Fatal("the source must not decide whether the condition fired")
	}
	if obs[0].Context["subject_kind"] != capacity.KindRootFS {
		t.Fatalf("context lost the subject kind: %+v", obs[0].Context)
	}
}

// A healthy subject still reports, or the engine reads its absence as "recovered"
// and a rule that should be watching it goes quiet.
func TestCapacityConditionReportsHealthySubjectsToo(t *testing.T) {
	db := openCapacityAlertDB(t)
	seedCapacity(t, db, "urgent", 10, 80, 1)
	seedCapacity(t, db, "calm", 10, 40, 1.8)

	src := newHostSource(EngineDeps{DB: db})
	obs, err := src.Evaluate(context.Background(), capacityRequest())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(obs) != 2 {
		t.Fatalf("got %d observations, want both subjects: %+v", len(obs), obs)
	}
	byID := map[string]float64{}
	for _, o := range obs {
		byID[o.SubjectID] = o.Value
	}
	urgent := byID[capacity.KindRootFS+":urgent"]
	calm := byID[capacity.KindRootFS+":calm"]
	if urgent == 0 || calm == 0 {
		t.Fatalf("both subjects must report: %+v", byID)
	}
	if urgent >= calm {
		t.Fatalf("the subject nearer the red line must report fewer days: urgent %v, calm %v", urgent, calm)
	}
}

// A refused subject is neither healthy nor firing. Emitting a large value for it
// would make "not enough data" read as a thousand days of headroom.
func TestCapacityConditionEmitsNothingForARefusedSubject(t *testing.T) {
	db := openCapacityAlertDB(t)
	seedCapacity(t, db, "tooshort", 3, 80, 2)
	seedCapacity(t, db, "flat", 14, 60, 0)

	src := newHostSource(EngineDeps{DB: db})
	obs, err := src.Evaluate(context.Background(), capacityRequest())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(obs) != 0 {
		t.Fatalf("a refused subject produced an observation: %+v", obs)
	}
}

func TestCapacityConditionIsDeclared(t *testing.T) {
	var found *alerting.AlertConditionDecl
	for i, c := range hostSourceSpec().Conditions {
		if c.Key == conditionCapacityDays {
			found = &hostSourceSpec().Conditions[i]
		}
	}
	if found == nil {
		t.Fatal("capacity_days is not declared on the host source")
	}
	if found.Kind != alerting.KindThreshold {
		t.Fatalf("kind = %v, want a threshold", found.Kind)
	}
	if found.Unit != "days" {
		t.Fatalf("unit = %q, want days", found.Unit)
	}
	if found.DefaultOperator != "<" || found.DefaultThreshold != 14 {
		t.Fatalf("defaults = %q %v, want < 14", found.DefaultOperator, found.DefaultThreshold)
	}
	// A fit over the retained series is not a per-tick query.
	if found.PollInterval < time.Minute {
		t.Fatalf("poll interval = %v, want it held off the fast path", found.PollInterval)
	}
}
