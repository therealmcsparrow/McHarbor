// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

func TestHasPermissionIgnoresStackScopedAssignments(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	seedRBACRole(t, db, "role_stack_view", `["stacks.view"]`)
	seedRBACUserRole(t, db, "ur-1", "user-1", "role_stack_view", "env-1", "whoami")

	svc := NewService(db)

	allowed, err := svc.HasPermission("user-1", "env-1", PermStacksView)
	if err != nil {
		t.Fatalf("HasPermission returned error: %v", err)
	}
	if allowed {
		t.Fatal("expected stack-scoped assignment to be ignored for environment permission checks")
	}
}

func TestHasStackPermissionIncludesEnvironmentAndStackAssignments(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	seedRBACRole(t, db, "role_env_view", `["stacks.view"]`)
	seedRBACRole(t, db, "role_stack_manage", `["stacks.manage"]`)
	seedRBACUserRole(t, db, "ur-1", "user-1", "role_env_view", "env-1", "")
	seedRBACUserRole(t, db, "ur-2", "user-1", "role_stack_manage", "env-1", "whoami")

	svc := NewService(db)

	viewAllowed, err := svc.HasStackPermission("user-1", "env-1", "whoami", PermStacksView)
	if err != nil {
		t.Fatalf("HasStackPermission(view) returned error: %v", err)
	}
	if !viewAllowed {
		t.Fatal("expected environment-scoped stacks.view to apply to stack route")
	}

	manageAllowed, err := svc.HasStackPermission("user-1", "env-1", "whoami", PermStacksManage)
	if err != nil {
		t.Fatalf("HasStackPermission(manage) returned error: %v", err)
	}
	if !manageAllowed {
		t.Fatal("expected matching stack-scoped stacks.manage to apply")
	}

	otherManageAllowed, err := svc.HasStackPermission("user-1", "env-1", "uptime-kuma", PermStacksManage)
	if err != nil {
		t.Fatalf("HasStackPermission(other manage) returned error: %v", err)
	}
	if otherManageAllowed {
		t.Fatal("expected stack-scoped manage permission to be limited to the assigned stack")
	}
}

func TestAllowedStackNamesIncludesDirectAndGroupAssignments(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	seedRBACRole(t, db, "role_stack_view", `["stacks.view"]`)
	seedRBACUserRole(t, db, "ur-1", "user-1", "role_stack_view", "env-1", "whoami")
	seedRBACGroupMembership(t, db, "gm-1", "group-1", "user-1")
	seedRBACGroupRole(t, db, "gr-1", "group-1", "role_stack_view", "env-1", "uptime-kuma")

	svc := NewService(db)

	stackNames, err := svc.AllowedStackNames("user-1", "env-1", PermStacksView)
	if err != nil {
		t.Fatalf("AllowedStackNames returned error: %v", err)
	}

	expected := []string{"uptime-kuma", "whoami"}
	if !reflect.DeepEqual(stackNames, expected) {
		t.Fatalf("expected stack names %#v, got %#v", expected, stackNames)
	}

	anyAllowed, err := svc.HasAnyStackPermission("user-1", "env-1", PermStacksView)
	if err != nil {
		t.Fatalf("HasAnyStackPermission returned error: %v", err)
	}
	if !anyAllowed {
		t.Fatal("expected stack-scoped permissions to allow stack list access")
	}
}

func openRBACTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "rbac-test.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}

func createRBACTestTables(t *testing.T, db *sql.DB) {
	t.Helper()

	if _, err := db.Exec(`
CREATE TABLE users (
	id TEXT PRIMARY KEY,
	username TEXT NOT NULL
);

CREATE TABLE roles (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	permissions TEXT NOT NULL
);

CREATE TABLE user_roles (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	role_id TEXT NOT NULL,
	environment_id TEXT,
	stack_name TEXT,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE groups (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL
);

CREATE TABLE group_members (
	id TEXT PRIMARY KEY,
	group_id TEXT NOT NULL,
	user_id TEXT NOT NULL
);

CREATE TABLE group_roles (
	id TEXT PRIMARY KEY,
	group_id TEXT NOT NULL,
	role_id TEXT NOT NULL,
	environment_id TEXT,
	stack_name TEXT,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`); err != nil {
		t.Fatalf("creating rbac test tables: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO users (id, username) VALUES ('user-1', 'carlo')`); err != nil {
		t.Fatalf("seeding users table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO groups (id, name) VALUES ('group-1', 'Operators')`); err != nil {
		t.Fatalf("seeding groups table: %v", err)
	}
}

func seedRBACRole(t *testing.T, db *sql.DB, id, permissions string) {
	t.Helper()

	if _, err := db.Exec(`INSERT INTO roles (id, name, permissions) VALUES (?, ?, ?)`, id, id, permissions); err != nil {
		t.Fatalf("seeding role %s: %v", id, err)
	}
}

func seedRBACUserRole(t *testing.T, db *sql.DB, id, userID, roleID, environmentID, stackName string) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO user_roles (id, user_id, role_id, environment_id, stack_name) VALUES (?, ?, ?, ?, ?)`,
		id, userID, roleID, nullableString(environmentID), nullableString(stackName),
	); err != nil {
		t.Fatalf("seeding user role %s: %v", id, err)
	}
}

func seedRBACGroupMembership(t *testing.T, db *sql.DB, id, groupID, userID string) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO group_members (id, group_id, user_id) VALUES (?, ?, ?)`,
		id, groupID, userID,
	); err != nil {
		t.Fatalf("seeding group membership %s: %v", id, err)
	}
}

func seedRBACGroupRole(t *testing.T, db *sql.DB, id, groupID, roleID, environmentID, stackName string) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO group_roles (id, group_id, role_id, environment_id, stack_name) VALUES (?, ?, ?, ?, ?)`,
		id, groupID, roleID, nullableString(environmentID), nullableString(stackName),
	); err != nil {
		t.Fatalf("seeding group role %s: %v", id, err)
	}
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}

	return value
}
