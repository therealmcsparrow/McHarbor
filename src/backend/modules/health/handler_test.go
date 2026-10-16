// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/clusterinfo"
	"github.com/therealmcsparrow/mcharbor/core/router"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

func TestMountRegistersHealthRoutes(t *testing.T) {
	app := &router.AppDeps{}
	Mount(app)

	r := chi.NewRouter()
	app.MountPublicRoutes(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/about", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("about status = %d", rec.Code)
	}
}

func TestHandleHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	handleHealth(rec, httptest.NewRequest(http.MethodGet, "/health", nil), "instance-1")

	var resp struct {
		Success bool              `json:"success"`
		Data    map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if !resp.Success || resp.Data["status"] != "ok" || resp.Data["instanceId"] != "instance-1" {
		t.Fatalf("health response = %#v", resp)
	}
}

func TestHandleAbout(t *testing.T) {
	previous := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Deps: []*debug.Module{
				{Path: "github.com/go-chi/cors", Version: "v1.2.1"},
				{Path: "gotest.example/ignored", Version: "v1.0.0"},
				{Path: "github.com/moby/moby/client", Version: "v0.6.0"},
			},
		}, true
	}
	t.Cleanup(func() { readBuildInfo = previous })

	rec := httptest.NewRecorder()
	handleAbout(rec, httptest.NewRequest(http.MethodGet, "/about", nil))

	var resp struct {
		Success bool          `json:"success"`
		Data    aboutResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode about response: %v", err)
	}
	if !resp.Success {
		t.Fatal("about response should be successful")
	}
	if resp.Data.Version != appversion.Current() || resp.Data.GoVersion == "" || resp.Data.Platform == "" {
		t.Fatalf("about response missing metadata: %#v", resp.Data)
	}
	if resp.Data.Dependencies == nil {
		t.Fatal("dependencies should be an empty slice, not nil")
	}
	if len(resp.Data.Dependencies) != 2 {
		t.Fatalf("dependency count = %d, want 2", len(resp.Data.Dependencies))
	}
	if resp.Data.Dependencies[1].Name != "docker-sdk" || resp.Data.Dependencies[1].Version != "v0.6.0" {
		t.Fatalf("docker dependency not normalized: %#v", resp.Data.Dependencies)
	}
}

func TestHandleAboutWithoutBuildInfoUsesEmptyDependencySlice(t *testing.T) {
	previous := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) { return nil, false }
	t.Cleanup(func() { readBuildInfo = previous })

	rec := httptest.NewRecorder()
	handleAbout(rec, httptest.NewRequest(http.MethodGet, "/about", nil))

	var resp struct {
		Data aboutResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode about response: %v", err)
	}
	if resp.Data.Dependencies == nil || len(resp.Data.Dependencies) != 0 {
		t.Fatalf("dependencies = %#v, want empty slice", resp.Data.Dependencies)
	}
}

type fakeReady struct{ ok bool }

func (f fakeReady) Ready(context.Context) (bool, []clusterinfo.ReadyCheck) {
	return f.ok, []clusterinfo.ReadyCheck{{Name: "database", OK: f.ok}}
}
func (fakeReady) Membership() clusterinfo.Membership { return nil }
func (f fakeReady) Info() clusterinfo.Readiness      { return f }

// TestReadyzIs503UntilReady guards the load-balancer contract: livez stays 200 while
// readyz turns 503 for a node that must be kept out of rotation.
func TestReadyzIs503UntilReady(t *testing.T) {
	for _, tc := range []struct {
		svc  any
		want int
	}{{nil, http.StatusServiceUnavailable}, {fakeReady{false}, http.StatusServiceUnavailable}, {fakeReady{true}, http.StatusOK}} {
		app := &router.AppDeps{}
		if tc.svc != nil {
			app.RegisterService("cluster", tc.svc)
		}
		Mount(app)
		r := chi.NewRouter()
		app.MountPublicRoutes(r)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != tc.want {
			t.Fatalf("readyz with %v = %d, want %d", tc.svc, rec.Code, tc.want)
		}
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("livez = %d", rec.Code)
		}
	}
}

func TestHandleAboutReportsHowMcHarborRuns(t *testing.T) {
	t.Cleanup(func() { SetNativeHost(false) })
	for _, native := range []bool{false, true} {
		SetNativeHost(native)
		rec := httptest.NewRecorder()
		handleAbout(rec, httptest.NewRequest(http.MethodGet, "/about", nil))
		var resp struct {
			Data aboutResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode about response: %v", err)
		}
		if resp.Data.Native != native {
			t.Fatalf("native = %v, want %v", resp.Data.Native, native)
		}
	}
}
