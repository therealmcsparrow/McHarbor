// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// migratedRBACDB is a fully migrated database, because the wildcard path reads
// plugin_permissions, which the minimal RBAC test schema does not create.
func migratedRBACDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "scope.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if err := UpsertCatalog(database); err != nil {
		t.Fatal(err)
	}
	return database
}

func mustExec(t *testing.T, database *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := database.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func assertHas(t *testing.T, svc *Service, env string, perm Permission, want bool) {
	t.Helper()
	got, err := svc.HasPermission(context.Background(), "u1", env, perm)
	if err != nil {
		t.Fatalf("HasPermission(%q, %s): %v", env, perm, err)
	}
	if got != want {
		t.Fatalf("HasPermission(%q, %s) = %v, want %v", env, perm, got, want)
	}
}

// TestEnvironmentGrantCarriesNoGlobalKeys is the escalation this guards: a role
// assigned on one environment granted users.manage to any request naming that
// environment, so an environment admin could assign themselves role_admin.
func TestEnvironmentGrantCarriesNoGlobalKeys(t *testing.T) {
	database := migratedRBACDB(t)
	mustExec(t, database, `INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`)
	mustExec(t, database, `INSERT INTO roles (id, name, permissions) VALUES ('mixed', 'mixed', '["users.manage","containers.manage"]')`)
	mustExec(t, database, `INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'u1', 'mixed', 'envA')`)
	svc := NewService(database)

	assertHas(t, svc, "envA", PermUsersManage, false)
	assertHas(t, svc, "", PermUsersManage, false)
	assertHas(t, svc, "envA", PermContainersManage, true)
	assertHas(t, svc, "envB", PermContainersManage, false)
}

func TestWildcardOnAnEnvironmentGrantIsEnvironmentKeysOnly(t *testing.T) {
	database := migratedRBACDB(t)
	mustExec(t, database, `INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`)
	mustExec(t, database, `INSERT INTO roles (id, name, permissions) VALUES ('all', 'all', '["*"]')`)
	mustExec(t, database, `INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'u1', 'all', 'envA')`)
	svc := NewService(database)

	assertHas(t, svc, "envA", PermContainersManage, true)
	for _, global := range []Permission{PermUsersManage, PermRolesManage, PermPluginsManage, PermEnvironmentsManage} {
		assertHas(t, svc, "envA", global, false)
	}
}

func TestGlobalGrantIsUnchanged(t *testing.T) {
	database := migratedRBACDB(t)
	mustExec(t, database, `INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`)
	mustExec(t, database, `INSERT INTO roles (id, name, permissions) VALUES ('admin', 'admin', '["users.manage"]')`)
	mustExec(t, database, `INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur1', 'u1', 'admin')`)
	svc := NewService(database)

	assertHas(t, svc, "", PermUsersManage, true)
	assertHas(t, svc, "envA", PermUsersManage, true)
}

func TestEnvironmentElevationCarriesNoGlobalKey(t *testing.T) {
	database := migratedRBACDB(t)
	mustExec(t, database, `INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`)
	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	for id, perm := range map[string]string{"e1": "users.manage", "e2": "containers.manage"} {
		mustExec(t, database, `INSERT INTO elevation_requests (id, requester_id, permission_key, status, environment_id, approved_until)
			VALUES (?, 'u1', ?, 'approved', 'envA', ?)`, id, perm, until)
	}
	svc := NewService(database)

	assertHas(t, svc, "envA", PermUsersManage, false)
	assertHas(t, svc, "envA", PermContainersManage, true)
}
