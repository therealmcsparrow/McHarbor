// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/firstrun"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

func TestPathWithinKeepsSQLiteInsideTheDataDir(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]bool{
		filepath.Join(dir, "mcharbor.db"):             true,
		filepath.Join(dir, "sub", "x.db"):             true,
		filepath.Join(dir, "..", "escape.db"):         false,
		filepath.Join(dir, "sub", "..", "..", "e.db"): false,
		filepath.Join(t.TempDir(), "other.db"):        false,
	}
	for path, want := range cases {
		if got := pathWithin(dir, path); got != want {
			t.Errorf("pathWithin(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestSetupTestNeedsTheBootToken guards the unauthenticated installer route: it
// dials arbitrary hosts and creates files, so it answers only to the token.
func TestSetupTestNeedsTheBootToken(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "fr.db")+"?_pragma=foreign_keys(0)")
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
	authSvc := auth.NewService(database)
	gate := firstrun.NewWithToken("boot-token")
	dataDir := t.TempDir()
	h := &Handler{app: &router.AppDeps{
		AuthService: authSvc,
		FirstRun:    gate,
		Config:      &config.Config{DataDir: dataDir},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}

	call := func(token, body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/database/setup/test", strings.NewReader(body))
		if token != "" {
			req.Header.Set(firstrun.Header, token)
		}
		rec := httptest.NewRecorder()
		h.HandleSetupTest(rec, req)
		return rec.Code
	}
	if got := call("", `{"driver":"sqlite"}`); got != http.StatusForbidden {
		t.Fatalf("no token: got %d, want 403", got)
	}
	if got := call("wrong", `{"driver":"sqlite"}`); got != http.StatusForbidden {
		t.Fatalf("wrong token: got %d, want 403", got)
	}
	outside := strings.ReplaceAll(filepath.Join(t.TempDir(), "evil.db"), `\`, `\\`)
	if got := call("boot-token", `{"driver":"sqlite","path":"`+outside+`"}`); got != http.StatusBadRequest {
		t.Fatalf("a SQLite path outside the data dir: got %d, want 400", got)
	}
}
