// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestNarrationRoutesStateTheirRefusal checks the API says why nothing was
// sent, and that opting an environment in is audited as a warning.
func TestNarrationRoutesStateTheirRefusal(t *testing.T) {
	f := newFixture(t)
	in := openCrashLoop(t, f)
	app := &router.AppDeps{DB: f.db, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AuditLog: coreAudit.NewLogger(f.db, "s")}
	h := &Handler{app: app, svc: f.svc}
	mux := chi.NewRouter()
	mux.Post("/incidents/{id}/narrate", h.HandleNarrate)
	mux.Put("/incidents/settings", h.HandleSaveSettings)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	if w := call(http.MethodPost, "/incidents/"+in.ID+"/narrate", ""); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "not_opted_in") {
		t.Fatalf("narrate before opt-in: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodPut, "/incidents/settings", `{"envId":"e1","narrateOptIn":true,"proactive":false,"hourlyCap":0}`); w.Code != http.StatusBadRequest {
		t.Fatalf("a zero hourly cap was accepted: %d", w.Code)
	}
	w := call(http.MethodPut, "/incidents/settings", `{"envId":"e1","narrateOptIn":true,"proactive":true,"hourlyCap":5}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"modelConfigured":true`) || !strings.Contains(w.Body.String(), `"provider":"fake"`) {
		t.Fatalf("opt-in: %d %s", w.Code, w.Body.String())
	}
	var severity string
	if err := f.db.QueryRow(`SELECT severity FROM audit_logs WHERE action = 'incident.settings.updated'`).Scan(&severity); err != nil || severity != "warning" {
		t.Fatalf("opt-in audit severity = %q %v", severity, err)
	}
	f.model.none = true
	if w := call(http.MethodPost, "/incidents/"+in.ID+"/narrate", ""); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "no_model") {
		t.Fatalf("narrate without a model: %d %s", w.Code, w.Body.String())
	}
	st, err := f.svc.Store.SaveSettings(t.Context(), Settings{EnvID: "e1", NarrateOptIn: false, Proactive: true, HourlyCap: 5}, "admin")
	if err != nil || st.Proactive || st.OptedInBy != "" {
		t.Fatalf("opting out must also turn proactive off and forget who opted in: %+v %v", st, err)
	}
}
