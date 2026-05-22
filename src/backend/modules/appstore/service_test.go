// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestListIncludesAllInstallationsWithoutDuplicatingCatalogRows(t *testing.T) {
	db := openAppStoreTestDB(t)
	createAppStoreTestTables(t, db)
	seedAppStoreCatalog(t, db, "app-1", "whoami", "Whoami")
	seedAppStoreCatalog(t, db, "app-2", "uptime-kuma", "Uptime Kuma")
	seedAppInstallation(t, db, "install-1", "whoami", "stack-1", "whoami-prod", "env-1", "2026-03-17T08:00:00Z")
	seedAppInstallation(t, db, "install-2", "whoami", "stack-2", "whoami-lab", "env-2", "2026-03-17T09:00:00Z")
	seedEnvironment(t, db, "env-1", "Production")
	seedEnvironment(t, db, "env-2", "Homelab")

	svc := newAppStoreTestService(db)

	apps, total, err := svc.List("", "", 1, 10)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(apps) != 2 {
		t.Fatalf("expected 2 catalog rows, got %d", len(apps))
	}

	var whoami *AppTemplate
	for i := range apps {
		if apps[i].Slug == "whoami" {
			whoami = &apps[i]
			break
		}
	}
	if whoami == nil {
		t.Fatal("expected whoami app in catalog results")
	}
	if !whoami.Installed {
		t.Fatal("expected whoami to be marked installed")
	}
	if len(whoami.Installations) != 2 {
		t.Fatalf("expected 2 installations for whoami, got %d", len(whoami.Installations))
	}
	if whoami.Installations[0].EnvironmentName != "Homelab" {
		t.Fatalf("expected newest installation environment Homelab, got %q", whoami.Installations[0].EnvironmentName)
	}
	if whoami.Installations[1].EnvironmentName != "Production" {
		t.Fatalf("expected older installation environment Production, got %q", whoami.Installations[1].EnvironmentName)
	}
}

func TestBySlugIncludesInstallationLocations(t *testing.T) {
	db := openAppStoreTestDB(t)
	createAppStoreTestTables(t, db)
	seedAppStoreCatalog(t, db, "app-1", "whoami", "Whoami")
	seedEnvironment(t, db, "env-1", "Production")
	seedAppInstallation(t, db, "install-1", "whoami", "stack-1", "whoami-prod", "env-1", "2026-03-17T08:00:00Z")

	svc := newAppStoreTestService(db)

	app, err := svc.BySlug("whoami")
	if err != nil {
		t.Fatalf("BySlug returned error: %v", err)
	}
	if app == nil {
		t.Fatal("expected whoami app, got nil")
	}
	if !app.Installed {
		t.Fatal("expected app to be marked installed")
	}
	if len(app.Installations) != 1 {
		t.Fatalf("expected 1 installation, got %d", len(app.Installations))
	}
	if app.Installations[0].EnvironmentName != "Production" {
		t.Fatalf("expected environment name Production, got %q", app.Installations[0].EnvironmentName)
	}
	if app.StackID != "stack-1" {
		t.Fatalf("expected StackID stack-1, got %q", app.StackID)
	}
}

func newAppStoreTestService(db *sql.DB) *Service {
	return NewService(db, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func openAppStoreTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "appstore-test.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}

func createAppStoreTestTables(t *testing.T, db *sql.DB) {
	t.Helper()

	if _, err := db.Exec(`
CREATE TABLE appstore_catalog (
	id TEXT PRIMARY KEY,
	slug TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	category TEXT NOT NULL DEFAULT '',
	image TEXT NOT NULL DEFAULT '',
	logo TEXT NOT NULL DEFAULT '',
	website TEXT NOT NULL DEFAULT '',
	docs_url TEXT NOT NULL DEFAULT '',
	ports TEXT NOT NULL DEFAULT '[]',
	volumes TEXT NOT NULL DEFAULT '[]',
	env_vars TEXT NOT NULL DEFAULT '[]',
	compose_override TEXT NOT NULL DEFAULT '',
	min_memory TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT 'bundled',
	version TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE appstore_installed (
	id TEXT PRIMARY KEY,
	catalog_slug TEXT NOT NULL,
	stack_id TEXT NOT NULL,
	stack_name TEXT NOT NULL DEFAULT '',
	environment_id TEXT,
	installed_at TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE environments (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL
);

CREATE TABLE stacks (
	id TEXT PRIMARY KEY,
	status TEXT NOT NULL DEFAULT 'running'
);
`); err != nil {
		t.Fatalf("creating app store test tables: %v", err)
	}
}

func seedAppStoreCatalog(t *testing.T, db *sql.DB, id, slug, name string) {
	t.Helper()

	if _, err := db.Exec(`
		INSERT INTO appstore_catalog (
			id, slug, name, description, category, image, logo, website, docs_url,
			ports, volumes, env_vars, compose_override, min_memory, source, version, created_at, updated_at
		) VALUES (?, ?, ?, '', 'Utility', 'containous/whoami', '', '', '', '[]', '[]', '[]', '', '', 'bundled', '1.0.0', '2026-03-17T08:00:00Z', '2026-03-17T08:00:00Z')
	`, id, slug, name); err != nil {
		t.Fatalf("seeding appstore_catalog: %v", err)
	}
}

func seedAppInstallation(t *testing.T, db *sql.DB, id, slug, stackID, stackName, environmentID, installedAt string) {
	t.Helper()

	if _, err := db.Exec(`
		INSERT INTO appstore_installed (
			id, catalog_slug, stack_id, stack_name, environment_id, installed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, slug, stackID, stackName, environmentID, installedAt, installedAt, installedAt); err != nil {
		t.Fatalf("seeding appstore_installed: %v", err)
	}
}

func seedEnvironment(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()

	if _, err := db.Exec(`INSERT INTO environments (id, name) VALUES (?, ?)`, id, name); err != nil {
		t.Fatalf("seeding environments: %v", err)
	}
}
