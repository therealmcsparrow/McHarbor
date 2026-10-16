// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package governance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "g.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing the test database: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('dev', 'dev', 'x')`,
		`INSERT INTO environments (id, name) VALUES ('env-1', 'prod')`,
		`INSERT INTO roles (id, name, description, permissions, created_at, updated_at) VALUES ('r_stack', 'Stack deleter', '', '["stacks.delete"]', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id, stack_name, created_at, updated_at) VALUES ('ur', 'dev', 'r_stack', 'env-1', 'web', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %s: %v", q, err)
		}
	}
	return database
}

func routes() []apispec.Operation {
	return []apispec.Operation{
		{Method: "DELETE", Path: "/api/stacks/{name}/", Perms: []rbac.PermRequirement{{Keys: []rbac.Permission{rbac.PermStacksDelete}, Scope: rbac.ScopeStack}}},
		{Method: "POST", Path: "/api/stacks/adopt", Perms: []rbac.PermRequirement{{Keys: []rbac.Permission{rbac.PermStacksManage}, Scope: rbac.ScopeEnvironment}}},
		{Method: "POST", Path: "/api/stacks/{name}", Perms: []rbac.PermRequirement{{Keys: []rbac.Permission{rbac.PermStacksDelete}, Scope: rbac.ScopeStack}}},
	}
}

// TestSimulateReadsTheRouteGateAndTheRealDecision covers the stack-scoped case
// the simulator exists for: a grant on one stack allows that stack only.
func TestSimulateReadsTheRouteGateAndTheRealDecision(t *testing.T) {
	database := openDB(t)
	svc := NewService(database, rbac.NewService(database), routes)
	ctx := context.Background()

	got, err := svc.Simulate(ctx, SimulateInput{UserID: "dev", EnvironmentID: "env-1", Method: "DELETE", Path: "/api/stacks/web"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Allowed || got.Route != "/api/stacks/{name}/" || len(got.Gates) != 1 || got.Gates[0].Decisions[0].Reasons[0].ID != "r_stack" {
		t.Fatalf("stack web: %+v", got)
	}
	other, err := svc.Simulate(ctx, SimulateInput{UserID: "dev", EnvironmentID: "env-1", Method: "DELETE", Path: "/api/stacks/db"})
	if err != nil || other.Allowed {
		t.Fatalf("a grant on one stack must not reach another: %+v %v", other, err)
	}
	adopt, err := svc.Simulate(ctx, SimulateInput{UserID: "dev", EnvironmentID: "env-1", Method: "POST", Path: "/api/stacks/adopt"})
	if err != nil || adopt.Route != "/api/stacks/adopt" {
		t.Fatalf("a static segment must beat a parameter: %+v %v", adopt, err)
	}
	perm, err := svc.Simulate(ctx, SimulateInput{UserID: "dev", EnvironmentID: "env-1", Permission: "stacks.delete"})
	if err != nil || perm.Allowed {
		t.Fatalf("a stack-scoped grant is not an environment grant: %+v %v", perm, err)
	}
	if _, err := svc.Simulate(ctx, SimulateInput{UserID: "dev", Method: "GET", Path: "/api/nowhere"}); !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("unknown route: %v", err)
	}
	if _, err := svc.Simulate(ctx, SimulateInput{UserID: "dev"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty simulation: %v", err)
	}
}

func TestConflictsReadsRolesAndPolicies(t *testing.T) {
	database := openDB(t)
	svc := NewService(database, rbac.NewService(database), routes)
	out, err := svc.Conflicts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range out {
		if w.SubjectID == "role_admin" && w.Kind == rbac.ConflictWildcardGrant && w.Severity == "info" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the built-in admin wildcard should be reported as info: %+v", out)
	}
}

// TestRoleTemplatesReferenceKnownKeys keeps the presets migration honest: every
// key a template grants must exist in the catalog, and all six must be present.
func TestRoleTemplatesReferenceKnownKeys(t *testing.T) {
	database := openDB(t)
	known := map[string]bool{}
	for _, e := range rbac.CatalogSeed() {
		known[string(e.Key)] = true
	}
	for _, key := range []string{"viewer", "operator", "developer", "auditor", "security_admin", "platform_admin"} {
		var raw string
		if err := database.QueryRow(`SELECT permissions FROM role_presets WHERE key = ?`, key).Scan(&raw); err != nil {
			t.Fatalf("preset %s: %v", key, err)
		}
		var perms []string
		if err := json.Unmarshal([]byte(raw), &perms); err != nil || len(perms) == 0 {
			t.Fatalf("preset %s permissions: %v", key, err)
		}
		for _, p := range perms {
			if !known[p] {
				t.Fatalf("preset %s grants unknown key %q", key, p)
			}
			if p == "*" {
				t.Fatalf("preset %s must not be a wildcard", key)
			}
		}
	}
	if _, err := os.Stat("../../core/db/migrations/093_governance.sql"); err != nil {
		t.Fatalf("migration file: %v", err)
	}
}
