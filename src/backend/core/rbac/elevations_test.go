// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func seedApprovedElevation(t *testing.T, db *sql.DB, userID string, perm Permission, until time.Time) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO elevation_requests (id, requester_id, permission_key, status, environment_id, approved_until)
		 VALUES (?, ?, ?, 'approved', NULL, ?)`,
		"elev-"+userID+"-"+string(perm), userID, string(perm),
		until.UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seeding elevation: %v", err)
	}
}

// TestElevationGrantsRevealToInteractiveSession is the positive half: an
// approved, unexpired elevation is what makes secrets.reveal reachable at all,
// since the key is in no role preset.
func TestElevationGrantsRevealToInteractiveSession(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	seedApprovedElevation(t, db, "user-1", PermSecretsReveal, time.Now().Add(15*time.Minute))

	svc := NewService(db)

	allowed, err := svc.HasPermission(context.Background(), "user-1", "env-1", PermSecretsReveal)
	if err != nil {
		t.Fatalf("HasPermission returned error: %v", err)
	}
	if !allowed {
		t.Fatal("an approved, unexpired elevation should grant secrets.reveal")
	}
}

// TestExpiredElevationDoesNotGrantReveal pins the time bound. The default
// window is 15 minutes precisely because it expires.
func TestExpiredElevationDoesNotGrantReveal(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	seedApprovedElevation(t, db, "user-1", PermSecretsReveal, time.Now().Add(-time.Minute))

	svc := NewService(db)

	allowed, err := svc.HasPermission(context.Background(), "user-1", "env-1", PermSecretsReveal)
	if err != nil {
		t.Fatalf("HasPermission returned error: %v", err)
	}
	if allowed {
		t.Fatal("an expired elevation must not grant secrets.reveal")
	}
}

// TestElevationIsNotAppliedToAPIKeys is the property step 2.10(d) of the
// secrets plan asks to pin, and it is deliberate rather than incidental.
//
// An elevation is approved for a person in a session. Extending it to a
// long-lived non-interactive credential that happens to act as them is a wider
// grant than the approver agreed to — and `secrets.reveal` is the one endpoint
// that returns a plaintext credential. The effect is that the reveal endpoint
// is unreachable by an API key, full stop.
//
// If this test fails, something has started folding elevations into the
// API-key path. That is not a bug to be fixed by updating the expectation.
func TestElevationIsNotAppliedToAPIKeys(t *testing.T) {
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	seedApprovedElevation(t, db, "user-1", PermSecretsReveal, time.Now().Add(15*time.Minute))

	// The key is even granted the permission outright — the fence it hits is
	// the elevation exclusion, not a missing scope.
	seedRBACRole(t, db, "role_reveal", `["secrets.reveal"]`)
	svc := NewService(db)

	ctx := WithAPIKey(context.Background(), &APIKeyPrincipal{
		KeyID:   "key-1",
		RoleIDs: []string{"role_reveal"},
	})

	allowed, err := svc.HasPermission(ctx, "user-1", "env-1", PermSecretsReveal)
	if err != nil {
		t.Fatalf("HasPermission returned error: %v", err)
	}
	if allowed {
		t.Fatal("secrets.reveal must be unreachable on the API-key path, elevation or not")
	}
}

// TestSecretsRevealIsInNoRolePreset would be the natural place to assert the
// preset rule, but presets live in SQL rather than in Go. The `permissions`
// sanity check owns that assertion; this comment exists so the next person
// looking for it here finds the pointer instead of adding a second one.
func TestSecretsManageGlobalIsGlobalScoped(t *testing.T) {
	svc := NewService(nil)
	scope, ok := svc.scopeFor(PermSecretsManageGlobal)
	if !ok {
		t.Fatal("secrets.manage_global is missing from CatalogSeed()")
	}
	if scope != "global" {
		t.Fatalf("secrets.manage_global should be global-scoped, got %q", scope)
	}

	// The scope map's only behavioural use is the owner fast path in
	// authorize.go, so a `global` scope here cannot widen anything — it is
	// descriptive, for the role editor. This assertion records that the
	// remaining four keys are environment-scoped as designed.
	for _, perm := range []Permission{
		PermSecretsView, PermSecretsManage, PermSecretsDelete, PermSecretsReveal,
	} {
		scope, ok := svc.scopeFor(perm)
		if !ok {
			t.Errorf("%s is missing from CatalogSeed()", perm)
			continue
		}
		if scope != "environment" {
			t.Errorf("%s should be environment-scoped, got %q", perm, scope)
		}
	}
}
