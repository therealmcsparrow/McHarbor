// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"path/filepath"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// newListTestService builds a stacks service over a fully migrated database.
// The migration is the point: the bug this file guards was a SELECT and its
// scanner disagreeing about how many columns the real schema has.
func newListTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()

	database, err := coredb.OpenSQLite(filepath.Join(dir, "stacks_list_test.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	if _, err := database.Exec(
		"INSERT INTO environments (id, name) VALUES ('envAAA', 'Test Env')",
	); err != nil {
		t.Fatalf("seeding environment: %v", err)
	}

	enc, err := encryption.New(dir, "")
	if err != nil {
		t.Fatalf("creating encryption service: %v", err)
	}
	return NewService(database, nil, dir, enc)
}

func seedStack(t *testing.T, svc *Service, id, name string, fileMode bool) {
	t.Helper()
	mode := 0
	if fileMode {
		mode = 1
	}
	if _, err := svc.db.Exec(`
		INSERT INTO stacks (id, name, environment_id, project_path, compose_file,
			status, created_at, updated_at, secret_file_mode)
		VALUES (?, ?, 'envAAA', ?, 'docker-compose.yml', 'unknown',
			'2026-09-14T00:00:00Z', '2026-09-14T00:00:00Z', ?)
	`, id, name, "/data/"+name, mode); err != nil {
		t.Fatalf("seeding stack %s: %v", name, err)
	}
}

// TestListReturnsStacksFromTheDatabase is the regression test for a 500 that
// emptied the Stacks page for every environment.
//
// `listFromDB` selects twelve columns; `scanStack` scanned eleven, because
// migration 117 added `secret_file_mode` and only the single-row scanner was
// updated. Every List call failed with "expected 12 destination arguments in
// Scan, not 11", and the page rendered as though there were no stacks at all.
//
// Docker is nil here, so discovery fails and is swallowed as designed — which
// is exactly the path a real list takes when a daemon is unreachable, and it
// must still return the managed rows.
func TestListReturnsStacksFromTheDatabase(t *testing.T) {
	svc := newListTestService(t)
	ctx := context.Background()

	seedStack(t, svc, "stk1", "wordpress", true)
	seedStack(t, svc, "stk2", "homeassistant", false)

	stacks, err := svc.List(ctx, "envAAA")
	if err != nil {
		t.Fatalf("listing stacks: %v", err)
	}
	if len(stacks) != 2 {
		t.Fatalf("expected both seeded stacks, got %d: %+v", len(stacks), stacks)
	}

	byName := map[string]Stack{}
	for _, st := range stacks {
		byName[st.Name] = st
	}
	if !byName["wordpress"].SecretFileMode {
		t.Error("file mode must survive the list scan, not default to false")
	}
	if byName["homeassistant"].SecretFileMode {
		t.Error("a stack without file mode must not come back with it set")
	}
}

// TestListAndByNameAgree pins the two scanners to each other. They read the
// same table through different code paths, and the last time they drifted the
// list broke while the detail page kept working, which is what made it hard to
// spot.
func TestListAndByNameAgree(t *testing.T) {
	svc := newListTestService(t)

	seedStack(t, svc, "stk1", "wordpress", true)

	listed, err := svc.List(context.Background(), "envAAA")
	if err != nil {
		t.Fatalf("listing stacks: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one stack, got %d", len(listed))
	}

	detail, err := svc.ByName(context.Background(), "wordpress")
	if err != nil {
		t.Fatalf("loading stack by name: %v", err)
	}
	if detail == nil {
		t.Fatal("ByName returned no stack")
	}

	if listed[0].ID != detail.ID {
		t.Errorf("id mismatch: list %s, detail %s", listed[0].ID, detail.ID)
	}
	if listed[0].SecretFileMode != detail.SecretFileMode {
		t.Errorf("secret file mode mismatch: list %v, detail %v",
			listed[0].SecretFileMode, detail.SecretFileMode)
	}
	if listed[0].Orchestrator != detail.Orchestrator {
		t.Errorf("orchestrator mismatch: list %s, detail %s",
			listed[0].Orchestrator, detail.Orchestrator)
	}
}
