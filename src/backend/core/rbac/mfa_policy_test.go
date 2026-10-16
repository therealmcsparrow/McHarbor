// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"testing"
)

func openMFAPolicyTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openRBACTestDB(t)
	createRBACTestTables(t, db)
	if _, err := db.Exec(`CREATE TABLE settings (id TEXT PRIMARY KEY, key TEXT NOT NULL UNIQUE, value TEXT, category TEXT)`); err != nil {
		t.Fatalf("creating settings: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, username) VALUES ('admin-1', 'root'), ('viewer-1', 'viewer')`); err != nil {
		t.Fatalf("seeding users: %v", err)
	}
	seedRBACRole(t, db, "role-admin", `["settings.manage","settings.view"]`)
	seedRBACRole(t, db, "role-viewer", `["containers.view","settings.view"]`)
	seedRBACUserRole(t, db, "ur-admin", "admin-1", "role-admin", "", "")
	seedRBACUserRole(t, db, "ur-viewer", "viewer-1", "role-viewer", "", "")
	return db
}

func setMFARequirement(t *testing.T, db *sql.DB, value string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO settings (id, key, value, category) VALUES ('s1', ?, ?, 'security')
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, SettingMFARequired, value); err != nil {
		t.Fatalf("setting requirement: %v", err)
	}
	InvalidateMFAPolicy()
}

func TestParseMFARequirement(t *testing.T) {
	cases := map[string]struct {
		want MFARequirement
		ok   bool
	}{
		"off":          {MFARequiredOff, true},
		" " + "Admins": {MFARequiredAdmins, true},
		"ALL":          {MFARequiredAll, true},
		"":             {MFARequiredOff, false},
		"yes":          {MFARequiredOff, false},
	}
	for raw, tc := range cases {
		got, ok := ParseMFARequirement(raw)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("ParseMFARequirement(%q) = %q,%v want %q,%v", raw, got, ok, tc.want, tc.ok)
		}
	}
}

func TestMFAPolicyAppliesTo(t *testing.T) {
	db := openMFAPolicyTestDB(t)
	policy := NewMFAPolicy(db, NewService(db))
	ctx := context.Background()

	check := func(label, userID string, want bool) {
		t.Helper()
		got, err := policy.AppliesTo(ctx, userID)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if got != want {
			t.Fatalf("%s: AppliesTo(%s) = %v, want %v", label, userID, got, want)
		}
	}

	check("missing row reads as off", "admin-1", false)

	setMFARequirement(t, db, "off")
	check("off", "admin-1", false)
	check("off", "viewer-1", false)

	setMFARequirement(t, db, "admins")
	check("admins covers a settings.manage holder", "admin-1", true)
	check("admins skips a non-holder", "viewer-1", false)
	check("admins never covers the synthetic user", "system", false)

	setMFARequirement(t, db, "all")
	check("all covers everyone", "viewer-1", true)
	check("all never covers the synthetic user", "system", false)

	setMFARequirement(t, db, "garbage")
	check("an unrecognised value reads as off", "admin-1", false)

	setMFARequirement(t, db, "")
	check("a cleared value reads as off (break-glass)", "admin-1", false)
}

func TestMFAPolicyCachesUntilInvalidated(t *testing.T) {
	db := openMFAPolicyTestDB(t)
	policy := NewMFAPolicy(db, NewService(db))
	ctx := context.Background()

	setMFARequirement(t, db, "all")
	if got, err := policy.Requirement(ctx); err != nil || got != MFARequiredAll {
		t.Fatalf("requirement = %q, %v, want all", got, err)
	}
	if _, err := db.Exec(`UPDATE settings SET value = 'off' WHERE key = ?`, SettingMFARequired); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got, err := policy.Requirement(ctx); err != nil || got != MFARequiredAll {
		t.Fatalf("requirement = %q, %v, want the cached value until invalidated", got, err)
	}
	InvalidateMFAPolicy()
	if got, err := policy.Requirement(ctx); err != nil || got != MFARequiredOff {
		t.Fatalf("requirement = %q, %v, want off after invalidation", got, err)
	}
}
