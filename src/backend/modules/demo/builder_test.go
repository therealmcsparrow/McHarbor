// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

func sourceDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "real.db")+"?_foreign_keys=OFF")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Log(err)
		}
	})
	if err := db.Migrate(conn, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	stmts := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO users (id, username, email, password_hash, display_name, role, created_at, updated_at) VALUES ('u1', 'carlo', 'carlo@example.com', '$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA', 'Carlo', 'admin', ?, ?)`, []any{now.Format(time.RFC3339), now.Format(time.RFC3339)}},
		{`INSERT INTO sessions (id, user_id, provider, expires_at, created_at, updated_at) VALUES ('live', 'u1', 'local', ?, ?, ?)`, []any{now.Add(time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)}},
		{`INSERT INTO sessions (id, user_id, provider, expires_at, created_at, updated_at) VALUES ('expired', 'u1', 'local', ?, ?, ?)`, []any{now.Add(-time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)}},
		{`INSERT INTO appstore_plugins (id, slug, name, version, source, enabled, installed_at, updated_at) VALUES ('p1', 'addon-probe', 'Probe', '2026.09.01', 'catalog', 1, ?, ?)`, []any{now.Format(time.RFC3339), now.Format(time.RFC3339)}},
		{`INSERT INTO environments (id, name, connection_type, is_default, is_active, created_at, updated_at) VALUES ('real-env', 'Real host', 'socket', 1, 1, ?, ?)`, []any{now.Format(time.RFC3339), now.Format(time.RFC3339)}},
	}
	for _, s := range stmts {
		if _, err := conn.Exec(s.q, s.args...); err != nil {
			t.Fatalf("%s: %v", s.q, err)
		}
	}
	return conn
}

func TestBuildDatabaseCopiesAccountsAndPluginsButNothingReal(t *testing.T) {
	dir := t.TempDir()
	src := sourceDB(t, dir)
	migDir := filepath.Join(dir, "plugins", "addon-probe@2026.09.01", "migrations")
	if err := os.MkdirAll(migDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(migDir, "0001_init.sql"), []byte("CREATE TABLE plugin_addon_probe_items (id TEXT PRIMARY KEY);\nCREATE INDEX idx_probe ON plugin_addon_probe_items(id);"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewService(src, dir, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	path := filepath.Join(dir, "demo.db")
	if err := s.buildDatabase(context.Background(), src, path); err != nil {
		t.Fatal(err)
	}
	out, err := openDemoDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := out.Close(); err != nil {
			t.Log(err)
		}
	}()
	count := func(q string) int {
		var n int
		if err := out.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	if count("SELECT COUNT(*) FROM users WHERE id = 'u1' AND password_hash <> ''") != 1 {
		t.Fatal("the signed-in user was not copied, so demo mode would open on the setup wizard")
	}
	if count("SELECT COUNT(*) FROM sessions WHERE id = 'live'") != 1 {
		t.Fatal("the live session was not copied; the user would be signed out by entering demo mode")
	}
	if count("SELECT COUNT(*) FROM sessions WHERE id = 'expired'") != 0 {
		t.Fatal("an expired session was carried into the demo")
	}
	if count("SELECT COUNT(*) FROM environments") != 0 {
		t.Fatal("a real environment leaked into demo.db")
	}
	if count("SELECT COUNT(*) FROM plugin_addon_probe_items") != 0 {
		t.Fatal("plugin table missing")
	}
	if count("SELECT COUNT(*) FROM plugin_migrations WHERE origin = 'addon-probe' AND filename = '0001_init.sql'") != 1 {
		t.Fatal("plugin migration not recorded, so a later repair would run it twice")
	}
}
