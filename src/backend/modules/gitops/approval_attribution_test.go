// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestApprovalIsAttributedToTheCallerNotTheBody guards against an approver
// recording a resolution in someone else's name through the request body.
func TestApprovalIsAttributedToTheCallerNotTheBody(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "gitops.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing the test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO gitops_approvals (id, pipeline_id, stage_id, commit_sha, status) VALUES ('a1', 'p1', 's1', 'abc', 'pending'), ('a2', 'p1', 's1', 'def', 'pending')`); err != nil {
		t.Fatalf("seed approvals: %v", err)
	}

	h := NewHandler(&router.AppDeps{DB: database, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	r := chi.NewRouter()
	r.Post("/approvals/{id}/approve", h.HandleApprove)
	r.Post("/approvals/{id}/reject", h.HandleReject)

	for id, path := range map[string]string{"a1": "/approvals/a1/approve", "a2": "/approvals/a2/reject"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"note":"ok","resolvedBy":"someone-else"}`))
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "approver-1", Username: "approver"}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: HTTP %d %s", path, rec.Code, rec.Body.String())
		}
		var resolvedBy string
		if err := database.QueryRow(`SELECT resolved_by FROM gitops_approvals WHERE id = ?`, id).Scan(&resolvedBy); err != nil {
			t.Fatal(err)
		}
		if resolvedBy != "approver-1" {
			t.Fatalf("%s: resolved_by = %q, want the authenticated caller", path, resolvedBy)
		}
	}
}
