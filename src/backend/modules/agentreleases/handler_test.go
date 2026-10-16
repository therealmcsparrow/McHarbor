// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

func testHandler(t *testing.T, store *Store) (*Handler, http.Handler) {
	t.Helper()
	app := &router.AppDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if store != nil {
		app.DB = store.db
		app.AuditLog = audit.NewLogger(store.db, "test")
	}
	h := NewHandler(app, store)
	mux := chi.NewRouter()
	mux.Get("/releases", h.HandleList)
	mux.Delete("/releases/{version}", h.HandleDelete)
	mux.Put("/releases/target", h.HandleSetTarget)
	mux.Put("/releases/settings", h.HandleSaveSettings)
	return h, mux
}

func asUser(req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "operator"}))
}

func TestReleaseRoutesRefuseWithoutAUser(t *testing.T) {
	_, mux := testHandler(t, nil)
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/releases", nil),
		httptest.NewRequest(http.MethodDelete, "/releases/2026.09.02", nil),
		httptest.NewRequest(http.MethodPut, "/releases/target", strings.NewReader(`{"version":"2026.09.02"}`)),
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status %d, want 401", req.Method, req.URL.Path, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, asUser(httptest.NewRequest(http.MethodGet, "/releases", nil)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no store: status %d, want 503", rec.Code)
	}
}

func TestDeleteTargetIsAConflictWithTheReasonCode(t *testing.T) {
	pub, priv := newKey(t)
	rs := newReleaseServer(t)
	rs.publishMirror("/m", "2026.09.02", buildRelease(t, priv, "2026.09.02"))
	s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/m"}}, storeOpts{})
	if _, err := s.Pull(t.Context(), "2026.09.02", "", "t"); err != nil {
		t.Fatal(err)
	}
	_, mux := testHandler(t, s)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, asUser(httptest.NewRequest(http.MethodPut, "/releases/target", strings.NewReader(`{"version":"2026.09.02"}`))))
	if rec.Code != http.StatusOK {
		t.Fatalf("set target: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, asUser(httptest.NewRequest(http.MethodDelete, "/releases/2026.09.02", nil)))
	var body struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding delete response: %v", err)
	}
	if rec.Code != http.StatusConflict || body.Code != "err.agent_release.is_target" {
		t.Fatalf("delete target: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, asUser(httptest.NewRequest(http.MethodPut, "/releases/settings", strings.NewReader(`{"mode":"notify","checkCron":"nope","sources":[{"kind":"mirror","url":"https://m.example"}]}`))))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"bad_cron"`) || strings.Contains(rec.Body.String(), "invalid agent release settings") {
		t.Fatalf("bad settings must come back as a reason code, not error text: %d %s", rec.Code, rec.Body.String())
	}
}
