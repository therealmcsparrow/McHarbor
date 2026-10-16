// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package approvals

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

type fakeEnc struct{}

func (fakeEnc) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (fakeEnc) Decrypt(p string) (string, error) { return strings.TrimPrefix(p, "enc:"), nil }

// allowAll grants every permission to the users in allowed.
type allowAll struct{ allowed map[string]bool }

func (a allowAll) RouteDecision(_ *http.Request, userID, _ string, _ rbac.Permission) (bool, error) {
	return a.allowed[userID], nil
}
func (a allowAll) HasStackPermission(_ context.Context, userID, _, _ string, _ rbac.Permission) (bool, error) {
	return a.allowed[userID], nil
}

type fixture struct {
	db      *sql.DB
	svc     *Service
	root    http.Handler
	calls   atomic.Int32
	lastRaw atomic.Value
}

func newFixture(t *testing.T, checker PermissionChecker) *fixture {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "a.db")+"?_pragma=foreign_keys(1)")
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
	f := &fixture{db: database, svc: NewService(database, fakeEnc{})}
	gate := NewGate(f.svc, checker, audit.NewLogger(database, "s"), slog.Default(), nil)

	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					if auth.UserFromContext(req.Context()) == nil {
						id := req.Header.Get("X-User")
						req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: id, Username: id}))
					}
					next.ServeHTTP(w, req)
				})
			})
			r.Use(gate.Middleware)
			handler := func(w http.ResponseWriter, req *http.Request) {
				raw, err := io.ReadAll(req.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				f.lastRaw.Store(string(raw))
				f.calls.Add(1)
				w.WriteHeader(http.StatusNoContent)
			}
			r.Route("/containers", func(r chi.Router) {
				r.Route("/{id}", func(r chi.Router) {
					r.Delete("/", handler)
				})
			})
			r.Post("/appstore/plugins/uninstall", handler)
		})
	})
	f.root = r
	return f
}

func (f *fixture) do(t *testing.T, method, path, user, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-User", user)
	rec := httptest.NewRecorder()
	f.root.ServeHTTP(rec, req)
	return rec
}

func (f *fixture) rule(t *testing.T, category string, approvers int) *Rule {
	t.Helper()
	r, err := f.svc.CreateRule(context.Background(), RuleInput{Category: category, RequiredApprovers: approvers, ExpiryMinutes: 60}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func heldID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Success bool         `json:"success"`
		Data    HeldResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Success || env.Data.Reason != ReasonApprovalRequired || env.Data.ApprovalID == "" {
		t.Fatalf("held envelope: %+v", env)
	}
	return env.Data.ApprovalID
}

var yes = func(context.Context, string, string) (bool, error) { return true, nil }

func TestNoRuleIsTodaysBehaviour(t *testing.T) {
	f := newFixture(t, allowAll{allowed: map[string]bool{"req": true}})
	if rec := f.do(t, "DELETE", "/api/containers/c1?env=e1", "req", ""); rec.Code != http.StatusNoContent || f.calls.Load() != 1 {
		t.Fatalf("ungated delete: %d calls=%d", rec.Code, f.calls.Load())
	}
}

func TestRuleHoldsRequestAndDoesNotRunIt(t *testing.T) {
	f := newFixture(t, allowAll{allowed: map[string]bool{"req": true}})
	f.rule(t, CategoryContainerRemove, 1)
	id := heldID(t, f.do(t, "DELETE", "/api/containers/c1?env=e1", "req", ""))
	if f.calls.Load() != 0 {
		t.Fatal("a held request reached its handler")
	}
	again := heldID(t, f.do(t, "DELETE", "/api/containers/c1?env=e1", "req", ""))
	if again != id {
		t.Fatalf("a repeated click queued a second approval: %s vs %s", again, id)
	}
	req, err := f.svc.Get(context.Background(), id)
	if err != nil || req.Status != StatusPending || req.ResourceName != "c1" || req.Path != "/api/containers/c1?env=e1" {
		t.Fatalf("held request: %+v %v", req, err)
	}
}

func TestCallerWithoutPermissionIsNotQueued(t *testing.T) {
	f := newFixture(t, allowAll{allowed: map[string]bool{}})
	f.rule(t, CategoryContainerRemove, 1)
	f.do(t, "DELETE", "/api/containers/c1", "nobody", "")
	if f.calls.Load() != 1 {
		t.Fatal("a caller the route would refuse must reach the route's own gate, not the queue")
	}
	items, _, err := f.svc.List(context.Background(), "", "", 1, 25)
	if err != nil || len(items) != 0 {
		t.Fatalf("queued for an unauthorised caller: %d %v", len(items), err)
	}
}

func TestSecretsAreRedactedButReplayedExactly(t *testing.T) {
	f := newFixture(t, allowAll{allowed: map[string]bool{"req": true}})
	f.rule(t, CategoryPluginUninstall, 1)
	body := `{"slug":"addon-kvm","token":"s3cr3t"}`
	id := heldID(t, f.do(t, "POST", "/api/appstore/plugins/uninstall", "req", body))
	req, err := f.svc.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(req.BodyRedacted, "s3cr3t") || !strings.Contains(req.BodyRedacted, "addon-kvm") || req.ResourceName != "addon-kvm" {
		t.Fatalf("snapshot: %q resource=%q", req.BodyRedacted, req.ResourceName)
	}
	if _, err := f.svc.Decide(context.Background(), id, "boss", "boss", DecideInput{Approve: true}, yes); err != nil {
		t.Fatal(err)
	}
	claimed, raw, err := f.svc.Claim(context.Background(), id, "req")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), auth.UserContextKey, &auth.User{ID: "req", Username: "req"})
	from := httptest.NewRequest("POST", "/api/approvals/"+id+"/execute", nil)
	cw := replay(ctx, f.root, from, claimed, raw)
	if cw.status != http.StatusNoContent || f.calls.Load() != 1 || f.lastRaw.Load() != body {
		t.Fatalf("replay: status=%d calls=%d body=%v", cw.status, f.calls.Load(), f.lastRaw.Load())
	}
}

