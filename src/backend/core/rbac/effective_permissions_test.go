// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func createOverrideTable(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`
CREATE TABLE user_permissions (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	permission_key TEXT NOT NULL,
	effect TEXT NOT NULL,
	environment_id TEXT,
	stack_name TEXT,
	resource_type TEXT,
	resource_id TEXT,
	granted_by TEXT NOT NULL DEFAULT 'admin',
	expires_at TEXT,
	reason TEXT
)`); err != nil {
		t.Fatalf("creating user_permissions: %v", err)
	}
}

func seedOverride(t *testing.T, db *sql.DB, id, perm, effect, envID, resourceID string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO user_permissions (id, user_id, permission_key, effect, environment_id, resource_id) VALUES (?, 'user-1', ?, ?, ?, ?)`,
		id, perm, effect, nullableString(envID), nullableString(resourceID),
	); err != nil {
		t.Fatalf("seeding override %s: %v", id, err)
	}
}

func explanationByKey(e *PermissionExplanation) map[Permission]ExplainedPermission {
	out := make(map[Permission]ExplainedPermission, len(e.Permissions))
	for _, p := range e.Permissions {
		out[p.Key] = p
	}
	return out
}

func TestExplainPermissionsAgreesWithEffectivePermissions(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	createOverrideTable(t, db)
	seedRBACRole(t, db, "role_viewer", `["containers.view","images.view"]`)
	seedRBACRole(t, db, "role_env_ops", `["containers.manage"]`)
	seedRBACRole(t, db, "role_stack", `["stacks.manage"]`)
	seedRBACUserRole(t, db, "ur-1", "user-1", "role_viewer", "", "")
	seedRBACGroupMembership(t, db, "gm-1", "group-1", "user-1")
	seedRBACGroupRole(t, db, "gr-1", "group-1", "role_env_ops", "env-1", "")
	seedRBACUserRole(t, db, "ur-2", "user-1", "role_stack", "env-1", "web")
	until := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO elevation_requests (id, requester_id, permission_key, status, environment_id, approved_until) VALUES ('el-1', 'user-1', 'volumes.delete', 'approved', NULL, ?)`, until); err != nil {
		t.Fatalf("seeding elevation: %v", err)
	}

	svc := NewService(db)
	ctx := context.Background()
	scopes := []struct{ env, stack string }{{"", ""}, {"env-1", ""}, {"env-2", ""}, {"env-1", "web"}, {"env-1", "api"}}
	for _, scope := range scopes {
		explanation, err := svc.ExplainPermissions(ctx, "user-1", scope.env, scope.stack, nil)
		if err != nil {
			t.Fatalf("ExplainPermissions(%q,%q): %v", scope.env, scope.stack, err)
		}
		for _, entry := range CatalogSeed() {
			var want bool
			if scope.stack != "" {
				want, err = svc.HasStackPermission(ctx, "user-1", scope.env, scope.stack, entry.Key)
			} else {
				want, err = svc.HasPermission(ctx, "user-1", scope.env, entry.Key)
			}
			if err != nil {
				t.Fatalf("resolution: %v", err)
			}
			got := explanationByKey(explanation)[entry.Key]
			if got.Effective != want {
				t.Fatalf("scope %q/%q key %s: explanation says %v, gate says %v", scope.env, scope.stack, entry.Key, got.Effective, want)
			}
			if want && len(got.Sources) == 0 {
				t.Fatalf("scope %q/%q key %s is effective with no source", scope.env, scope.stack, entry.Key)
			}
		}
	}

	env1, err := svc.ExplainPermissions(ctx, "user-1", "env-1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	manage := explanationByKey(env1)[PermContainersManage]
	if len(manage.Sources) != 1 || manage.Sources[0].Via != GrantSourceGroup || manage.Sources[0].GroupName != "Operators" || manage.Sources[0].EnvironmentID != "env-1" {
		t.Fatalf("containers.manage should come from the Operators group in env-1, got %+v", manage.Sources)
	}
	deleteVolumes := explanationByKey(env1)[PermVolumesDelete]
	if len(deleteVolumes.Sources) != 1 || deleteVolumes.Sources[0].Kind != SourceKindElevation || deleteVolumes.Sources[0].ElevationID != "el-1" {
		t.Fatalf("volumes.delete should come from elevation el-1, got %+v", deleteVolumes.Sources)
	}
	if len(env1.StackScopes) != 1 || env1.StackScopes[0] != "web" {
		t.Fatalf("expected stack scope web, got %v", env1.StackScopes)
	}
}

func TestExplainPermissionsWildcardRole(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	createOverrideTable(t, db)
	seedRBACRole(t, db, "role_admin", `["*"]`)
	seedRBACUserRole(t, db, "ur-1", "user-1", "role_admin", "", "")

	explanation, err := NewService(db).ExplainPermissions(context.Background(), "user-1", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !explanation.Wildcard {
		t.Fatal("expected wildcard flag")
	}
	for _, p := range explanation.Permissions {
		if !p.Effective || len(p.Sources) == 0 || !p.Sources[0].Wildcard {
			t.Fatalf("key %s should be effective through the wildcard role, got %+v", p.Key, p)
		}
	}
}

func TestOverridesMatchBeyondTheMostSpecificRow(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	createOverrideTable(t, db)
	seedOverride(t, db, "ov-env", string(PermImagesManage), "grant", "env-other", "")
	seedOverride(t, db, "ov-global", string(PermImagesManage), "grant", "", "")
	seedOverride(t, db, "ov-deny", string(PermImagesView), "deny", "", "")
	seedOverride(t, db, "ov-res", string(PermNetworksManage), "grant", "", "net-1")
	seedRBACRole(t, db, "role_viewer", `["images.view"]`)
	seedRBACUserRole(t, db, "ur-1", "user-1", "role_viewer", "", "")

	svc := NewService(db)
	allowed, err := svc.Authorize(AuthzRequest{UserID: "user-1", Permission: PermImagesManage, EnvironmentID: "env-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("the global grant override must apply when the more specific one names another environment")
	}

	explanation, err := svc.ExplainPermissions(context.Background(), "user-1", "env-1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	byKey := explanationByKey(explanation)
	if got := byKey[PermImagesManage]; got.Effective || !got.AuthorizeGrant {
		t.Fatalf("images.manage: gates do not read overrides, Authorize does; got %+v", got)
	}
	if got := byKey[PermImagesView]; !got.Effective || got.AuthorizeGrant {
		t.Fatalf("images.view: role grants it, deny override wins in Authorize; got %+v", got)
	}
	network := byKey[PermNetworksManage]
	if network.AuthorizeGrant || len(network.Sources) != 1 || !network.Sources[0].Conditional {
		t.Fatalf("a resource-bound override is conditional and changes no scope-wide verdict; got %+v", network)
	}
	authorizeView, err := svc.Authorize(AuthzRequest{UserID: "user-1", Permission: PermImagesView, EnvironmentID: "env-1"})
	if err != nil {
		t.Fatal(err)
	}
	if authorizeView != byKey[PermImagesView].AuthorizeGrant {
		t.Fatal("explanation and Authorize disagree on images.view")
	}
}
