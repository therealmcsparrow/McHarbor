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
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// TestElevatableRule guards what the request form may offer: plugin operational
// keys by default, access administration never by default, an admin's choice over
// both, and the stored request carrying the permission's readable label.
func TestElevatableRule(t *testing.T) {
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
	if err := rbac.UpsertCatalog(database); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('req', 'req', 'x')`,
		`INSERT INTO plugin_permissions (origin, permission_key, description, scope) VALUES ('addon-kvm', 'vm.manage', 'Manage virtual machines', 'environment')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	svc := NewService(database)
	ctx := context.Background()

	row, err := svc.Create(ctx, "req", CreateRequest{PermissionKey: "vm.manage", Justification: "migrate", RequestedMinutes: 30})
	if err != nil {
		t.Fatalf("a plugin operational key must be requestable by default: %v", err)
	}
	if got, err := svc.ByID(ctx, row.ID); err != nil || got.PermissionLabel != "Manage virtual machines" {
		t.Fatalf("label = %q", got.PermissionLabel)
	}
	if _, err := svc.Create(ctx, "req", CreateRequest{PermissionKey: "users.manage", Justification: "x"}); !errors.Is(err, ErrNotElevatable) {
		t.Fatalf("access administration must not be requestable by default: %v", err)
	}
	if _, err := svc.Create(ctx, "req", CreateRequest{PermissionKey: "nope.manage", Justification: "x"}); !errors.Is(err, ErrUnknownPermission) {
		t.Fatalf("unknown key: %v", err)
	}

	if err := svc.SetElevatable(ctx, "vm.manage", false, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "req", CreateRequest{PermissionKey: "vm.manage", Justification: "x"}); !errors.Is(err, ErrNotElevatable) {
		t.Fatalf("an admin's off switch must win over the default: %v", err)
	}
	if err := svc.SetElevatable(ctx, "users.manage", true, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "req", CreateRequest{PermissionKey: "users.manage", Justification: "x"}); err != nil {
		t.Fatalf("an admin's on switch must win over the exclusion: %v", err)
	}
	if err := svc.SetElevatable(ctx, "nope.manage", true, "admin"); !errors.Is(err, ErrUnknownPermission) {
		t.Fatalf("override for unknown key: %v", err)
	}
}
