// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package roles

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	coreaudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	_ "modernc.org/sqlite"
)

func TestServiceCRUD(t *testing.T) {
	database := openRolesTestDB(t)
	createRolesTestTables(t, database)
	seedRole(t, database, "role_admin", "Admin", "", `["*"]`, true)

	service := NewService(database)
	role, err := service.Create(t.Context(), &CreateRoleInput{Name: "Operator", Description: "Can operate", Permissions: []string{"containers.view"}})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if role.ID == "" || role.Name != "Operator" || len(role.Permissions) != 1 {
		t.Fatalf("unexpected created role: %#v", role)
	}

	items, err := service.List(t.Context())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two roles, got %#v", items)
	}
	emptyRoles, err := NewService(openRolesTestDBWithSchema(t)).List(t.Context())
	if err != nil {
		t.Fatalf("empty List returned error: %v", err)
	}
	if len(emptyRoles) != 0 {
		t.Fatalf("expected empty role list, got %#v", emptyRoles)
	}

	got, err := service.Get(t.Context(), role.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got == nil || got.Name != "Operator" {
		t.Fatalf("unexpected role: %#v", got)
	}

	name := "Maintainer"
	description := "Maintains systems"
	updated, err := service.Update(t.Context(), role.ID, &UpdateRoleInput{
		Name:        &name,
		Description: &description,
		Permissions: []string{"settings.view"},
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.Name != name || updated.Description != description || updated.Permissions[0] != "settings.view" {
		t.Fatalf("unexpected updated role: %#v", updated)
	}

	emptyUpdate, err := service.Update(t.Context(), "missing", &UpdateRoleInput{})
	if err != nil {
		t.Fatalf("empty missing update returned error: %v", err)
	}
	if emptyUpdate != nil {
		t.Fatalf("expected nil missing update, got %#v", emptyUpdate)
	}

	if exists, err := service.NameExists(t.Context(), "Maintainer", ""); err != nil || !exists {
		t.Fatalf("expected name to exist, exists=%v err=%v", exists, err)
	}
	if exists, err := service.NameExists(t.Context(), "Maintainer", role.ID); err != nil || exists {
		t.Fatalf("expected excluded name to not exist, exists=%v err=%v", exists, err)
	}
	if isSystem, err := service.IsSystem(t.Context(), "role_admin"); err != nil || !isSystem {
		t.Fatalf("expected system role, isSystem=%v err=%v", isSystem, err)
	}
	if isSystem, err := service.IsSystem(t.Context(), "missing"); err != nil || isSystem {
		t.Fatalf("expected missing role to not be system, isSystem=%v err=%v", isSystem, err)
	}

	if err := service.Delete(t.Context(), role.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	missing, err := service.Get(t.Context(), role.ID)
	if err != nil {
		t.Fatalf("Get deleted returned error: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected deleted role to be missing, got %#v", missing)
	}
}

func TestServiceFallbacksAndErrors(t *testing.T) {
	database := openRolesTestDB(t)
	createRolesTestTables(t, database)
	seedRole(t, database, "role_bad", "Broken", "", `not-json`, false)

	items, err := NewService(database).List(t.Context())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 || len(items[0].Permissions) != 0 {
		t.Fatalf("expected invalid permissions to fall back to empty slice, got %#v", items)
	}
	got, err := NewService(database).Get(t.Context(), "role_bad")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got == nil || len(got.Permissions) != 0 {
		t.Fatalf("expected invalid get permissions to fall back, got %#v", got)
	}

	if _, err := scanRole(errorRoleScannable{}); err == nil {
		t.Fatal("expected scanRole error")
	}

	scanDB := openRolesTestDB(t)
	if _, err := scanDB.Exec(`
CREATE TABLE roles (
	id TEXT PRIMARY KEY,
	name TEXT,
	description TEXT,
	permissions TEXT,
	is_system INTEGER,
	created_at TEXT,
	updated_at TEXT
);
INSERT INTO roles (id, name, permissions, is_system, created_at, updated_at)
VALUES ('role-1', NULL, '[]', 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
`); err != nil {
		t.Fatalf("creating scan error db: %v", err)
	}
	if _, err := NewService(scanDB).List(t.Context()); err == nil {
		t.Fatal("expected list scan error")
	}

	closed := openRolesTestDB(t)
	createRolesTestTables(t, closed)
	if err := closed.Close(); err != nil {
		t.Fatalf("closing db: %v", err)
	}
	service := NewService(closed)
	if _, err := service.List(t.Context()); err == nil {
		t.Fatal("expected list query error")
	}
	if _, err := service.Get(t.Context(), "role-1"); err == nil {
		t.Fatal("expected get query error")
	}
	if _, err := service.Create(t.Context(), &CreateRoleInput{Name: "x"}); err == nil {
		t.Fatal("expected create insert error")
	}
	name := "x"
	description := "x"
	if _, err := service.Update(t.Context(), "role-1", &UpdateRoleInput{Name: &name}); err == nil {
		t.Fatal("expected update name error")
	}
	if _, err := service.Update(t.Context(), "role-1", &UpdateRoleInput{Description: &description}); err == nil {
		t.Fatal("expected update description error")
	}
	if _, err := service.Update(t.Context(), "role-1", &UpdateRoleInput{Permissions: []string{"x"}}); err == nil {
		t.Fatal("expected update permissions error")
	}
	if err := service.Delete(t.Context(), "role-1"); err == nil {
		t.Fatal("expected delete error")
	}
	if _, err := service.IsSystem(t.Context(), "role-1"); err == nil {
		t.Fatal("expected is system error")
	}
	if _, err := service.NameExists(t.Context(), "x", ""); err == nil {
		t.Fatal("expected name exists error")
	}
}

func TestHandlers(t *testing.T) {
	database := openRolesTestDB(t)
	createRolesTestTables(t, database)
	seedRole(t, database, "role_admin", "Admin", "", `["*"]`, true)
	seedRole(t, database, "role_viewer", "Viewer", "Can view", `["containers.view"]`, false)
	handler := NewHandler(rolesApp(database))

	assertRoleStatus(t, http.StatusOK, func(w *httptest.ResponseRecorder) {
		handler.HandleList(w, httptest.NewRequest(http.MethodGet, "/roles", nil))
	})
	assertRoleStatus(t, http.StatusOK, func(w *httptest.ResponseRecorder) {
		handler.HandleListPermissions(w, httptest.NewRequest(http.MethodGet, "/roles/permissions", nil))
	})
	assertRoleStatus(t, http.StatusOK, func(w *httptest.ResponseRecorder) {
		handler.HandleGet(w, rolesRouteRequest(http.MethodGet, "/roles/role_viewer", "", "id", "role_viewer"))
	})
	assertRoleStatus(t, http.StatusNotFound, func(w *httptest.ResponseRecorder) {
		handler.HandleGet(w, rolesRouteRequest(http.MethodGet, "/roles/missing", "", "id", "missing"))
	})

	for _, body := range []string{"{", `{}`} {
		assertRoleStatus(t, http.StatusBadRequest, func(w *httptest.ResponseRecorder) {
			handler.HandleCreate(w, httptest.NewRequest(http.MethodPost, "/roles", strings.NewReader(body)))
		})
	}
	assertRoleStatus(t, http.StatusConflict, func(w *httptest.ResponseRecorder) {
		handler.HandleCreate(w, httptest.NewRequest(http.MethodPost, "/roles", strings.NewReader(`{"name":"Viewer"}`)))
	})
	assertRoleStatus(t, http.StatusCreated, func(w *httptest.ResponseRecorder) {
		handler.HandleCreate(w, httptest.NewRequest(http.MethodPost, "/roles", strings.NewReader(`{"name":"Operator"}`)))
	})

	assertRoleStatus(t, http.StatusForbidden, func(w *httptest.ResponseRecorder) {
		handler.HandleUpdate(w, rolesRouteRequest(http.MethodPut, "/roles/role_admin", `{"name":"Root"}`, "id", "role_admin"))
	})
	assertRoleStatus(t, http.StatusBadRequest, func(w *httptest.ResponseRecorder) {
		handler.HandleUpdate(w, rolesRouteRequest(http.MethodPut, "/roles/role_viewer", "{", "id", "role_viewer"))
	})
	assertRoleStatus(t, http.StatusBadRequest, func(w *httptest.ResponseRecorder) {
		handler.HandleUpdate(w, rolesRouteRequest(http.MethodPut, "/roles/role_viewer", `{"name":""}`, "id", "role_viewer"))
	})
	assertRoleStatus(t, http.StatusConflict, func(w *httptest.ResponseRecorder) {
		handler.HandleUpdate(w, rolesRouteRequest(http.MethodPut, "/roles/role_viewer", `{"name":"Admin"}`, "id", "role_viewer"))
	})
	assertRoleStatus(t, http.StatusOK, func(w *httptest.ResponseRecorder) {
		handler.HandleUpdate(w, rolesRouteRequest(http.MethodPut, "/roles/role_viewer", `{"description":"Updated"}`, "id", "role_viewer"))
	})
	assertRoleStatus(t, http.StatusNotFound, func(w *httptest.ResponseRecorder) {
		handler.HandleUpdate(w, rolesRouteRequest(http.MethodPut, "/roles/missing", `{"description":"Updated"}`, "id", "missing"))
	})

	assertRoleStatus(t, http.StatusForbidden, func(w *httptest.ResponseRecorder) {
		handler.HandleDelete(w, rolesRouteRequest(http.MethodDelete, "/roles/role_admin", "", "id", "role_admin"))
	})
	assertRoleStatus(t, http.StatusNotFound, func(w *httptest.ResponseRecorder) {
		handler.HandleDelete(w, rolesRouteRequest(http.MethodDelete, "/roles/missing", "", "id", "missing"))
	})
	assertRoleStatus(t, http.StatusNoContent, func(w *httptest.ResponseRecorder) {
		handler.HandleDelete(w, rolesRouteRequest(http.MethodDelete, "/roles/role_viewer", "", "id", "role_viewer"))
	})
}

func TestHandlerErrors(t *testing.T) {
	closed := openRolesTestDB(t)
	createRolesTestTables(t, closed)
	if err := closed.Close(); err != nil {
		t.Fatalf("closing db: %v", err)
	}
	handler := NewHandler(rolesApp(closed))
	for _, call := range []func(*httptest.ResponseRecorder){
		func(w *httptest.ResponseRecorder) {
			handler.HandleList(w, httptest.NewRequest(http.MethodGet, "/roles", nil))
		},
		func(w *httptest.ResponseRecorder) {
			handler.HandleGet(w, rolesRouteRequest(http.MethodGet, "/roles/x", "", "id", "x"))
		},
		func(w *httptest.ResponseRecorder) {
			handler.HandleCreate(w, httptest.NewRequest(http.MethodPost, "/roles", strings.NewReader(`{"name":"x"}`)))
		},
		func(w *httptest.ResponseRecorder) {
			handler.HandleUpdate(w, rolesRouteRequest(http.MethodPut, "/roles/x", `{"name":"x"}`, "id", "x"))
		},
		func(w *httptest.ResponseRecorder) {
			handler.HandleDelete(w, rolesRouteRequest(http.MethodDelete, "/roles/x", "", "id", "x"))
		},
	} {
		assertRoleStatus(t, http.StatusInternalServerError, call)
	}

	createErrorDB := openRolesTestDB(t)
	createRolesTestTables(t, createErrorDB)
	if _, err := createErrorDB.Exec(`CREATE TRIGGER roles_insert_error BEFORE INSERT ON roles BEGIN SELECT RAISE(ABORT, 'insert blocked'); END;`); err != nil {
		t.Fatalf("creating insert trigger: %v", err)
	}
	assertRoleStatus(t, http.StatusInternalServerError, func(w *httptest.ResponseRecorder) {
		NewHandler(rolesApp(createErrorDB)).HandleCreate(w, httptest.NewRequest(http.MethodPost, "/roles", strings.NewReader(`{"name":"x"}`)))
	})

	updateErrorDB := openRolesTestDB(t)
	createRolesTestTables(t, updateErrorDB)
	seedRole(t, updateErrorDB, "role_viewer", "Viewer", "", `[]`, false)
	if _, err := updateErrorDB.Exec(`CREATE TRIGGER roles_update_error BEFORE UPDATE ON roles BEGIN SELECT RAISE(ABORT, 'update blocked'); END;`); err != nil {
		t.Fatalf("creating update trigger: %v", err)
	}
	assertRoleStatus(t, http.StatusInternalServerError, func(w *httptest.ResponseRecorder) {
		NewHandler(rolesApp(updateErrorDB)).HandleUpdate(w, rolesRouteRequest(http.MethodPut, "/roles/role_viewer", `{"name":"Operator"}`, "id", "role_viewer"))
	})

	deleteErrorDB := openRolesTestDB(t)
	createRolesTestTables(t, deleteErrorDB)
	seedRole(t, deleteErrorDB, "role_viewer", "Viewer", "", `[]`, false)
	if _, err := deleteErrorDB.Exec(`CREATE TRIGGER roles_delete_error BEFORE DELETE ON roles BEGIN SELECT RAISE(ABORT, 'delete blocked'); END;`); err != nil {
		t.Fatalf("creating delete trigger: %v", err)
	}
	assertRoleStatus(t, http.StatusInternalServerError, func(w *httptest.ResponseRecorder) {
		NewHandler(rolesApp(deleteErrorDB)).HandleDelete(w, rolesRouteRequest(http.MethodDelete, "/roles/role_viewer", "", "id", "role_viewer"))
	})

	nameCheckErrorDB := openRolesTestDB(t)
	if _, err := nameCheckErrorDB.Exec(`CREATE TABLE roles (id TEXT PRIMARY KEY, is_system INTEGER NOT NULL DEFAULT 0); INSERT INTO roles (id, is_system) VALUES ('role_viewer', 0);`); err != nil {
		t.Fatalf("creating name check error db: %v", err)
	}
	assertRoleStatus(t, http.StatusInternalServerError, func(w *httptest.ResponseRecorder) {
		NewHandler(rolesApp(nameCheckErrorDB)).HandleUpdate(w, rolesRouteRequest(http.MethodPut, "/roles/role_viewer", `{"name":"Operator"}`, "id", "role_viewer"))
	})
	assertRoleStatus(t, http.StatusInternalServerError, func(w *httptest.ResponseRecorder) {
		NewHandler(rolesApp(nameCheckErrorDB)).HandleDelete(w, rolesRouteRequest(http.MethodDelete, "/roles/role_viewer", "", "id", "role_viewer"))
	})
}

func TestMountRegistersRoutes(t *testing.T) {
	database := openRolesTestDB(t)
	createRolesTestTables(t, database)
	seedRole(t, database, "role_admin", "Admin", "", `["*"]`, true)
	app := rolesApp(database)
	Mount(app)

	r := chi.NewRouter()
	app.MountProtectedRoutes(r)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, rolesRequestWithSystem(http.MethodGet, "/roles/permissions", ""))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected mounted permissions route, got %d", resp.Code)
	}
}

type errorRoleScannable struct{}

func (errorRoleScannable) Scan(...any) error {
	return errors.New("scan failed")
}

func openRolesTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "roles-test.db")
	database, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	return database
}

