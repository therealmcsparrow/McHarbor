// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func seedServicePrincipal(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(`ALTER TABLE users ADD COLUMN principal_kind TEXT NOT NULL DEFAULT 'user'`); err != nil {
		t.Fatalf("adding principal_kind: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, username, principal_kind) VALUES (?, ?, 'service')`, id, id); err != nil {
		t.Fatalf("seeding service principal: %v", err)
	}
}

// TestElevationNeverFoldsIntoAServicePrincipal is the property decision 1 of 07c
// was conditional on. An elevation is approved for a person in a session; an
// unattended runbook acting as a service principal must hold exactly its roles.
// The same approved row still elevates an ordinary user, so the fence is the
// principal kind and not a broken elevation lookup.
func TestElevationNeverFoldsIntoAServicePrincipal(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	seedServicePrincipal(t, db, "svc-runbooks")
	until := time.Now().Add(15 * time.Minute)
	seedApprovedElevation(t, db, "svc-runbooks", PermContainersManage, until)
	seedApprovedElevation(t, db, "user-1", PermContainersManage, until)

	svc := NewService(db)
	ctx := context.Background()

	allowed, err := svc.HasPermission(ctx, "svc-runbooks", "env-1", PermContainersManage)
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if allowed {
		t.Fatal("an approved elevation folded into a service principal; it must never")
	}
	human, err := svc.HasPermission(ctx, "user-1", "env-1", PermContainersManage)
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if !human {
		t.Fatal("the same elevation must still elevate an ordinary user")
	}
	grants, err := svc.activeElevations(ctx, "svc-runbooks", "env-1")
	if err != nil {
		t.Fatalf("activeElevations: %v", err)
	}
	if len(grants) != 0 {
		t.Fatalf("the explainer would list %d elevation(s) for a service principal", len(grants))
	}
}

// TestServicePrincipalKeepsItsRoles pins the other half: the principal is an
// ordinary grantee, so what a role editor assigns it is exactly what it holds.
func TestServicePrincipalKeepsItsRoles(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	seedServicePrincipal(t, db, "svc-runbooks")
	seedRBACRole(t, db, "role_ops", `["containers.manage"]`)
	seedRBACUserRole(t, db, "ur-1", "svc-runbooks", "role_ops", "", "")

	allowed, err := NewService(db).HasPermission(context.Background(), "svc-runbooks", "env-1", PermContainersManage)
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if !allowed {
		t.Fatal("a role assigned to a service principal must grant what it contains")
	}
}

// TestBreakGlassNeverReachesAServicePrincipal: an open break-glass row is the
// wildcard, and it must not attach to a principal that acts with nobody present.
func TestBreakGlassNeverReachesAServicePrincipal(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	seedServicePrincipal(t, db, "svc-runbooks")
	if _, err := db.Exec(`CREATE TABLE break_glass_sessions (id TEXT PRIMARY KEY, user_id TEXT, ended_at TEXT, expires_at TEXT)`); err != nil {
		t.Fatalf("creating break_glass_sessions: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO break_glass_sessions (id, user_id, ended_at, expires_at) VALUES ('bg-1', 'svc-runbooks', NULL, ?)`,
		time.Now().Add(time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("seeding break-glass: %v", err)
	}

	allowed, err := NewService(db).HasPermission(context.Background(), "svc-runbooks", "env-1", PermSecretsReveal)
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if allowed {
		t.Fatal("a break-glass row made a service principal a wildcard")
	}
}
