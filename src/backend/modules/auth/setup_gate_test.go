// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	coreauth "github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/firstrun"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestFirstAdminNeedsTheBootToken guards /auth/setup: before any account exists
// it is open to the network, so creating the first admin takes the token that
// only the container log shows.
func TestFirstAdminNeedsTheBootToken(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "setup.db")+"?_pragma=foreign_keys(0)")
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
	h := &Handler{app: &router.AppDeps{
		DB:          database,
		AuthService: coreauth.NewService(database),
		FirstRun:    firstrun.NewWithToken("boot-token"),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
	call := func(token string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(`{}`))
		if token != "" {
			req.Header.Set(firstrun.Header, token)
		}
		rec := httptest.NewRecorder()
		h.HandleSetup(rec, req)
		return rec.Code
	}
	if got := call(""); got != http.StatusForbidden {
		t.Fatalf("no token: got %d, want 403", got)
	}
	if got := call("boot-token"); got == http.StatusForbidden {
		t.Fatal("the boot token must get past the gate (an empty body then fails validation, not the gate)")
	}
}
