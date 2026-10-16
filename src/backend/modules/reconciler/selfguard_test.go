// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// sgDaemon serves inspect for McHarbor's container (recognised by its image role label) and
// an ordinary one, both running, and records every stop and remove it receives.
type sgDaemon struct {
	mu    sync.Mutex
	calls []string
}

func (d *sgDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	docs := map[string]string{
		"harbor": `{"Id":"abababababab0000000000000000000000000000000000000000000000000001","Name":"/harbor","State":{"Running":true},"Config":{"Image":"x/harbor:1","Labels":{"com.mcharbor.role":"app"}},"HostConfig":{}}`,
		"web":    `{"Id":"cdcdcdcdcdcd0000000000000000000000000000000000000000000000000002","Name":"/web","State":{"Running":true},"Config":{"Image":"nginx","Labels":{}},"HostConfig":{}}`,
	}
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[1] != "containers" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := parts[2]
	if strings.HasPrefix(name, "abababababab") {
		name = "harbor"
	} else if strings.HasPrefix(name, "cdcdcdcdcdcd") {
		name = "web"
	}
	switch parts[3] {
	case "json":
		doc, ok := docs[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if _, err := io.WriteString(w, doc); err != nil {
			return
		}
	case "stop":
		d.mu.Lock()
		d.calls = append(d.calls, "stop:"+name)
		d.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (d *sgDaemon) called(call string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Contains(d.calls, call)
}

// TestReconcileRefusesMcHarborContainerWithoutHostSelf is T6.3: a manual reconcile that
// would stop McHarbor's own container needs host_self.manage, is audited when refused, and
// never reaches Docker; a holder, or an ordinary container, is reconciled as before.
func TestReconcileRefusesMcHarborContainerWithoutHostSelf(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "sg.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if err := rbac.UpsertCatalog(database); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('ops', 'ops', 'x'), ('root', 'root', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-hs', 'r-hs', '["host_self.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('u1', 'root', 'r-hs', NULL)`,
		`INSERT INTO desired_states (id, name, env_id, container_name, image_ref, desired_status, created_at, updated_at) VALUES
			('ds-self', 'ds-self', 'envA', 'harbor', 'x/harbor:1', 'stopped', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z'),
			('ds-web', 'ds-web', 'envA', 'web', 'nginx', 'stopped', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	daemon := &sgDaemon{}
	srv := httptest.NewServer(daemon)
	t.Cleanup(srv.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	pool := coredocker.NewClientPool(database, nil, logger)
	pool.SetClient("envA", cli)
	rbacSvc := rbac.NewService(database)
	auditLog := coreAudit.NewLogger(database, "")
	h := NewHandler(&router.AppDeps{DB: database, DockerPool: pool, RBACService: rbacSvc, AuditLog: auditLog, Logger: logger, Encryption: testEncryption(t)}, nil)
	mux := chi.NewRouter()
	mux.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id := req.Header.Get("X-Test-User")
			next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: id, Username: id})))
		})
	})
	mux.Use(selfguard.Middleware(rbacSvc, auditLog))
	mux.Post("/reconciler/{id}/reconcile", h.HandleReconcile)
	call := func(user, id string) (int, string) {
		req := httptest.NewRequest(http.MethodPost, "/reconciler/"+id+"/reconcile", nil)
		req.Header.Set("X-Test-User", user)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	code, body := call("ops", "ds-self")
	if code != http.StatusForbidden || !strings.Contains(body, "err.host_self.container_required") || !strings.Contains(body, `"permission":"host_self.manage"`) {
		t.Fatalf("reconcile stopping McHarbor by ops: HTTP %d %s; want 403 naming host_self.manage", code, body)
	}
	if daemon.called("stop:harbor") {
		t.Fatal("the refused reconcile stopped McHarbor's container")
	}
	var denied int
	if err := database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'host_self.denied' AND details LIKE '%op=reconcile%'`).Scan(&denied); err != nil {
		t.Fatal(err)
	}
	if denied != 1 {
		t.Fatalf("host_self.denied rows = %d, want 1", denied)
	}

	if code, body := call("ops", "ds-web"); code != http.StatusOK || !daemon.called("stop:web") {
		t.Errorf("reconcile of an ordinary container by ops: HTTP %d %s; want 200 and a stop", code, body)
	}
	if code, body := call("root", "ds-self"); code != http.StatusOK || !daemon.called("stop:harbor") {
		t.Errorf("reconcile of McHarbor by a host_self.manage holder: HTTP %d %s; want 200 and a stop", code, body)
	}
}
