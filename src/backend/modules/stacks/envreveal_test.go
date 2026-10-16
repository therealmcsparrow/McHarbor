// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

const stackPinnedSecret = "pinned-stack-test-secret-91ab"

// TestStackEnvRevealNeedsManageAndNeverShowsTheRootOfTrust guards the stack half of F-1:
// stacks.view used to reveal every credential in a stack, and a stack that copied
// McHarbor's secret under another name handed it to anyone who asked.
func TestStackEnvRevealNeedsManageAndNeverShowsTheRootOfTrust(t *testing.T) {
	svc := newListTestService(t)
	seedStack(t, svc, "s1", "web", false)
	if err := svc.saveEnvVars(context.Background(), "s1", map[string]string{
		"DB_PASSWORD": "hunter2hunter2", "PLAIN": "1", "COPIED": stackPinnedSecret,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rbac.UpsertCatalog(svc.db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('viewer', 'viewer', 'x'), ('ops', 'ops', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-view', 'r-view', '["stacks.view"]'), ('r-ops', 'r-ops', '["stacks.view","stacks.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'viewer', 'r-view', 'envAAA'), ('ur2', 'ops', 'r-ops', 'envAAA')`,
	} {
		if _, err := svc.db.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	h := &Handler{service: svc, app: &router.AppDeps{
		Config:      &config.Config{Secret: stackPinnedSecret},
		RBACService: rbac.NewService(svc.db),
		AuditLog:    audit.NewLogger(svc.db, ""),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
	r := chi.NewRouter()
	r.Get("/stacks/{name}/env-vars", h.HandleGetEnvVars)
	get := func(userID, query string) (int, map[string]string) {
		req := httptest.NewRequest(http.MethodGet, "/stacks/web/env-vars?env=envAAA"+query, nil)
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var body struct {
			Data map[string]string `json:"data"`
		}
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
		}
		return rec.Code, body.Data
	}

	code, masked := get("viewer", "")
	if code != http.StatusOK || masked["DB_PASSWORD"] != EnvMaskSentinel || masked["PLAIN"] != "1" || masked["COPIED"] != EnvMaskSentinel {
		t.Fatalf("viewer masked view: HTTP %d %v", code, masked)
	}
	if code, _ := get("viewer", "&reveal=true"); code != http.StatusForbidden {
		t.Fatalf("reveal with stacks.view only: HTTP %d, want 403", code)
	}
	code, revealed := get("ops", "&reveal=true")
	if code != http.StatusOK || revealed["DB_PASSWORD"] != "hunter2hunter2" || revealed["COPIED"] != EnvMaskSentinel {
		t.Fatalf("stacks.manage reveal: HTTP %d %v, want the password and the copied secret still masked", code, revealed)
	}
	var rows int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'reveal_env_vars' AND username = 'ops'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("reveal audit rows = %d, want 1", rows)
	}
}
