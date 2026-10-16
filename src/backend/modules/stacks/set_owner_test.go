// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// TestSetOwnerChangesOnlyThatEnvironmentsStack guards the owner update: stack
// names repeat across environments, and the owner grants owner-scoped access,
// so changing envA's "web" must not hand envB's "web" to the same person.
func TestSetOwnerChangesOnlyThatEnvironmentsStack(t *testing.T) {
	svc := newListTestService(t)
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('alice', 'alice', 'x')`,
		`INSERT INTO environments (id, name) VALUES ('envA', 'A'), ('envB', 'B')`,
		`INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, created_at, updated_at)
		 VALUES ('sa', 'web', 'envA', '/data/a', 'docker-compose.yml', 'unknown', '2026-09-29T00:00:00Z', '2026-09-29T00:00:00Z'),
		        ('sb', 'web', 'envB', '/data/b', 'docker-compose.yml', 'unknown', '2026-09-29T00:00:00Z', '2026-09-29T00:00:00Z')`,
	} {
		if _, err := svc.db.Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := svc.SetOwner(context.Background(), "envA", "web", "alice"); err != nil {
		t.Fatal(err)
	}
	owner := func(id string) string {
		var o sql.NullString
		if err := svc.db.QueryRow(`SELECT owner_user_id FROM stacks WHERE id = ?`, id).Scan(&o); err != nil {
			t.Fatal(err)
		}
		return o.String
	}
	if owner("sa") != "alice" {
		t.Fatalf("envA's stack: owner %q, want alice", owner("sa"))
	}
	if owner("sb") != "" {
		t.Fatalf("envB's same-name stack must keep its owner, got %q", owner("sb"))
	}
	if err := svc.SetOwner(context.Background(), "envC", "web", "alice"); !errors.Is(err, ErrStackNotFound) {
		t.Fatalf("a stack absent from the environment: got %v, want ErrStackNotFound", err)
	}
}
