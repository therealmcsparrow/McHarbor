// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestStackRoutesAreScopedToTheStacksEnvironment is H3 of the 2026-09-30 review: stacks are
// looked up by name while the gates read the caller's ?env=, so stacks.manage on envBBB
// revealed and rewrote a stack that lives in envAAA just by naming envBBB.
func TestStackRoutesAreScopedToTheStacksEnvironment(t *testing.T) {
	svc := newListTestService(t)
	seedStack(t, svc, "s1", "web", false)
	if err := svc.saveEnvVars(context.Background(), "s1", map[string]string{"DB_PASSWORD": "hunter2hunter2"}); err != nil {
		t.Fatal(err)
	}
	if err := rbac.UpsertCatalog(svc.db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO environments (id, name) VALUES ('envBBB', 'Other Env')`,
		`INSERT INTO users (id, username, password_hash) VALUES ('opsB', 'opsB', 'x'), ('opsA', 'opsA', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-ops', 'r-ops', '["stacks.view","stacks.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'opsB', 'r-ops', 'envBBB'), ('ur2', 'opsA', 'r-ops', 'envAAA')`,
	} {
		if _, err := svc.db.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	rbacSvc := rbac.NewService(svc.db)
	h := &Handler{service: svc, app: &router.AppDeps{
		Config:      &config.Config{},
		RBACService: rbacSvc,
		AuditLog:    audit.NewLogger(svc.db, ""),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
	r := chi.NewRouter()
	r.With(rbac.RequirePermission(rbacSvc, rbac.PermStacksManage)).Post("/stacks", h.HandleCreate)
	r.Route("/stacks/{name}", func(r chi.Router) {
		r.Use(rbac.PinEnvToResource(h.stackEnvironment))
		r.With(rbac.RequireStackPermission(rbacSvc, rbac.PermStacksView, "name")).Get("/env-vars", h.HandleGetEnvVars)
		r.With(rbac.RequireStackPermission(rbacSvc, rbac.PermStacksManage, "name")).Put("/env-vars", h.HandleUpdateEnvVars)
		r.With(rbac.RequireStackPermission(rbacSvc, rbac.PermStacksView, "name")).Get("/compose", h.HandleGetCompose)
		r.With(rbac.RequireStackPermission(rbacSvc, rbac.PermStacksManage, "name")).Put("/", h.HandleUpdate)
	})
	do := func(userID, method, target, body string) int {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	for _, c := range []struct{ method, target, body string }{
		{http.MethodGet, "/stacks/web/env-vars?env=envBBB&reveal=true", ""},
		{http.MethodGet, "/stacks/web/env-vars?env=envBBB", ""},
		{http.MethodPut, "/stacks/web/env-vars?env=envBBB", `{"DB_PASSWORD":"stolen"}`},
		{http.MethodGet, "/stacks/web/compose?env=envBBB&reveal=true", ""},
		{http.MethodPut, "/stacks/web?env=envBBB", `{"compose":"services: {}"}`},
	} {
		if code := do("opsB", c.method, c.target, c.body); code != http.StatusForbidden {
			t.Errorf("%s %s by a stacks.manage holder on another environment: HTTP %d, want 403", c.method, c.target, code)
		}
	}
	// A create is gated on ?env=, so its body may not name another environment.
	if code := do("opsB", http.MethodPost, "/stacks?env=envBBB", `{"name":"evil","compose":"services: {}","environmentId":"envAAA"}`); code != http.StatusForbidden {
		t.Errorf("create into another environment through the body: HTTP %d, want 403", code)
	}
	// The stack's own environment decides, whatever ?env= the caller names.
	if code := do("opsA", http.MethodGet, "/stacks/web/env-vars?env=envBBB&reveal=true", ""); code != http.StatusOK {
		t.Errorf("reveal by a stacks.manage holder on the stack's environment: HTTP %d, want 200", code)
	}
}
