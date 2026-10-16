// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestMoveRefusesATargetTheCallerHoldsNoGrantOn guards the move routes: the
// gate checks the source only, so the target in the body must be checked by the
// handler. The handler has no service, so reaching it would panic the test.
func TestMoveRefusesATargetTheCallerHoldsNoGrantOn(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "move.db")+"?_pragma=foreign_keys(0)")
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
		`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('ops', 'ops', '["containers.manage","containers.view"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'u1', 'ops', 'envA')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	h := &Handler{app: &router.AppDeps{
		RBACService: rbac.NewService(database),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}

	r := chi.NewRouter()
	r.Post("/containers/{id}/move", h.HandleMove)
	r.Post("/containers/{id}/move/stream", h.HandleMoveStream)
	r.Post("/containers/{id}/move/plan", h.HandleMovePlan)

	for _, path := range []string{"/containers/c1/move", "/containers/c1/move/stream", "/containers/c1/move/plan"} {
		req := httptest.NewRequest(http.MethodPost, path+"?env=envA", strings.NewReader(`{"targetEnvId":"envB"}`))
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "op"}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s to envB with a grant on envA only: got %d, want 403", path, rec.Code)
		}
	}
}
