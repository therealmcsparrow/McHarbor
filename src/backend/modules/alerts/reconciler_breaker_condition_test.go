// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

func TestBreakerConditionFollowsTheSettingsRow(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "br.db"))
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
	src := &platformSource{db: database}
	eval := func() alerting.AlertObservation {
		t.Helper()
		obs, err := src.Evaluate(context.Background(), alerting.EvaluateRequest{ConditionKey: conditionReconcilerBreakerOpen})
		if err != nil || len(obs) != 1 {
			t.Fatalf("evaluate: %+v %v", obs, err)
		}
		return obs[0]
	}
	if eval().Active {
		t.Fatal("a fresh instance reports the breaker open")
	}
	if _, err := database.Exec(`UPDATE reconciler_settings SET breaker_open = 1, breaker_reason = 'web: daemon said no' WHERE id = 'global'`); err != nil {
		t.Fatal(err)
	}
	if o := eval(); !o.Active || o.Context["error"] != "web: daemon said no" {
		t.Fatalf("open breaker: %+v", o)
	}
}
