// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

func TestInstanceExportConditionsFollowTheSchedules(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "ib.db"))
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
	old := time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)
	recent := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	for _, stmt := range []string{
		`INSERT INTO instance_backup_schedules (id, name, cron, enabled, key_kind, key_secret_enc, last_status, last_error, created_at)
		 VALUES ('s1', 'nightly', '0 3 * * *', 1, 'passphrase', 'x', 'failed', 'sftp: permission denied', '` + old + `'),
		        ('s2', 'hourly', '0 * * * *', 1, 'passphrase', 'x', 'completed', '', '` + old + `'),
		        ('s3', 'off', '0 * * * *', 0, 'passphrase', 'x', '', '', '` + old + `')`,
		`INSERT INTO instance_backup_exports (id, status, schedule_id, completed_at) VALUES ('e1', 'completed', 's2', '` + recent + `')`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	src := &platformSource{db: database}

	failed, err := src.Evaluate(context.Background(), alerting.EvaluateRequest{ConditionKey: conditionInstanceBackupFailed})
	if err != nil {
		t.Fatal(err)
	}
	active := map[string]bool{}
	for _, o := range failed {
		active[o.SubjectID] = o.Active
	}
	if !active["s1"] || active["s2"] || active["s3"] {
		t.Fatalf("failed: %+v", failed)
	}

	overdue, err := src.Evaluate(context.Background(), alerting.EvaluateRequest{ConditionKey: conditionInstanceBackupOverdue})
	if err != nil {
		t.Fatal(err)
	}
	since := map[string]time.Duration{}
	for _, o := range overdue {
		since[o.SubjectID] = time.Since(o.Since)
	}
	if _, ok := since["s3"]; ok {
		t.Fatal("a disabled schedule is reported overdue")
	}
	if since["s1"] < 71*time.Hour || since["s2"] > 2*time.Hour {
		t.Fatalf("overdue: never-succeeded counts from creation, the rest from their last export: %v", since)
	}
}
