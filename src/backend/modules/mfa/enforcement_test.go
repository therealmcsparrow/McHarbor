// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package mfa

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

type fakeEnrollments map[string]bool

func (f fakeEnrollments) IsEnabled(_ context.Context, userID string) (bool, error) {
	return f[userID], nil
}

func newPolicyDB(t *testing.T, requirement string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "mfa-policy.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing db: %v", err)
		}
	})
	if _, err := db.Exec(`CREATE TABLE settings (id TEXT PRIMARY KEY, key TEXT NOT NULL UNIQUE, value TEXT, category TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO settings (id, key, value, category) VALUES ('s1', ?, ?, 'security')`, rbac.SettingMFARequired, requirement); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rbac.InvalidateMFAPolicy()
	return db
}

func TestEnforcerEnrollmentOwed(t *testing.T) {
	ctx := context.Background()
	enrollments := fakeEnrollments{"enrolled": true}

	all := &Enforcer{enrollments: enrollments, policy: rbac.NewMFAPolicy(newPolicyDB(t, "all"), nil)}
	owed, err := all.EnrollmentOwed(ctx, &auth.User{ID: "fresh"})
	if err != nil || !owed {
		t.Fatalf("all + not enrolled: owed=%v err=%v, want owed", owed, err)
	}
	owed, err = all.EnrollmentOwed(ctx, &auth.User{ID: "enrolled"})
	if err != nil || owed {
		t.Fatalf("all + enrolled: owed=%v err=%v, want not owed", owed, err)
	}
	keyCtx := rbac.WithAPIKey(ctx, &rbac.APIKeyPrincipal{KeyID: "k1"})
	if owed, err := all.EnrollmentOwed(keyCtx, &auth.User{ID: "fresh"}); err != nil || owed {
		t.Fatal("an API-key request must never be held at enrolment")
	}
	if required, err := all.Required(ctx, &auth.User{ID: "enrolled"}); err != nil || !required {
		t.Fatal("Required must report coverage regardless of enrolment, so disable is refused")
	}

	off := &Enforcer{enrollments: enrollments, policy: rbac.NewMFAPolicy(newPolicyDB(t, "off"), nil)}
	if owed, err := off.EnrollmentOwed(ctx, &auth.User{ID: "fresh"}); err != nil || owed {
		t.Fatal("off must never hold a session")
	}
	if required, err := off.Required(ctx, &auth.User{ID: "fresh"}); err != nil || required {
		t.Fatal("off must not refuse disable")
	}
}
