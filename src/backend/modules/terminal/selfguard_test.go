// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package terminal

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
	dockerclient "github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

const termSelfID = "9a9a9a9a9a9a0000000000000000000000000000000000000000000000000001"

// TestShellInMcHarborsContainerNeedsHostSelf: the terminal opened a shell in McHarbor's own
// container for system.manage, and in a renamed one for anybody holding terminal.access.
// It now needs host_self.manage, and the refusal is audited.
func TestShellInMcHarborsContainerNeedsHostSelf(t *testing.T) {
	coredocker.SetSelfContainerIDs([]string{termSelfID[:12]})
	t.Cleanup(func() { coredocker.SetSelfContainerIDs(nil) })

	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "term.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
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
		`INSERT INTO users (id, username, password_hash) VALUES ('ops', 'ops', 'x'), ('root', 'root', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-ops', 'r-ops', '["containers.manage","system.manage"]'), ('r-hs', 'r-hs', '["host_self.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('u1', 'ops', 'r-ops', NULL), ('u2', 'root', 'r-ops', NULL), ('u3', 'root', 'r-hs', NULL)`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var doc string
		switch {
		case strings.Contains(r.URL.Path, "/containers/harbor/json"):
			doc = `{"Id":"` + termSelfID + `","Name":"/harbor","Config":{"Image":"custom:1","Labels":{}},"HostConfig":{}}`
		case strings.Contains(r.URL.Path, "/containers/web/json"):
			doc = `{"Id":"8b8b8b8b8b8b0000000000000000000000000000000000000000000000000002","Name":"/web","Config":{"Image":"nginx","Labels":{}},"HostConfig":{}}`
		default:
			w.WriteHeader(http.StatusNotFound)
			doc = `{"message":"not found"}`
		}
		if _, err := io.WriteString(w, doc); err != nil {
			t.Errorf("fake daemon write: %v", err)
		}
	}))
	t.Cleanup(daemon.Close)
	cli, err := dockerclient.New(dockerclient.WithHost("tcp://"+strings.TrimPrefix(daemon.URL, "http://")), dockerclient.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := coredocker.NewClientPool(database, nil, logger)
	pool.SetClient("envA", cli)
	rbacSvc := rbac.NewService(database)
	auditLog := audit.NewLogger(database, "")
	h := NewHandler(&router.AppDeps{DB: database, DockerPool: pool, Config: &config.Config{}, RBACService: rbacSvc, AuditLog: auditLog, Logger: logger})

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id := req.Header.Get("X-Test-User")
			next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: id, Username: id})))
		})
	})
	r.Use(selfguard.Middleware(rbacSvc, auditLog))
	r.Get("/terminal/ws", h.HandleWS)
	open := func(user, target string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, "/terminal/ws?env=envA&container="+target, nil)
		req.Header.Set("X-Test-User", user)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	if code, body := open("ops", "harbor"); code != http.StatusForbidden || !strings.Contains(body, "err.host_self.container_required") {
		t.Fatalf("shell in McHarbor's container with system.manage but no host_self.manage: HTTP %d %s", code, body)
	}
	var denied int
	if err := database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'host_self.denied'`).Scan(&denied); err != nil {
		t.Fatal(err)
	}
	if denied != 1 {
		t.Fatalf("terminal refusal audit rows = %d, want 1", denied)
	}
	// Past the gate the handler tries a WebSocket upgrade, which a plain request fails:
	// anything but the host_self refusal means the gate let it through.
	for _, tc := range []struct{ user, target string }{{"root", "harbor"}, {"ops", "web"}} {
		if code, body := open(tc.user, tc.target); strings.Contains(body, "err.host_self") || code == http.StatusForbidden {
			t.Errorf("shell for %s in %s: HTTP %d %s; want the gate passed", tc.user, tc.target, code, body)
		}
	}
}