func TestSeparationOfDutiesAndApproverCount(t *testing.T) {
	f := newFixture(t, allowAll{allowed: map[string]bool{"req": true}})
	f.rule(t, CategoryContainerRemove, 2)
	id := heldID(t, f.do(t, "DELETE", "/api/containers/c1", "req", ""))
	ctx := context.Background()
	if _, err := f.svc.Decide(ctx, id, "req", "req", DecideInput{Approve: true}, yes); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("self-approval: %v", err)
	}
	no := func(context.Context, string, string) (bool, error) { return false, nil }
	if _, err := f.svc.Decide(ctx, id, "b1", "b1", DecideInput{Approve: true}, no); !errors.Is(err, ErrApproverLacksPerm) {
		t.Fatalf("approver without the rule's permission: %v", err)
	}
	got, err := f.svc.Decide(ctx, id, "b1", "b1", DecideInput{Approve: true}, yes)
	if err != nil || got.Status != StatusPending || got.ApprovalsCount != 1 {
		t.Fatalf("first of two approvals: %+v %v", got, err)
	}
	if _, err := f.svc.Decide(ctx, id, "b1", "b1", DecideInput{Approve: true}, yes); !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("same approver twice: %v", err)
	}
	if _, _, err := f.svc.Claim(ctx, id, "req"); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("executing before the count is met: %v", err)
	}
	got, err = f.svc.Decide(ctx, id, "b2", "b2", DecideInput{Approve: true}, yes)
	if err != nil || got.Status != StatusApproved {
		t.Fatalf("second approval: %+v %v", got, err)
	}
}

