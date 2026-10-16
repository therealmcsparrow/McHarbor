// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// envPinRouter serves /api/agents/{envId}/secrets behind PinEnvToPath and an
// environment-scoped containers.manage gate, for a user whose only grant is on envA.
// The handler echoes the `?env=` it sees in X-Env, so a test can tell which scope reached it.
func envPinRouter(t *testing.T) http.Handler {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "pin.db")+"?_pragma=foreign_keys(0)")
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
	if err := UpsertCatalog(database); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r1', 'r1', '["containers.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'u1', 'r1', 'envA')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	svc := NewService(database)
	r := chi.NewRouter()
	r.Route("/api/agents/{envId}", func(r chi.Router) {
		r.Use(PinEnvToPath("envId"))
		r.With(RequirePermission(svc, PermContainersManage)).Get("/secrets", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Env", response.ParseEnvID(r))
			w.WriteHeader(http.StatusOK)
		})
	})
	return r
}

func callAs(h http.Handler, url string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "op"}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPinEnvToPathRefusesAGrantOnAnotherEnvironment(t *testing.T) {
	h := envPinRouter(t)
	if rec := callAs(h, "/api/agents/envB/secrets?env=envA"); rec.Code != http.StatusForbidden {
		t.Fatalf("a grant on envA must not reach envB through ?env=envA: got %d", rec.Code)
	}
	if rec := callAs(h, "/api/agents/envB/secrets"); rec.Code != http.StatusForbidden {
		t.Fatalf("no grant on envB: got %d", rec.Code)
	}
}

func TestPinEnvToPathScopesTheGrantToThePath(t *testing.T) {
	h := envPinRouter(t)
	for _, url := range []string{"/api/agents/envA/secrets", "/api/agents/envA/secrets?env=envB"} {
		rec := callAs(h, url)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: the grant on envA must pass on envA's own path: got %d", url, rec.Code)
		}
		if got := rec.Header().Get("X-Env"); got != "envA" {
			t.Fatalf("%s: the handler must see the path environment as ?env=, got %q", url, got)
		}
	}
}

func TestPinEnvToPathReportsItselfToAProbe(t *testing.T) {
	reqs := ProbeChain(PinEnvToPath("envId"), RequirePermission(nil, PermEnvironmentsView))
	if len(reqs) != 2 {
		t.Fatalf("want the pin and the gate, got %+v", reqs)
	}
	if param, ok := reqs[0].EnvPin(); !ok || param != "envId" {
		t.Fatalf("first entry must be the envId pin, got %+v", reqs[0])
	}
	if _, ok := reqs[1].EnvPin(); ok {
		t.Fatalf("the gate must not read as a pin: %+v", reqs[1])
	}
}

func TestPinEnvToResourceDecidesOnTheRowsOwnEnvironment(t *testing.T) {
	database := migratedRBACDB(t)
	mustExec(t, database, `INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`)
	mustExec(t, database, `INSERT INTO roles (id, name, permissions) VALUES ('ops', 'ops', '["containers.manage"]')`)
	mustExec(t, database, `INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'u1', 'ops', 'envA')`)
	svc := NewService(database)

	rows := map[string]string{"in-b": "envB", "in-a": "envA", "global": ""}
	resolve := func(r *http.Request) (string, bool, error) {
		env, ok := rows[chi.URLParam(r, "id")]
		return env, ok, nil
	}
	r := chi.NewRouter()
	r.Route("/api/things/{id}", func(r chi.Router) {
		r.Use(PinEnvToResource(resolve))
		r.With(RequirePermission(svc, PermContainersManage)).Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Env", response.ParseEnvID(r))
			w.WriteHeader(http.StatusOK)
		})
	})

	cases := []struct {
		url     string
		code    int
		seesEnv string
	}{
		{"/api/things/in-b/?env=envA", http.StatusForbidden, ""},
		{"/api/things/in-a/?env=envB", http.StatusOK, "envA"},
		{"/api/things/global/?env=envA", http.StatusForbidden, ""},
		{"/api/things/missing/?env=envA", http.StatusOK, "envA"},
	}
	for _, c := range cases {
		rec := callAs(r, c.url)
		if rec.Code != c.code {
			t.Fatalf("%s: got %d, want %d", c.url, rec.Code, c.code)
		}
		if c.seesEnv != "" && rec.Header().Get("X-Env") != c.seesEnv {
			t.Fatalf("%s: handler saw env %q, want %q", c.url, rec.Header().Get("X-Env"), c.seesEnv)
		}
	}
}
