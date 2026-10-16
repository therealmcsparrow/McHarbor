// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

func postureMux(t *testing.T) (*chi.Mux, *router.AppDeps) {
	t.Helper()
	app := &router.AppDeps{Config: &config.Config{}, Logger: slog.Default()}
	Mount(app)
	mux := chi.NewRouter()
	mux.Route("/api", func(r chi.Router) { app.MountProtectedRoutes(r) })
	return mux, app
}

func TestSecurityPostureRouteIsGatedOnSettingsView(t *testing.T) {
	mux, app := postureMux(t)
	found := false
	for _, op := range app.Spec.Operations() {
		if op.Path != "/api/settings/security-posture" {
			continue
		}
		found = true
		if op.Method != http.MethodGet {
			t.Errorf("%s %s exists; the posture is read-only", op.Method, op.Path)
		}
		if keys := op.PermissionKeys(); len(keys) != 1 || keys[0] != string(rbac.PermSettingsView) {
			t.Errorf("GET is gated by %v, want settings.view", keys)
		}
		if op.Response == nil {
			t.Error("the route declares no response shape")
		}
	}
	if !found {
		t.Fatal("GET /api/settings/security-posture is not registered")
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings/security-posture", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("an anonymous request answered %d, want 401", rec.Code)
	}
}

func TestSecurityPostureCarriesNoConfiguredValue(t *testing.T) {
	const origins = "https://secret-origin.internal.example"
	RecordPosture(config.PostureFacts{
		AuthDisabled: true, TLSEnabled: true, LocallyIssuedCert: true, HSTSMaxAge: 0,
		AllowedOrigins: origins, PluginRequireSignature: false,
	})
	t.Cleanup(func() { bootPosture.Store(nil) })

	mux, _ := postureMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/settings/security-posture", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "system"}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, leak := range []string{origins, "/app/data", "BEGIN ", ".pem", ".key"} {
		if strings.Contains(body, leak) {
			t.Errorf("payload leaks %q: %s", leak, body)
		}
	}

	var env struct {
		Data struct {
			Findings []map[string]any `json:"findings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Findings) != 4 {
		t.Fatalf("got %d findings, want auth, certificate, HSTS and plugin signature: %s", len(env.Data.Findings), body)
	}
	if env.Data.Findings[0]["envVar"] != "AUTH_DISABLE" || env.Data.Findings[0]["severity"] != PostureSeverityCritical {
		t.Errorf("the disabled-auth finding is not first and critical: %v", env.Data.Findings[0])
	}
	for _, f := range env.Data.Findings {
		for k := range f {
			if k != "envVar" && k != "severity" && k != "message" {
				t.Errorf("finding carries an unexpected field %q: %v", k, f)
			}
		}
	}
}

func TestSecurityPostureAllClearIsAnEmptyList(t *testing.T) {
	got := postureFrom(config.PostureFacts{AllowedOrigins: "https://harbor.example", PluginRequireSignature: true})
	if got.Findings == nil || len(got.Findings) != 0 {
		t.Fatalf("a hardened host reported %v, want an empty list", got.Findings)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"findings":[]}` {
		t.Fatalf("all-clear encodes as %s, want an empty array rather than null", raw)
	}
}