func TestExecuteOnlyOnceAndOnlyByRequester(t *testing.T) {
	f := newFixture(t, allowAll{allowed: map[string]bool{"req": true}})
	f.rule(t, CategoryContainerRemove, 1)
	id := heldID(t, f.do(t, "DELETE", "/api/containers/c1", "req", ""))
	ctx := context.Background()
	if _, err := f.svc.Decide(ctx, id, "boss", "boss", DecideInput{Approve: true}, yes); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Claim(ctx, id, "boss"); !errors.Is(err, ErrNotRequester) {
		t.Fatalf("approver executing: %v", err)
	}
	if _, _, err := f.svc.Claim(ctx, id, "req"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Claim(ctx, id, "req"); !errors.Is(err, ErrAlreadyExecuted) {
		t.Fatalf("second claim: %v", err)
	}
	done, err := f.svc.Finish(ctx, id, 204, "")
	if err != nil || done.Status != StatusExecuted || done.ResultStatus == nil || *done.ResultStatus != 204 {
		t.Fatalf("finish: %+v %v", done, err)
	}
}

func TestDeniedAndExpiredCannotExecute(t *testing.T) {
	f := newFixture(t, allowAll{allowed: map[string]bool{"req": true}})
	f.rule(t, CategoryContainerRemove, 1)
	ctx := context.Background()
	denied := heldID(t, f.do(t, "DELETE", "/api/containers/c1", "req", ""))
	if _, err := f.svc.Decide(ctx, denied, "boss", "boss", DecideInput{Approve: false}, yes); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Claim(ctx, denied, "req"); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("denied claim: %v", err)
	}

	expiring := heldID(t, f.do(t, "DELETE", "/api/containers/c2", "req", ""))
	if _, err := f.svc.Decide(ctx, expiring, "boss", "boss", DecideInput{Approve: true}, yes); err != nil {
		t.Fatal(err)
	}
	f.svc.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, _, err := f.svc.Claim(ctx, expiring, "req"); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired claim: %v", err)
	}
	if got, err := f.svc.Get(ctx, expiring); err != nil || got.Status != StatusExpired {
		t.Fatalf("expired status: %s", got.Status)
	}
}

func TestExecuteHandlerReplaysThroughRouter(t *testing.T) {
	f := newFixture(t, allowAll{allowed: map[string]bool{"req": true}})
	f.rule(t, CategoryContainerRemove, 1)
	id := heldID(t, f.do(t, "DELETE", "/api/containers/c9", "req", ""))
	if _, err := f.svc.Decide(context.Background(), id, "boss", "boss", DecideInput{Approve: true}, yes); err != nil {
		t.Fatal(err)
	}
	h := &Handler{app: &router.AppDeps{DB: f.db, Logger: slog.Default(), AuditLog: audit.NewLogger(f.db, "s"), Root: f.root}, svc: f.svc}
	exec := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/approvals/"+id+"/execute", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", id)
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		ctx = context.WithValue(ctx, auth.UserContextKey, &auth.User{ID: "req", Username: "req"})
		rec := httptest.NewRecorder()
		h.HandleExecute(rec, req.WithContext(ctx))
		return rec
	}
	if rec := exec(); rec.Code != http.StatusOK || f.calls.Load() != 1 {
		t.Fatalf("execute: %d calls=%d %s", rec.Code, f.calls.Load(), rec.Body.String())
	}
	if rec := exec(); rec.Code != http.StatusConflict || f.calls.Load() != 1 {
		t.Fatalf("second execute must not replay: %d calls=%d", rec.Code, f.calls.Load())
	}
}

func TestDryRunBulkDeleteIsNotHeld(t *testing.T) {
	g, _ := matchRoute("POST", "/users/bulk/delete")
	if g == nil || !g.SkipWhenDryRun || !bodyDryRun([]byte(`{"dryRun":true}`)) || bodyDryRun([]byte(`{"dryRun":false}`)) {
		t.Fatal("bulk delete dry-run must pass the gate unheld")
	}
}

func TestRuleValidation(t *testing.T) {
	for _, in := range []RuleInput{
		{Category: "container.explode"},
		{Category: CategoryUserDelete, RequiredApprovers: 9},
		{Category: CategoryUserDelete, ExpiryMinutes: 1},
		{Category: CategoryUserDelete, ApproverPermission: "not.a_key"},
	} {
		if err := validateRule(&in); !errors.Is(err, ErrInvalidRule) {
			t.Fatalf("%+v accepted", in)
		}
	}
	ok := RuleInput{Category: CategoryUserDelete}
	if err := validateRule(&ok); err != nil || ok.RequiredApprovers != 1 || ok.ApproverPermission != "approvals.decide" {
		t.Fatalf("defaults: %+v %v", ok, err)
	}
}

func TestEveryCategoryRouteMatches(t *testing.T) {
	for _, g := range guardedRoutes {
		concrete := strings.NewReplacer("{id}", "x1", "{name}", "n1").Replace(g.Pattern)
		got, _ := matchRoute(g.Method, concrete+"/")
		if got == nil || got.Category != g.Category {
			t.Fatalf("%s %s did not match its own category", g.Method, concrete)
		}
	}
	if got, _ := matchRoute("GET", "/containers/x1"); got != nil {
		t.Fatal("a read matched a destructive category")
	}
}
