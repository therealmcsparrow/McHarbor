// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package debugshell

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// mountForTest serves the real route table over the harness's service, with the
// real RBAC middleware in front, so a missing key is refused before any Docker call.
func mountForTest(t *testing.T, h *harness) (http.Handler, *apispec.Registry) {
	t.Helper()
	sharedMu.Lock()
	shared = h.svc
	sharedMu.Unlock()
	t.Cleanup(func() {
		sharedMu.Lock()
		shared = nil
		sharedMu.Unlock()
	})
	app := &router.AppDeps{DB: h.db, RBACService: rbac.NewService(h.db), AuditLog: audit.NewLogger(h.db, "test-secret"), Logger: slog.Default()}
	mux := chi.NewRouter()
	reg := apispec.NewRegistry()
	apispec.NewRouter(mux, reg, "/api", apispec.GroupProtected).Route("/containers/{id}", func(r *apispec.Router) {
		MountContainerRoutes(r, app)
	})
	return mux, reg
}

func as(userID string, req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
}

func TestRoutesAreGatedOnTheDebugShellKey(t *testing.T) {
	_, reg := mountForTest(t, newHarness(t))
	want := map[string]string{
		"POST /api/containers/{id}/debug-shell":   string(rbac.PermContainersDebugShell),
		"DELETE /api/containers/{id}/debug-shell": string(rbac.PermContainersDebugShell),
		"GET /api/containers/{id}/debug-shell":    string(rbac.PermContainersView),
	}
	for _, op := range reg.Operations() {
		key := op.Method + " " + op.Path
		perm, ok := want[key]
		if !ok {
			continue
		}
		delete(want, key)
		if keys := op.PermissionKeys(); len(keys) != 1 || keys[0] != perm {
			t.Errorf("%s is gated on %v, want %s", key, keys, perm)
		}
	}
	if len(want) != 0 {
		t.Errorf("routes not declared: %v", want)
	}
}

func TestInstallIsRefusedWithoutThePermission(t *testing.T) {
	h := newHarness(t)
	mux, _ := mountForTest(t, h)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, as("nobody", httptest.NewRequest(http.MethodPost, "/containers/svc/debug-shell?env=env1", nil)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403: %s", rec.Code, rec.Body.String())
	}
	if len(h.docker.execs) != 0 || len(h.docker.uploads) != 0 || h.docker.infoCalls != 0 {
		t.Fatalf("a refused install must not reach Docker (execs=%d uploads=%d)", len(h.docker.execs), len(h.docker.uploads))
	}
}

func TestInstallWritesTheAuditRowWithTheDigest(t *testing.T) {
	h := newHarness(t)
	mux, _ := mountForTest(t, h)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, as("system", httptest.NewRequest(http.MethodPost, "/containers/svc/debug-shell?env=env1", nil)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data Session `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var details string
	if err := h.db.QueryRow(`SELECT details FROM audit_logs WHERE action = 'debug_shell_install' AND entity_id = ?`, testContainerID).Scan(&details); err != nil {
		t.Fatalf("no debug_shell_install row: %v", err)
	}
	if body.Data.BusyboxDigest == "" || !strings.Contains(details, body.Data.BusyboxDigest) || !strings.Contains(details, body.Data.ID) {
		t.Fatalf("audit details %s must carry digest %s and session %s", details, body.Data.BusyboxDigest, body.Data.ID)
	}
}

func TestAShellBearingContainerIsRefusedWith409(t *testing.T) {
	h := newHarness(t)
	h.docker.shell = true
	mux, _ := mountForTest(t, h)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, as("system", httptest.NewRequest(http.MethodPost, "/containers/svc/debug-shell?env=env1", nil)))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "err.container.has_shell") {
		t.Fatalf("status %d body %s, want 409 err.container.has_shell", rec.Code, rec.Body.String())
	}
}
