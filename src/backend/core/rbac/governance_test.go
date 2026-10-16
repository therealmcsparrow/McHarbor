// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func createGovernanceTables(t *testing.T, db *sql.DB) {
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
	expires_at TEXT
);
CREATE TABLE break_glass_sessions (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	reason TEXT NOT NULL,
	started_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	ended_at TEXT,
	ended_by TEXT
);`); err != nil {
		t.Fatalf("creating governance tables: %v", err)
	}
}

func openSession(t *testing.T, db *sql.DB, id, userID string, until time.Time) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO break_glass_sessions (id, user_id, reason, started_at, expires_at) VALUES (?, ?, 'outage', ?, ?)`,
		id, userID, time.Now().UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
}

// TestBreakGlassGrantsEverythingOnlyWhileOpen pins the emergency grant to its
// window: open means wildcard, expired or ended means back to the roles.
func TestBreakGlassGrantsEverythingOnlyWhileOpen(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	createGovernanceTables(t, db)
	svc := NewService(db)
	ctx := context.Background()

	check := func() bool {
		ok, err := svc.HasPermission(ctx, "bg", "env-1", PermEnvironmentsManage)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if check() {
		t.Fatal("no session, no roles: must be denied")
	}
	openSession(t, db, "s1", "bg", time.Now().Add(-time.Minute))
	if check() {
		t.Fatal("an expired session must not grant")
	}
	openSession(t, db, "s2", "bg", time.Now().Add(30*time.Minute))
	if !check() {
		t.Fatal("an open session must grant")
	}
	if _, err := db.Exec(`UPDATE break_glass_sessions SET ended_at = ? WHERE id = 's2'`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if check() {
		t.Fatal("an ended session must stop granting at once")
	}
	if ok, err := svc.HasPermission(WithAPIKey(ctx, &APIKeyPrincipal{KeyID: "k"}), "bg", "", PermEnvironmentsManage); err != nil || ok {
		t.Fatalf("break-glass must never reach an API key: %v %v", ok, err)
	}
}

// TestExplainNamesTheGrantingRoleAndAgreesWithTheGate checks the simulator's
// decision comes from the live gate and that its reasons name the real source.
func TestExplainNamesTheGrantingRoleAndAgreesWithTheGate(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	createGovernanceTables(t, db)
	seedRBACRole(t, db, "role_ops", `["containers.delete"]`)
	seedRBACRole(t, db, "role_admin", `["*"]`)
	seedRBACUserRole(t, db, "ur-1", "alice", "role_ops", "env-1", "")
	svc := NewService(db)
	ctx := context.Background()

	got, err := svc.Explain(ctx, ExplainRequest{UserID: "alice", EnvironmentID: "env-1", Permission: PermContainersDelete})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Allowed || len(got.Reasons) != 1 || got.Reasons[0].Kind != ReasonRole || got.Reasons[0].ID != "role_ops" {
		t.Fatalf("explanation: %+v", got)
	}
	other, err := svc.Explain(ctx, ExplainRequest{UserID: "alice", EnvironmentID: "env-2", Permission: PermContainersDelete})
	if err != nil {
		t.Fatal(err)
	}
	if other.Allowed || other.Reasons[0].Kind != ReasonNoGrant {
		t.Fatalf("env-scoped role leaked into another environment: %+v", other)
	}
	gate, gateErr := svc.HasPermission(ctx, "alice", "env-2", PermContainersDelete)
	if gateErr != nil {
		t.Fatalf("live gate: %v", gateErr)
	}
	if gate != other.Allowed {
		t.Fatal("simulator disagrees with the live gate")
	}

	seedRBACUserRole(t, db, "ur-2", "root", "role_admin", "", "")
	root, err := svc.Explain(ctx, ExplainRequest{UserID: "root", Permission: PermUsersManage, Scope: ScopeGlobal})
	if err != nil || !root.Allowed || root.Reasons[0].Kind != ReasonWildcardRole {
		t.Fatalf("wildcard: %+v %v", root, err)
	}

	if _, err := db.Exec(`INSERT INTO user_permissions (id, user_id, permission_key, effect) VALUES ('o1', 'alice', 'containers.delete', 'deny')`); err != nil {
		t.Fatal(err)
	}
	denied, err := svc.Explain(ctx, ExplainRequest{UserID: "alice", EnvironmentID: "env-1", Permission: PermContainersDelete})
	if err != nil {
		t.Fatal(err)
	}
	if denied.PolicyAllowed || !hasReason(denied.Reasons, ReasonOverrideDeny) {
		t.Fatalf("deny override not explained: %+v", denied)
	}
}

func hasReason(rs []ExplainReason, kind string) bool {
	for _, r := range rs {
		if r.Kind == kind {
			return true
		}
	}
	return false
}

func TestDetectConflicts(t *testing.T) {
	roles := []RoleGrantSet{
		{ID: "role_admin", Name: "Admin", Permissions: []string{"*"}, IsSystem: true},
		{ID: "r1", Name: "Ops", Permissions: []string{"containers.*", "stacks.view"}},
		{ID: "r2", Name: "Custom all", Permissions: []string{"*"}},
		{ID: "r3", Name: "Plain", Permissions: []string{"containers.view"}},
	}
	policies := []ABACPolicy{
		{ID: "p1", Name: "allow all prod", Effect: "allow", Expression: "true;", Priority: 10, Enabled: true, Permissions: []string{"containers.delete"}},
		{ID: "p2", Name: "deny prod", Effect: "deny", Expression: `env.labels.tier === "prod"`, Priority: 20, Enabled: true, Permissions: []string{"containers.delete", "stacks.delete"}},
		{ID: "p3", Name: "tie a", Effect: "allow", Expression: "user.id === 'a'", Priority: 30, Enabled: true, Permissions: []string{"stacks.delete"}},
		{ID: "p4", Name: "tie b", Effect: "deny", Expression: "user.id === 'b'", Priority: 30, Enabled: true, Permissions: []string{"stacks.delete"}},
		{ID: "p5", Name: "disabled twin", Effect: "deny", Expression: "true", Priority: 1, Enabled: false, Permissions: []string{"stacks.delete"}},
		{ID: "p6", Name: "scoped", Effect: "deny", Expression: "true", Priority: 5, Enabled: true, ResourceType: "stack", Permissions: []string{"containers.delete"}},
	}
	got := DetectConflicts(roles, policies)
	want := map[string]string{
		"role_admin": ConflictWildcardGrant + ":info",
		"r1":         ConflictCategoryWildcard + ":warning",
		"r2":         ConflictWildcardGrant + ":warning",
		"p2":         ConflictPolicyShadowed + ":warning",
		"p4":         ConflictPolicyTie + ":info",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d findings, want %d: %+v", len(got), len(want), got)
	}
	for _, w := range got {
		if want[w.SubjectID] != w.Kind+":"+w.Severity {
			t.Fatalf("unexpected finding %+v", w)
		}
		if w.SubjectID == "p2" && (w.RelatedID != "p1" || len(w.Permissions) != 1 || w.Permissions[0] != "containers.delete") {
			t.Fatalf("shadow should name p1 over containers.delete only: %+v", w)
		}
	}
}
