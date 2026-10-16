// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package elevations

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestGetShowsARequestOnlyToItsRequesterAndDeciders guards the read that used
// to return any elevation request, reason and permission included, to anyone.
func TestGetShowsARequestOnlyToItsRequesterAndDeciders(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "elev.db")+"?_pragma=foreign_keys(0)")
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
		`INSERT INTO users (id, username, password_hash) VALUES ('asker', 'asker', 'x'), ('stranger', 'stranger', 'x'), ('admin', 'admin', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('um', 'um', '["users.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur1', 'admin', 'um')`,
		`INSERT INTO elevation_requests (id, requester_id, permission_key, status) VALUES ('el-1', 'asker', 'volumes.delete', 'pending')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	h := &Handler{svc: NewService(database), app: &router.AppDeps{
		RBACService: rbac.NewService(database),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
	r := chi.NewRouter()
	r.Get("/elevations/{id}", h.HandleGet)

	for user, want := range map[string]int{"asker": http.StatusOK, "admin": http.StatusOK, "stranger": http.StatusNotFound} {
		req := httptest.NewRequest(http.MethodGet, "/elevations/el-1", nil)
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: user, Username: user}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s: got %d, want %d", user, rec.Code, want)
		}
	}
}
