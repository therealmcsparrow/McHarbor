// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package elevations

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// TestRequesterCannotApproveOwnElevation guards separation of duties: a requester
// may withdraw, but only someone else may approve.
func TestRequesterCannotApproveOwnElevation(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "e.db")+"?_pragma=foreign_keys(1)")
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
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('req', 'req', 'x'), ('boss', 'boss', 'x')`,
		`INSERT OR REPLACE INTO permission_catalog (key, category, action, description, scope, is_elevatable) VALUES ('containers.delete', 'containers', 'delete', 'd', 'environment', 1)`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	svc := NewService(database)
	ctx := context.Background()
	req, err := svc.Create(ctx, "req", CreateRequest{PermissionKey: "containers.delete", Justification: "incident", RequestedMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Decide(ctx, req.ID, "req", true, ""); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("self-approval: %v", err)
	}
	if got, err := svc.Decide(ctx, req.ID, "boss", true, "ok"); err != nil || got.Status != StatusApproved {
		t.Fatalf("approval by another user: %+v %v", got, err)
	}

	second, err := svc.Create(ctx, "req", CreateRequest{PermissionKey: "containers.delete", Justification: "again", RequestedMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.Decide(ctx, second.ID, "req", false, "withdrawn"); err != nil || got.Status != StatusDenied {
		t.Fatalf("withdrawal must stay possible: %+v %v", got, err)
	}
}

// TestElevationForEnvScopedPluginKey guards the manifest-to-catalog scope
// translation: a plugin declares "env", permission_catalog only accepts
// "environment", and mirroring the raw value used to fail the whole request.
func TestElevationForEnvScopedPluginKey(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "e.db")+"?_pragma=foreign_keys(1)")
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
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('req', 'req', 'x')`,
		`INSERT INTO plugin_permissions (origin, permission_key, description, scope) VALUES ('addon-kvm', 'vm.delete', 'Delete VMs', 'env')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	svc := NewService(database)
	ctx := context.Background()
	if err := svc.SetElevatable(ctx, "vm.delete", true, "req"); err != nil {
		t.Fatalf("SetElevatable: %v", err)
	}
	if _, err := svc.Create(ctx, "req", CreateRequest{PermissionKey: "vm.delete", Justification: "incident", RequestedMinutes: 30}); err != nil {
		t.Fatalf("Create for an env-scoped plugin key: %v", err)
	}
	var scope string
	if err := database.QueryRow(`SELECT scope FROM permission_catalog WHERE key = 'vm.delete'`).Scan(&scope); err != nil || scope != "environment" {
		t.Fatalf("mirrored scope = %q, %v; want environment", scope, err)
	}
}
