// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package router

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/config"
)

func TestNewAppDeps(t *testing.T) {
	cfg := &config.Config{Port: 5474}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	app := NewAppDeps(cfg, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	if app.Config != cfg || app.Logger != logger {
		t.Fatalf("NewAppDeps did not assign core fields: %#v", app)
	}
	if app.StaticDir != "./static" {
		t.Fatalf("StaticDir = %q, want ./static", app.StaticDir)
	}
}

func TestRegisterAndMountRoutes(t *testing.T) {
	app := &AppDeps{}
	authCalled := false
	protectedCalled := false

	app.RegisterAuthRoutes(func(r *apispec.Router) {
		authCalled = true
		r.Get("/login", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	})
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		protectedCalled = true
		r.Get("/containers", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	})

	r := chi.NewRouter()
	app.MountAuthRoutes(r)
	app.MountProtectedRoutes(r)

	if app.Spec.Len() != 2 {
		t.Fatalf("spec recorded %d operations, want the 2 mounted routes", app.Spec.Len())
	}
	if !authCalled || !protectedCalled {
		t.Fatalf("mount callbacks not called: auth=%v protected=%v", authCalled, protectedCalled)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("auth route status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/containers", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("protected route status = %d", rec.Code)
	}
}

func TestNewRouterPublicAndAuthRoute(t *testing.T) {
	app := minimalAppDeps(t)
	app.RegisterPublicRoutes(func(r *apispec.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	})
	app.RegisterAuthRoutes(func(r *apispec.Router) {
		r.Get("/login", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) })
	})

	r := New(app)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("public route status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/login", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("auth route status = %d", rec.Code)
	}
}

func TestNewRouterUsesDefaultStaticDirWhenEmpty(t *testing.T) {
	app := minimalAppDeps(t)
	app.StaticDir = ""

	r := New(app)
	if r == nil {
		t.Fatal("New returned nil router")
	}
}

func TestNewRouterStaticDirFallback(t *testing.T) {
	app := minimalAppDeps(t)
	app.StaticDir = makeStaticDir(t)
	r := New(app)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log('ok');" {
		t.Fatalf("asset response = %d %q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/unknown/path", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "<div id=\"root\"></div>" {
		t.Fatalf("spa fallback response = %d %q", rec.Code, rec.Body.String())
	}
}

func TestBuildAllowedOrigins(t *testing.T) {
	app := minimalAppDeps(t)
	app.Config.AllowedOrigins = "https://one.test, https://two.test"

	got := buildAllowedOrigins(app)
	if len(got) != 2 || got[0] != "https://one.test" || got[1] != "https://two.test" {
		t.Fatalf("buildAllowedOrigins = %#v", got)
	}
}

func TestFileServerRedirectsMissingSlash(t *testing.T) {
	r := chi.NewRouter()
	fileServer(r, "/static", makeStaticDir(t))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static", nil))

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMovedPermanently)
	}
	if location := rec.Header().Get("Location"); location != "/static/" {
		t.Fatalf("Location = %q, want /static/", location)
	}
}

func minimalAppDeps(t *testing.T) *AppDeps {
	t.Helper()

	return &AppDeps{
		Config:    &config.Config{Port: 5474},
		Logger:    slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		StaticDir: filepath.Join(t.TempDir(), "missing"),
	}
}

func makeStaticDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatalf("create assets dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<div id=\"root\"></div>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log('ok');"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	return dir
}
