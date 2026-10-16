// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dashboard

import (
	"context"
	"path/filepath"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func newStatsTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()

	database, err := coredb.OpenSQLite(filepath.Join(dir, "dashboard_test.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	for _, id := range []string{"envAAA", "envBBB"} {
		if _, err := database.Exec(
			"INSERT INTO environments (id, name) VALUES (?, ?)", id, id,
		); err != nil {
			t.Fatalf("seeding environment %s: %v", id, err)
		}
	}

	// No Docker pool: stackCount reads the database and nothing else, which is
	// the whole point of testing it apart from Stats.
	return NewService(database, nil)
}

func seedStack(t *testing.T, svc *Service, id, name string, envID any) {
	t.Helper()
	if _, err := svc.db.Exec(`
		INSERT INTO stacks (id, name, environment_id, project_path, compose_file,
			status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'docker-compose.yml', 'unknown',
			'2026-09-14T00:00:00Z', '2026-09-14T00:00:00Z')
	`, id, name, envID, "/data/"+name); err != nil {
		t.Fatalf("seeding stack %s: %v", name, err)
	}
}

// TestStackCountIsScopedToTheEnvironment is the regression test for a figure
// that counted every stack in the database regardless of environment.
//
// Every other number on the dashboard comes from the selected environment's
// Docker daemon, so the unscoped count made the sidebar badge contradict the
// Stacks page it links to — five stacks listed, six counted.
func TestStackCountIsScopedToTheEnvironment(t *testing.T) {
	svc := newStatsTestService(t)
	ctx := context.Background()

	seedStack(t, svc, "stk1", "wordpress", "envAAA")
	seedStack(t, svc, "stk2", "homeassistant", "envAAA")
	seedStack(t, svc, "stk3", "eufy", "envBBB")

	if got := svc.stackCount(ctx, "envAAA"); got != 2 {
		t.Errorf("envAAA should count its own two stacks, got %d", got)
	}
	if got := svc.stackCount(ctx, "envBBB"); got != 1 {
		t.Errorf("envBBB should count its own stack, got %d", got)
	}
}

// TestStackCountIncludesUnattachedStacks: a discovered stack recorded without
// an environment is visible from every environment, so hiding it from the
// count would under-report against the list that shows it.
func TestStackCountIncludesUnattachedStacks(t *testing.T) {
	svc := newStatsTestService(t)
	ctx := context.Background()

	seedStack(t, svc, "stk1", "wordpress", "envAAA")
	seedStack(t, svc, "stk2", "orphan", nil)

	if got := svc.stackCount(ctx, "envAAA"); got != 2 {
		t.Errorf("an unattached stack counts in every environment, got %d", got)
	}
	if got := svc.stackCount(ctx, "envBBB"); got != 1 {
		t.Errorf("envBBB should still see the unattached stack, got %d", got)
	}
}

// TestStackCountUnscopedCountsEverything keeps the "no environment selected"
// path honest: that is what an empty envID means everywhere else in the
// codebase, and stacks.listFromDB already reads it the same way.
func TestStackCountUnscopedCountsEverything(t *testing.T) {
	svc := newStatsTestService(t)
	ctx := context.Background()

	seedStack(t, svc, "stk1", "wordpress", "envAAA")
	seedStack(t, svc, "stk2", "eufy", "envBBB")
	seedStack(t, svc, "stk3", "orphan", nil)

	if got := svc.stackCount(ctx, ""); got != 3 {
		t.Errorf("an unscoped count should total everything, got %d", got)
	}
}
