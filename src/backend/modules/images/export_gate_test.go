// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package images

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// TestImageExportNeedsManage goes through the mounted routes: an export is the whole `docker
// save` archive, unmasked Config.Env included, so images.view is refused and images.manage
// gets the stream.
func TestImageExportNeedsManage(t *testing.T) {
	app, _ := newImageApp(t)
	Mount(app)
	mux := chi.NewRouter()
	mux.Route("/api", func(r chi.Router) { app.MountProtectedRoutes(r) })

	export := func(userID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/images/app:1/export?env=envA", nil)
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := export("viewer"); rec.Code != http.StatusForbidden {
		t.Fatalf("export as images.view: HTTP %d, want 403; body %q", rec.Code, rec.Body.String())
	}
	rec := export("ops")
	if rec.Code != http.StatusOK {
		t.Fatalf("export as images.manage: HTTP %d, want 200; body %q", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != fakeImageTar {
		t.Fatalf("export body = %q, want the daemon's tar stream", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/x-tar" {
		t.Errorf("Content-Type = %q, want application/x-tar", got)
	}
}
