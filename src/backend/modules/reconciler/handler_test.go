// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestIntentEditsRespectEnforceAndGit checks enforce cannot be set through a
// plain edit, and a Git-owned row accepts a mode change and nothing else.
func TestIntentEditsRespectEnforceAndGit(t *testing.T) {
	loop, database, _, _ := newLoopFixture(t)
	r, _ := withResponder(t, loop, database)
	h := NewHandler(&router.AppDeps{DB: database, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, r)
	mux := chi.NewRouter()
	mux.Put("/reconciler/{id}", h.HandleUpdate)
	mux.Delete("/reconciler/{id}", h.HandleDelete)
	mux.Post("/reconciler", h.HandleCreate)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	if w := call(http.MethodPut, "/reconciler/ds-sync", `{"mode":"enforce"}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "enforce_route") {
		t.Fatalf("enforce through PUT: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodPost, "/reconciler", `{"name":"x","containerName":"x","imageRef":"x:1","mode":"enforce"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("enforce through POST: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodPut, "/reconciler/ds-sync", `{"mode":"propose"}`); w.Code != http.StatusOK {
		t.Fatalf("propose through PUT: %d %s", w.Code, w.Body.String())
	}

	if _, err := database.Exec(`INSERT INTO desired_state_sources (id, name, repo_id, env_id) VALUES ('src1', 's', 'r', 'env-far')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE desired_states SET source_id = 'src1', source_path = 'a.yaml#0' WHERE id = 'ds-drift'`); err != nil {
		t.Fatal(err)
	}
	if w := call(http.MethodPut, "/reconciler/ds-drift", `{"imageRef":"nginx:9"}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "git_managed") {
		t.Fatalf("editing a Git row: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodPut, "/reconciler/ds-drift", `{"mode":"propose"}`); w.Code != http.StatusOK {
		t.Fatalf("a mode change on a Git row: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodDelete, "/reconciler/ds-drift", ``); w.Code != http.StatusConflict {
		t.Fatalf("deleting a Git row: %d %s", w.Code, w.Body.String())
	}
}
