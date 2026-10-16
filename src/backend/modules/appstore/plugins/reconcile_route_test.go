// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func reconcileRequest(h *Handler, slug, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Post("/appstore/plugins/{slug}/reconcile", h.handleReconcile)
	req := httptest.NewRequest(http.MethodPost, "/appstore/plugins/"+slug+"/reconcile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestReconcileRouteKeepsAMatchingInstallAndNamesAMissingVersion(t *testing.T) {
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	svc := NewService(database, nil, "")
	if _, err := svc.InsertInstalledRow(context.Background(), "comm-ntfy", "2026.09.02", "ntfy", "", "", true); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(nil, svc, nil)

	w := reconcileRequest(h, "comm-ntfy", `{"version":"2026.09.02","enabled":false}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"action":"kept"`) {
		t.Fatalf("a matching install: %d %s", w.Code, w.Body.String())
	}
	if enabled, err := svc.LookupEnabled(context.Background(), "comm-ntfy"); err != nil || enabled {
		t.Fatalf("the enabled flag was not applied: %v %v", enabled, err)
	}

	w = reconcileRequest(h, "comm-ntfy", `{"version":"2026.09.09"}`)
	var env struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusConflict || env.Data["slug"] != "comm-ntfy" {
		t.Fatalf("a version the catalog does not carry must fail by name: %d %s", w.Code, w.Body.String())
	}
	if _, leaked := env.Data["detail"]; leaked {
		t.Fatalf("the raw error text must stay in the server log, not the response: %s", w.Body.String())
	}
	if v, err := svc.LookupVersion(context.Background(), "comm-ntfy"); err != nil || v != "2026.09.02" {
		t.Fatalf("a failed reconcile touched the install: %q", v)
	}

	if w := reconcileRequest(h, "comm-ntfy", `{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("no version: %d", w.Code)
	}
}
