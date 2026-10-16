// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package accessreview

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "access.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO roles (id, name, permissions) VALUES ('r-star', 'Zeta Root', '["*"]'), ('r-img', '=HYPERLINK("x")', '["images.view"]')`,
		`INSERT INTO users (id, username, password_hash) VALUES ('u-1', 'reviewer', 'x')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur-1', 'u-1', 'r-img')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return NewService(database, rbac.NewService(database), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestExportListsHoldersAndFlagsWildcards(t *testing.T) {
	svc := newTestService(t)
	export, err := svc.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	var images *CatalogExportEntry
	for i := range export.Permissions {
		if export.Permissions[i].Key == string(rbac.PermImagesView) {
			images = &export.Permissions[i]
		}
	}
	if images == nil || images.Origin != "host" || images.Scope == "" {
		t.Fatalf("images.view missing or malformed: %+v", images)
	}
	var direct, wildcard bool
	for _, holder := range images.Roles {
		if holder.ID == "r-img" && !holder.Wildcard {
			direct = true
		}
		if holder.ID == "r-star" && holder.Wildcard {
			wildcard = true
		}
	}
	if !direct || !wildcard {
		t.Fatalf("images.view holders must list the direct role and the flagged wildcard role: %+v", images.Roles)
	}
	if len(export.Permissions) != len(rbac.CatalogSeed()) {
		t.Fatalf("every host key is exported: %d vs %d", len(export.Permissions), len(rbac.CatalogSeed()))
	}

	body, err := ExportCSV(export)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.HasPrefix(text, "key,category,action,description,scope,origin,elevatable,roles\n") {
		t.Fatalf("csv header: %q", text[:80])
	}
	if strings.Contains(text, `"=HYPERLINK`) || strings.Contains(text, "\n=HYPERLINK") || !strings.Contains(text, "Zeta Root (*)") {
		t.Fatalf("role names must be formula-guarded and wildcard holders marked:\n%s", text)
	}
}

func TestEffectiveResolvesThroughRBAC(t *testing.T) {
	svc := newTestService(t)
	out, err := svc.Effective(context.Background(), "u-1", "", "")
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}
	if out.Username != "reviewer" || out.RoleNames["r-img"] == "" {
		t.Fatalf("names not resolved: %+v", out)
	}
	effective := 0
	for _, p := range out.Permissions {
		if p.Effective {
			effective++
			if p.Key != rbac.PermImagesView {
				t.Fatalf("only images.view is held, got %s", p.Key)
			}
		}
	}
	if effective != 1 {
		t.Fatalf("expected exactly one effective key, got %d", effective)
	}
	if _, err := svc.Effective(context.Background(), "nobody", "", ""); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
	if _, err := svc.Effective(context.Background(), "u-1", "env-missing", ""); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("unknown environment: %v", err)
	}
}
