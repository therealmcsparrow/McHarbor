// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package openapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

type docsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	} `json:"data"`
}

func serveDocs(t *testing.T, h *Handler, target string) docsResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	h.HandleDocs(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", target, rec.Code, rec.Body.String())
	}
	var resp docsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode docs: %v", err)
	}
	return resp
}

func appWithRoute(t *testing.T) *router.AppDeps {
	t.Helper()
	app := &router.AppDeps{Spec: apispec.NewRegistry()}
	rt := apispec.NewRouter(chi.NewRouter(), app.Spec, "/api", apispec.GroupProtected)
	rt.Get("/things", func(w http.ResponseWriter, r *http.Request) {})
	return app
}

// TestMountRegistersDocsRoute proves the docs route is served and recorded in
// the registry it documents, like every other route.
func TestMountRegistersDocsRoute(t *testing.T) {
	app := &router.AppDeps{}
	Mount(app)

	r := chi.NewRouter()
	app.MountProtectedRoutes(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("docs route status = %d", rec.Code)
	}
	found := false
	for _, op := range app.Spec.Operations() {
		if op.Method == http.MethodGet && op.Path == "/api/docs" {
			found = true
		}
	}
	if !found {
		t.Fatalf("GET /api/docs is not in the registry: %+v", app.Spec.Operations())
	}
}

// TestHandleDocsServesTheRecordedRoutesWithoutADatabase covers an instance
// with no database wired: the core document must still render, not panic.
func TestHandleDocsServesTheRecordedRoutesWithoutADatabase(t *testing.T) {
	resp := serveDocs(t, NewHandler(appWithRoute(t)), "/docs/")
	if !resp.Success || resp.Data.OpenAPI != apispec.SpecVersion {
		t.Fatalf("docs = %+v, want success and openapi %s", resp, apispec.SpecVersion)
	}
	if _, ok := resp.Data.Paths["/api/things"]; !ok {
		t.Fatalf("recorded route missing from the document: %v", resp.Data.Paths)
	}
}

// TestHandleDocsMergesPluginRoutesUnlessCoreIsRequested proves the live
// document carries installed plugin routes while ?source=core stays core-only.
func TestHandleDocsMergesPluginRoutesUnlessCoreIsRequested(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "docs.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing the test database: %v", err)
		}
	})
	if _, err := database.Exec(`
CREATE TABLE plugin_routes (id TEXT, origin TEXT, method TEXT, path TEXT, requires_permission TEXT, is_public INTEGER);
CREATE TABLE plugin_permissions (origin TEXT, permission_key TEXT, scope TEXT);
INSERT INTO plugin_routes VALUES ('1', 'addon-kvm', 'GET', '/api/vms', 'vm.view', 0);`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	app := appWithRoute(t)
	app.DB = database
	h := NewHandler(app)

	live := serveDocs(t, h, "/docs/")
	if _, ok := live.Data.Paths["/api/vms"]; !ok {
		t.Fatalf("live document lacks the plugin route: %v", live.Data.Paths)
	}
	core := serveDocs(t, h, "/docs/?source=core")
	if _, ok := core.Data.Paths["/api/vms"]; ok {
		t.Fatal("?source=core must not include plugin routes")
	}
	if _, ok := core.Data.Paths["/api/things"]; !ok {
		t.Fatal("?source=core lost the core route")
	}
}
