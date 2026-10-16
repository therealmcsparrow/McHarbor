// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

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
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestViewersDoNotSeeWhereAnEnvironmentConnects guards the rule that Docker
// socket paths and connection targets never reach a caller who cannot change
// them; managers still need them to edit the environment.
func TestViewersDoNotSeeWhereAnEnvironmentConnects(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "env.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('viewer', 'viewer', 'x'), ('manager', 'manager', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-view', 'r-view', '["environments.view"]'), ('r-manage', 'r-manage', '["environments.view","environments.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur1', 'viewer', 'r-view'), ('ur2', 'manager', 'r-manage')`,
		`INSERT INTO environments (id, name, connection_type, socket_path, host, port, ssh_host, ssh_user) VALUES ('e1', 'prod', 'socket', '/var/run/docker.sock', '10.0.0.5', 2376, 'bastion.internal', 'deploy')`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(&router.AppDeps{DB: database, RBACService: rbac.NewService(database), Logger: logger})
	r := chi.NewRouter()
	r.Get("/environments", h.HandleList)
	r.Get("/environments/{id}", h.HandleGet)

	call := func(userID, path string) string {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s: HTTP %d %s", userID, path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	secrets := []string{"docker.sock", "10.0.0.5", "2376", "bastion.internal", "deploy"}

	for _, path := range []string{"/environments", "/environments/e1"} {
		body := call("viewer", path)
		for _, s := range secrets {
			if strings.Contains(body, s) {
				t.Errorf("viewer %s: response carries %q: %s", path, s, body)
			}
		}
		if !strings.Contains(body, `"name":"prod"`) {
			t.Errorf("viewer %s: the environment itself is missing: %s", path, body)
		}
		body = call("manager", path)
		for _, s := range secrets {
			if !strings.Contains(body, s) {
				t.Errorf("manager %s: response lost %q, which the edit form needs: %s", path, s, body)
			}
		}
	}
}
