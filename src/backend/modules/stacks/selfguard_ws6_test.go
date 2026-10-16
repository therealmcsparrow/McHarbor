// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// TestAdoptingMcHarborsStackNeedsHostSelf is T6.10: the adopt route now asks for
// host_self.manage on a stack that runs McHarbor, as the Take over button already did.
func TestAdoptingMcHarborsStackNeedsHostSelf(t *testing.T) {
	svc, _ := sgService(t)
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
	r.Post("/stacks/adopt", h.HandleAdopt)
	adopt := func(user, body string) (int, string) {
		req := httptest.NewRequest(http.MethodPost, "/stacks/adopt?env=envAAA", strings.NewReader(body))
		req.Header.Set("X-Test-User", user)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	compose := `"services:\n  app:\n    image: custom:1\n"`

	for _, body := range []string{
		`{"name":"harbor","compose":` + compose + `}`,
		`{"name":"standalone","compose":` + compose + `,"containerId":"` + sgSelfID[:12] + `"}`,
	} {
		code, resp := adopt("ops", body)
		if code != http.StatusForbidden || !strings.Contains(resp, "err.host_self.stack_required") || !strings.Contains(resp, `"permission":"host_self.manage"`) {
			t.Errorf("adopt %s by ops: HTTP %d %s; want 403 naming host_self.manage", body, code, resp)
		}
	}
	var denied, stacks int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'host_self.denied' AND details LIKE '%op=adopt%'`).Scan(&denied); err != nil {
		t.Fatal(err)
	}
	if denied != 2 {
		t.Errorf("adopt refusal audit rows = %d, want 2", denied)
	}
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM stacks`).Scan(&stacks); err != nil {
		t.Fatal(err)
	}
	if stacks != 0 {
		t.Fatalf("a refused adopt recorded %d stacks", stacks)
	}

	if code, resp := adopt("ops", `{"name":"web","compose":`+compose+`}`); code != http.StatusCreated {
		t.Errorf("adopt of an ordinary stack by ops: HTTP %d %s; want 201", code, resp)
	}
	if code, resp := adopt("root", `{"name":"harbor","compose":`+compose+`}`); code != http.StatusCreated {
		t.Errorf("adopt of McHarbor's stack by a host_self.manage holder: HTTP %d %s; want 201", code, resp)
	}
}

// TestOwnedStackSeamRefusesMcHarborWithNobodyBehindIt is T6.5: GitOps deploys and tears
// down through DeployOwnedStack / RemoveOwnedStack with no user. Compose that reaches
// McHarbor's host, or a stack that runs McHarbor, is refused there unless an operator's
// host_self.manage decision rides on the context; an ordinary deploy is untouched.
func TestOwnedStackSeamRefusesMcHarborWithNobodyBehindIt(t *testing.T) {
	svc, _ := sgService(t)
	bare := context.Background()
	sock := "services:\n  sock:\n    image: docker:cli\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock\n"

	if _, err := svc.DeployOwnedStack(bare, "envAAA", "probe", "gitops:p1:prod", sock); !errors.Is(err, selfguard.ErrRequired) {
		t.Fatalf("unattended socket deploy on the self environment: %v; want a host_self refusal", err)
	}
	if existing, err := svc.ownedStack(bare, "probe"); err != nil || existing != nil {
		t.Fatalf("a refused deploy recorded a stack: %+v %v", existing, err)
	}
	if _, err := os.Stat(svc.getProjectPath("probe")); !os.IsNotExist(err) {
		t.Fatalf("a refused deploy wrote its project directory: %v", err)
	}
	if err := svc.ownedStackRefusal(selfguard.WithDecision(bare, true), "envAAA", "probe", nil, []byte(sock), svc.getProjectPath("probe"), nil); err != nil {
		t.Fatalf("socket deploy with an allowing decision: %v", err)
	}
	if err := svc.ownedStackRefusal(bare, "envBBB", "probe", nil, []byte(sock), svc.getProjectPath("probe"), nil); err != nil {
		t.Fatalf("unattended socket deploy on another environment: %v", err)
	}
	if err := svc.ownedStackRefusal(bare, "envAAA", "plain", nil, []byte("services:\n  web:\n    image: nginx\n"), svc.getProjectPath("plain"), nil); err != nil {
		t.Fatalf("unattended ordinary deploy on the self environment: %v", err)
	}

	if _, err := svc.db.Exec(`INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, managed_by, created_at, updated_at)
		VALUES ('st-h', 'harbor', 'envAAA', ?, 'docker-compose.yml', 'running', 'gitops-preview:x', '2026-09-01', '2026-09-01')`, svc.getProjectPath("harbor")); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveOwnedStack(bare, "envAAA", "harbor", "gitops-preview:x", true); !errors.Is(err, selfguard.ErrRequired) {
		t.Fatalf("unattended teardown of the stack that runs McHarbor: %v; want a host_self refusal", err)
	}
	if existing, err := svc.ownedStack(bare, "harbor"); err != nil || existing == nil {
		t.Fatalf("the refused teardown deleted the stack row: %v", err)
	}
	existing, err := svc.ownedStack(bare, "harbor")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ownedStackRefusal(selfguard.WithDecision(bare, true), "envAAA", "harbor", existing, nil, "", nil); err != nil {
		t.Fatalf("teardown with an allowing decision: %v", err)
	}
}
