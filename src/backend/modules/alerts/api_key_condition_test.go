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

func TestAPIKeyRotationConditions(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "k.db"))
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
	now := time.Now().UTC()
	stamp := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, username, password_hash) VALUES ('u', 'owner', 'x')`, nil},
		{`INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, scopes, expires_at, is_active, created_at, updated_at) VALUES ('old', 'u', 'old key', 'h1', 'p', '[]', NULL, 1, ?, ?)`, []any{stamp(-100 * 24 * time.Hour), stamp(0)}},
		{`INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, scopes, expires_at, is_active, created_at, updated_at) VALUES ('exp', 'u', 'expiring', 'h2', 'p', '[]', ?, 1, ?, ?)`, []any{stamp(3*24*time.Hour + time.Hour), stamp(-time.Hour), stamp(0)}},
		{`INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, scopes, expires_at, is_active, created_at, updated_at) VALUES ('dead', 'u', 'revoked', 'h3', 'p', '[]', ?, 0, ?, ?)`, []any{stamp(time.Hour), stamp(-500 * 24 * time.Hour), stamp(0)}},
	} {
		if _, err := database.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	src := &platformSource{db: database}
	eval := func(key string) map[string]alerting.AlertObservation {
		t.Helper()
		obs, err := src.Evaluate(context.Background(), alerting.EvaluateRequest{ConditionKey: key})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]alerting.AlertObservation{}
		for _, o := range obs {
			out[o.SubjectID] = o
		}
		return out
	}
	ages := eval(conditionAPIKeyRotationDue)
	if ages["old"].Value != 100 || len(ages) != 2 {
		t.Fatalf("ages: %+v", ages)
	}
	if _, ok := ages["dead"]; ok {
		t.Fatal("a revoked key must not be observed")
	}
	expiring := eval(conditionAPIKeyExpiring)
	if len(expiring) != 1 || expiring["exp"].Value != 3 {
		t.Fatalf("expiring: %+v", expiring)
	}
}
