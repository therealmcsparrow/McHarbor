// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"testing"
)

func containerIDOf(t *testing.T, database *sql.DB, table, id string) string {
	t.Helper()
	var got string
	// SQL identifier: table is one of two literals passed by the tests below.
	if err := database.QueryRow("SELECT container_id FROM "+table+" WHERE id = ?", id).Scan(&got); err != nil {
		t.Fatalf("reading %s %s: %v", table, id, err)
	}
	return got
}

func TestRelinkContainerFollowsTheNameWithinTheEnvironment(t *testing.T) {
	svc, database := newStageService(t)
	const now = "2026-10-10T00:00:00Z"
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := database.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	plan := `INSERT INTO container_backup_plans (id, name, environment_id, container_id, container_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	exec(plan, "p-web", "web", "env1", "old", "web", now, now)
	exec(plan, "p-slash", "web2", "env1", "old", "/web", now, now)
	exec(plan, "p-other-env", "web", "env2", "old", "web", now, now)
	exec(plan, "p-db", "db", "env1", "dbid", "db", now, now)

	run := `INSERT INTO container_backup_runs (id, plan_id, environment_id, container_id, container_name, status, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'success', ?, ?, ?)`
	exec(run, "r-named", nil, "env1", "old", "web", now, now, now)
	exec(run, "r-via-plan", "p-web", "env1", "old", "", now, now, now)
	exec(run, "r-db", "p-db", "env1", "dbid", "db", now, now, now)

	n, err := svc.RelinkContainer(context.Background(), "env1", "new", "/web")
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("relinked %d rows, want 4", n)
	}
	for table, ids := range map[string]map[string]string{
		"container_backup_plans": {"p-web": "new", "p-slash": "new", "p-other-env": "old", "p-db": "dbid"},
		"container_backup_runs":  {"r-named": "new", "r-via-plan": "new", "r-db": "dbid"},
	} {
		for id, want := range ids {
			if got := containerIDOf(t, database, table, id); got != want {
				t.Errorf("%s %s: container_id %q, want %q", table, id, got, want)
			}
		}
	}

	n, err = svc.RelinkContainer(context.Background(), "env1", "new", "web")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("second relink touched %d rows, want 0", n)
	}
}
