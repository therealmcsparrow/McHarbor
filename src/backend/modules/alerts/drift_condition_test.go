// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

func TestDesiredStateDriftFollowsWhatTheReconcilerRecorded(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "drift.db"))
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
	var many []string
	for i := 0; i < 7; i++ {
		many = append(many, fmt.Sprintf(`{"class":"existence","field":"undeclared.c%d"}`, i))
	}
	for _, stmt := range []string{
		`INSERT INTO desired_states (id, name, kind, drift_detected, last_report, created_at, updated_at) VALUES
		 ('ds1', 'shop', 'stack', 1, '{"diffs":[{"class":"state","field":"service.db"},{"class":"existence","field":"service.sidecar"}]}', '2026-09-26', '2026-09-26'),
		 ('ds2', 'web', 'container', 0, NULL, '2026-09-26', '2026-09-26'),
		 ('ds3', 'prod', 'environment', 1, '{"diffs":[` + strings.Join(many, ",") + `]}', '2026-09-26', '2026-09-26')`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	obs, err := (&platformSource{db: database}).Evaluate(context.Background(), alerting.EvaluateRequest{ConditionKey: conditionDesiredStateDrift})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]alerting.AlertObservation{}
	for _, o := range obs {
		byID[o.SubjectID] = o
	}
	if o := byID["ds1"]; !o.Active || o.ValueLabel != "state, existence" || o.Context["kind"] != "stack" {
		t.Fatalf("drifted stack = %+v", o)
	}
	if o := byID["ds2"]; o.Active || o.ValueLabel != "in sync" {
		t.Fatalf("in-sync container = %+v", o)
	}
	if f := byID["ds3"].Context["fields"]; !strings.HasSuffix(f, "+2 more") || strings.Count(f, "undeclared") != 5 {
		t.Fatalf("a long report was not cut down for a chat message: %q", f)
	}

	found := false
	for _, c := range platformSourceSpec().Conditions {
		if c.Key == conditionDesiredStateDrift {
			found = true
		}
	}
	if !found {
		t.Fatal("the condition is not in the catalog, so no rule can be made for it")
	}
}
