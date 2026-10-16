// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apispec_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

const (
	permView   rbac.Permission = "volumes.view"
	permManage rbac.Permission = "volumes.manage"
	permDelete rbac.Permission = "volumes.delete"
)

func noop(w http.ResponseWriter, r *http.Request) {}

// mountSample mirrors the shape of a real module's routes.go closely enough
// that a regression in nesting, prefixes or probing shows up here.
func mountSample(rt *apispec.Router, svc *rbac.Service) {
	rt.Route("/volumes", func(r *apispec.Router) {
		r.With(rbac.RequirePermission(svc, permView)).Get("/", noop)
		r.With(rbac.RequirePermission(svc, permManage)).Post("/", noop)
		r.Route("/{name}", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(svc, permView)).Get("/", noop)
			r.With(rbac.RequirePermission(svc, permDelete)).Delete("/", noop)
		})
	})
}

func buildSample(t *testing.T) (*apispec.Registry, chi.Router) {
	t.Helper()
	mux := chi.NewRouter()
	reg := apispec.NewRegistry()
	mountSample(apispec.NewRouter(mux, reg, "/api", apispec.GroupProtected), nil)
	return reg, mux
}

func find(reg *apispec.Registry, method, path string) (apispec.Operation, bool) {
	for _, op := range reg.Operations() {
		if op.Method == method && op.Path == path {
			return op, true
		}
	}
	return apispec.Operation{}, false
}

func TestRouterRecordsNestedPaths(t *testing.T) {
	reg, _ := buildSample(t)

	want := []struct{ method, path string }{
		{"GET", "/api/volumes"},
		{"POST", "/api/volumes"},
		{"GET", "/api/volumes/{name}"},
		{"DELETE", "/api/volumes/{name}"},
	}
	if got := len(reg.Operations()); got != len(want) {
		t.Fatalf("recorded %d operations, want %d", got, len(want))
	}
	for _, w := range want {
		if _, ok := find(reg, w.method, w.path); !ok {
			t.Errorf("no operation recorded for %s %s", w.method, w.path)
		}
	}
}

// TestPermissionsRecoveredFromMiddleware is the load-bearing test of the whole
// design: the permission is read back off the chain that enforces it, so the
// spec cannot disagree with the gate.
func TestPermissionsRecoveredFromMiddleware(t *testing.T) {
	reg, _ := buildSample(t)

	cases := []struct {
		method, path string
		want         rbac.Permission
	}{
		{"GET", "/api/volumes", permView},
		{"POST", "/api/volumes", permManage},
		{"GET", "/api/volumes/{name}", permView},
		{"DELETE", "/api/volumes/{name}", permDelete},
	}

	for _, c := range cases {
		op, ok := find(reg, c.method, c.path)
		if !ok {
			t.Fatalf("no operation for %s %s", c.method, c.path)
		}
		keys := op.PermissionKeys()
		if len(keys) != 1 || keys[0] != string(c.want) {
			t.Errorf("%s %s: recovered %v, want [%s]", c.method, c.path, keys, c.want)
		}
		if len(op.Perms) != 1 || op.Perms[0].Scope != rbac.ScopeEnvironment {
			t.Errorf("%s %s: scope %v, want environment", c.method, c.path, op.Perms)
		}
	}
}

// TestNestedPermissionsDoNotLeak guards the aliasing bug a shared slice would
// cause: a permission applied to one branch must not appear on its sibling.
func TestNestedPermissionsDoNotLeak(t *testing.T) {
	mux := chi.NewRouter()
	reg := apispec.NewRegistry()
	rt := apispec.NewRouter(mux, reg, "/api", apispec.GroupProtected)

	rt.Route("/things", func(r *apispec.Router) {
		r.With(rbac.RequirePermission(nil, permManage)).Post("/gated", noop)
		r.Get("/open", noop, apispec.Exempt("read-only public listing"))
	})

	gated, _ := find(reg, "POST", "/api/things/gated")
	if got := gated.PermissionKeys(); len(got) != 1 || got[0] != string(permManage) {
		t.Errorf("gated route recovered %v", got)
	}

	open, _ := find(reg, "GET", "/api/things/open")
	if got := open.PermissionKeys(); len(got) != 0 {
		t.Errorf("sibling route inherited permissions %v", got)
	}
	if open.Exempt == "" {
		t.Error("exemption reason was not recorded")
	}
}

// TestProbeNeverReachesRequests is the safety property: a probe is a mount-time
// construct, and the middleware it probed must still guard real traffic.
func TestProbeNeverReachesRequests(t *testing.T) {
	_, mux := buildSample(t)

	req := httptest.NewRequest(http.MethodGet, "/api/volumes", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// No authenticated user in context, so the real middleware must reject.
	// A 200 here would mean the probe branch swallowed the gate.
	if rec.Code == http.StatusOK {
		t.Fatalf("unauthenticated request passed the permission gate (status %d)", rec.Code)
	}
}

func TestDocumentIsValidAndCarriesPermissions(t *testing.T) {
	reg, _ := buildSample(t)
	doc := apispec.Document(reg, "2026.09.01")

	if doc["openapi"] != apispec.SpecVersion {
		t.Errorf("openapi version = %v, want %s", doc["openapi"], apispec.SpecVersion)
	}

	// It must survive a JSON round trip; a map with a non-serialisable value
	// would fail here rather than at request time.
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("document does not marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("document does not round trip: %v", err)
	}

	paths, ok := back["paths"].(map[string]any)
	if !ok {
		t.Fatal("document has no paths object")
	}
	item, ok := paths["/api/volumes/{name}"].(map[string]any)
	if !ok {
		t.Fatal("nested path missing from document")
	}
	del, ok := item["delete"].(map[string]any)
	if !ok {
		t.Fatal("delete operation missing from document")
	}
	if _, ok := del["x-mcharbor-permission"]; !ok {
		t.Error("operation does not carry x-mcharbor-permission")
	}

	// The path parameter must be declared, or a generated client cannot
	// build the URL.
	if _, ok := item["parameters"]; !ok {
		t.Error("path parameters missing for templated path")
	}
}

func TestDocumentIsDeterministic(t *testing.T) {
	regA, _ := buildSample(t)
	regB, _ := buildSample(t)

	a, err := json.Marshal(apispec.Document(regA, "2026.09.01"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	b, err := json.Marshal(apispec.Document(regB, "2026.09.01"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(a) != string(b) {
		t.Error("document is not byte-stable across builds; CI could not diff it")
	}
}
