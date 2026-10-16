// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestCascadeConflictsMatchesSQLServerRule pins the three shapes of error 1785
// the corpus produced: a second path through an intermediate table, two keys to
// one parent, and a self-reference.
func TestCascadeConflictsMatchesSQLServerRule(t *testing.T) {
	graph := map[string][]string{
		"container_backup_plans": {"environments"},
	}
	cases := []struct {
		name          string
		parent, child string
		kept          map[string][]string
		want          bool
	}{
		{"first path is allowed", "container_backup_plans", "container_backup_runs", nil, false},
		{"second path through the plan is refused", "environments", "container_backup_runs",
			map[string][]string{"container_backup_runs": {"container_backup_plans"}}, true},
		{"two keys to one parent are refused", "gitops_pipeline_stages", "gitops_promotions",
			map[string][]string{"gitops_promotions": {"gitops_pipeline_stages"}}, true},
		{"a self-reference is a cycle", "container_backup_runs", "container_backup_runs", nil, true},
		{"an unrelated parent is allowed", "users", "container_backup_runs",
			map[string][]string{"container_backup_runs": {"container_backup_plans"}}, false},
		{"a new edge above an existing child's other parent is refused", "environments", "container_backup_plans_x",
			map[string][]string{"container_backup_runs": {"container_backup_plans_x", "environments"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parents := map[string][]string{}
			for k, v := range graph {
				parents[k] = append([]string(nil), v...)
			}
			for k, v := range tc.kept {
				parents[k] = append(parents[k], v...)
			}
			if got := cascadeConflicts(parents, tc.parent, tc.child); got != tc.want {
				t.Fatalf("cascadeConflicts(%s → %s) = %v, want %v", tc.parent, tc.child, got, tc.want)
			}
		})
	}
}

// TestSplitTopLevelKeepsNestedCommas guards the item split the resolver edits:
// a comma inside a type, a default or a literal is not an item boundary.
func TestSplitTopLevelKeepsNestedCommas(t *testing.T) {
	got := splitTopLevel("a NVARCHAR(450), b NVARCHAR(23) DEFAULT (CONVERT(nvarchar(23), SYSUTCDATETIME(), 126) + 'Z'), c NVARCHAR(MAX) DEFAULT 'x, y'")
	if len(got) != 3 || !strings.Contains(got[1], "126) + 'Z')") || !strings.Contains(got[2], "'x, y'") {
		t.Fatalf("split %q", got)
	}
}

// TestMSSQLCascadePathsBecomeTriggers runs the resolver against a real SQL Server:
// the table SQL Server would refuse is created, and deleting the grandparent still
// removes its row, as SQLite's cascade does.
func TestMSSQLCascadePathsBecomeTriggers(t *testing.T) {
	dsn := os.Getenv("MCHARBOR_TEST_MSSQL_DSN")
	if dsn == "" {
		t.Skip("MCHARBOR_TEST_MSSQL_DSN not set")
	}
	database, err := openMSSQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	})
	ctx := context.Background()
	drop := func() {
		for _, table := range []string{"cpt_run", "cpt_plan", "cpt_env"} {
			if _, err := database.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
				t.Fatal(err)
			}
		}
	}
	drop()
	t.Cleanup(drop)

	body := `CREATE TABLE IF NOT EXISTS cpt_env (id TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS cpt_plan (
    id TEXT PRIMARY KEY,
    env_id TEXT NOT NULL REFERENCES cpt_env(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS cpt_run (
    id TEXT PRIMARY KEY,
    plan_id TEXT REFERENCES cpt_plan(id) ON DELETE SET NULL,
    env_id TEXT NOT NULL REFERENCES cpt_env(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_cpt_run_env ON cpt_run(env_id);
INSERT INTO cpt_env (id) VALUES ('e1');
INSERT INTO cpt_plan (id, env_id) VALUES ('p1', 'e1');
INSERT INTO cpt_run (id, plan_id, env_id) VALUES ('r1', 'p1', 'e1');
INSERT INTO cpt_run (id, plan_id, env_id) VALUES ('r2', NULL, 'e1');`
	if err := execMigrationBody(ctx, database, DriverMSSQL, "cascade_paths_test.sql", body); err != nil {
		t.Fatalf("SQL Server refused the rebuilt cascade: %v", err)
	}
	if _, err := database.ExecContext(ctx, "DELETE FROM cpt_env WHERE id = 'e1'"); err != nil {
		t.Fatalf("deleting the environment: %v", err)
	}
	var left int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM cpt_run").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d run rows survived their environment; the trigger did not carry the cascade", left)
	}
}
