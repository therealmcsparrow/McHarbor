// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

const sgSelfID = "abababababab0000000000000000000000000000000000000000000000000007"

// sgDaemon serves one McHarbor container (by id only) in compose project "harbor" and one
// ordinary container in project "web", and records stops.
type sgDaemon struct {
	hostsSelf bool
	// links, probeErr and probes drive and record the stand-in for the bind-check helper.
	links    map[string]bool
	probeErr error
	probes   [][]string
	mu       sync.Mutex
	stops    []string
}

func (d *sgDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/containers/json"):
		list := `[{"Id":"cdcdcdcdcdcd0000000000000000000000000000000000000000000000000008","Names":["/web-app-1"],"Image":"nginx","State":"running","Labels":{"com.docker.compose.project":"web"}}`
		if d.hostsSelf {
			list += `,{"Id":"` + sgSelfID + `","Names":["/harbor-app-1"],"Image":"custom:1","State":"running","Labels":{"com.docker.compose.project":"harbor"}}`
		}
		sgWrite(w, list+`]`)
	case strings.HasSuffix(path, "/stop"):
		d.mu.Lock()
		d.stops = append(d.stops, path)
		d.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case d.hostsSelf && strings.Contains(path, "/containers/"+sgSelfID[:12]):
		sgWrite(w, `{"Id":"`+sgSelfID+`","Name":"/harbor-app-1","Config":{"Image":"custom:1","Labels":{"com.docker.compose.project":"harbor"}},"Mounts":[]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		sgWrite(w, `{"message":"not found"}`)
	}
}

func (d *sgDaemon) stopped() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.stops)
}

func sgService(t *testing.T) (*Service, *sgDaemon) {
	t.Helper()
	docker.SetSelfContainerIDs([]string{sgSelfID[:12]})
	t.Cleanup(func() { docker.SetSelfContainerIDs(nil) })
	svc := newListTestService(t)
	if _, err := svc.db.Exec("INSERT INTO environments (id, name) VALUES ('envBBB', 'Other')"); err != nil {
		t.Fatal(err)
	}
	pool := docker.NewClientPool(svc.db, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	self := &sgDaemon{hostsSelf: true}
	for env, d := range map[string]*sgDaemon{"envAAA": self, "envBBB": {}} {
		srv := httptest.NewServer(d)
		t.Cleanup(srv.Close)
		cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")), client.WithAPIVersion("1.47"))
		if err != nil {
			t.Fatal(err)
		}
		pool.SetClient(env, cli)
	}
	// The fake daemon runs no helper; this stands in for it, answering from self.links.
	pool.SetBindSymlinkCheck(func(_ context.Context, _ string, _ docker.SelfHost, paths []string) (map[string]bool, error) {
		self.mu.Lock()
		defer self.mu.Unlock()
		self.probes = append(self.probes, paths)
		if self.probeErr != nil {
			return nil, self.probeErr
		}
		out := map[string]bool{}
		for _, p := range paths {
			if self.links[p] {
				out[p] = true
			}
		}
		return out, nil
	})
	t.Cleanup(func() { pool.SetBindSymlinkCheck(nil) })
	svc.dockerPool = pool
	return svc, self
}

// TestStackThatRunsMcHarborNeedsHostSelf covers "stack down/recreate that would take McHarbor
// down": stacks.manage alone stopped the stack holding McHarbor's own container.
func TestStackThatRunsMcHarborNeedsHostSelf(t *testing.T) {
	svc, daemon := sgService(t)
	if err := rbac.UpsertCatalog(svc.db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('ops', 'ops', 'x'), ('root', 'root', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-ops', 'r-ops', '["stacks.view","stacks.manage"]'), ('r-hs', 'r-hs', '["host_self.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('u1', 'ops', 'r-ops', 'envAAA'), ('u2', 'root', 'r-ops', 'envAAA'), ('u3', 'root', 'r-hs', NULL)`,
	} {
		if _, err := svc.db.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	rbacSvc := rbac.NewService(svc.db)
	auditLog := audit.NewLogger(svc.db, "")
	h := &Handler{service: svc, app: &router.AppDeps{
		Config: &config.Config{}, RBACService: rbacSvc, AuditLog: auditLog,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id := req.Header.Get("X-Test-User")
			next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: id, Username: id})))
		})
	})
	r.Use(selfguard.Middleware(rbacSvc, auditLog))
	r.Post("/stacks/{name}/stop", h.HandleStop)
	stop := func(user, name string) (int, string) {
		req := httptest.NewRequest(http.MethodPost, "/stacks/"+name+"/stop?env=envAAA", nil)
		req.Header.Set("X-Test-User", user)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	if code, body := stop("ops", "harbor"); code != http.StatusForbidden || !strings.Contains(body, "err.host_self.stack_required") {
		t.Fatalf("stop of McHarbor's stack by ops: HTTP %d %s; want 403 err.host_self.stack_required", code, body)
	}
	if daemon.stopped() != 0 {
		t.Fatal("the refused stop reached Docker")
	}
	var denied int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'host_self.denied' AND entity_type = 'stack'`).Scan(&denied); err != nil {
		t.Fatal(err)
	}
	if denied != 1 {
		t.Fatalf("stack refusal audit rows = %d, want 1", denied)
	}
	if code, body := stop("ops", "web"); code != http.StatusOK {
		t.Fatalf("stop of an ordinary stack by ops: HTTP %d %s", code, body)
	}
	if code, body := stop("root", "harbor"); code != http.StatusOK {
		t.Fatalf("stop of McHarbor's stack by a host_self.manage holder: HTTP %d %s", code, body)
	}
	if daemon.stopped() != 2 {
		t.Fatalf("stops reaching Docker = %d, want 2 (web by ops, harbor by root)", daemon.stopped())
	}
}

// TestComposeUpThatReachesTheHostNeedsHostSelf is the deploy half of the create-time rule:
// a compose file mounting the Docker socket onto McHarbor's own environment.
func TestComposeUpThatReachesTheHostNeedsHostSelf(t *testing.T) {
	svc, _ := sgService(t)
	dir := filepath.Join(t.TempDir(), "probe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	compose := "services:\n  sock:\n    image: docker:cli\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock\n"
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	user := context.WithValue(context.Background(), auth.UserContextKey, &auth.User{ID: "ops"})

	ref := svc.composeHostAccessRefusal(selfguard.WithDecision(user, false), "docker-compose.yml", dir, nil, "envAAA")
	if ref == nil || !slices.Contains(ref.Reasons, "sock:docker_socket") {
		t.Fatalf("socket compose on the self environment without host_self.manage: refusal %+v", ref)
	}
	if ref := svc.composeHostAccessRefusal(selfguard.WithDecision(user, true), "docker-compose.yml", dir, nil, "envAAA"); ref != nil {
		t.Fatalf("a host_self.manage holder was refused: %+v", ref)
	}
	if ref := svc.composeHostAccessRefusal(selfguard.WithDecision(user, false), "docker-compose.yml", dir, nil, "envBBB"); ref != nil {
		t.Fatalf("the rule reached an environment McHarbor does not run on: %+v", ref)
	}
}

func sgWrite(w io.Writer, s string) {
	if _, err := io.WriteString(w, s); err != nil {
		slog.Debug("stacks test daemon: write failed", "error", err)
	}
}