func openRolesTestDBWithSchema(t *testing.T) *sql.DB {
	t.Helper()

	database := openRolesTestDB(t)
	createRolesTestTables(t, database)
	return database
}

func createRolesTestTables(t *testing.T, database *sql.DB) {
	t.Helper()

	if _, err := database.Exec(`
CREATE TABLE roles (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	permissions TEXT NOT NULL,
	is_system INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE audit_logs (
	id TEXT PRIMARY KEY,
	user_id TEXT,
	username TEXT,
	action TEXT NOT NULL,
	entity_type TEXT,
	entity_id TEXT,
	entity_name TEXT,
	details TEXT,
	ip_address TEXT,
	environment_id TEXT,
	timestamp TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE in_app_notifications (
	id TEXT PRIMARY KEY,
	level TEXT NOT NULL,
	title TEXT NOT NULL,
	message TEXT NOT NULL,
	action TEXT,
	entity_type TEXT,
	entity_id TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE user_roles (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, role_id TEXT NOT NULL);
CREATE TABLE group_roles (id TEXT PRIMARY KEY, group_id TEXT NOT NULL, role_id TEXT NOT NULL);
CREATE TABLE api_key_roles (api_key_id TEXT NOT NULL, role_id TEXT NOT NULL);
`); err != nil {
		t.Fatalf("creating roles schema: %v", err)
	}
}

func seedRole(t *testing.T, database *sql.DB, id, name, description, permissions string, system bool) {
	t.Helper()

	if _, err := database.Exec(
		`INSERT INTO roles (id, name, description, permissions, is_system, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		id, name, description, permissions, boolRoleInt(system),
	); err != nil {
		t.Fatalf("seeding role: %v", err)
	}
}

func rolesApp(database *sql.DB) *router.AppDeps {
	return &router.AppDeps{
		DB:          database,
		RBACService: rbac.NewService(database),
		AuditLog:    coreaudit.NewLogger(database, "test-audit-secret"),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func rolesRouteRequest(method, target, body, paramKey, paramValue string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add(paramKey, paramValue)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func rolesRequestWithSystem(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "system", Username: "admin"}))
}

func assertRoleStatus(t *testing.T, expected int, call func(*httptest.ResponseRecorder)) {
	t.Helper()

	resp := httptest.NewRecorder()
	call(resp)
	if resp.Code != expected {
		t.Fatalf("expected status %d, got %d body=%s", expected, resp.Code, resp.Body.String())
	}
}

func boolRoleInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
