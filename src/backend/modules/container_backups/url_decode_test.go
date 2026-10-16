// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestChiURLParam_MultiSegmentMountTarget documents the wire-format
// quirk the operator hit on Nextcloud backups: chi's wildcard route
// preserves the URL-encoded form (`%2Fvar%2Fwww%2Fhtml%2Fdata`)
// because Go's net/http refuses to silently rewrite `%2F` to `/`
// inside path segments (path-traversal guard). MountEntriesForRestore
// must therefore URL-decode the param before matching it against
// `manifest.json`'s selectedMounts entries.
func TestChiURLParam_MultiSegmentMountTarget(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/restore-options/{mountTarget:.*}", func(w http.ResponseWriter, req *http.Request) {
		got := chi.URLParam(req, "mountTarget")
		if _, err := w.Write([]byte(got)); err != nil {
			t.Errorf("writing response: %v", err)
		}
	})

	cases := []struct{ name, path, want string }{
		{"single-slash", "/restore-options/%2Fdata", "%2Fdata"},
		{"two-slash", "/restore-options/%2Fvar%2Fdata", "%2Fvar%2Fdata"},
		{"four-slash", "/restore-options/%2Fvar%2Fwww%2Fhtml%2Fdata", "%2Fvar%2Fwww%2Fhtml%2Fdata"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status %d, want 200", rr.Code)
			}
			if got := rr.Body.String(); got != c.want {
				t.Errorf("URLParam = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDecodeRestoreMountTarget(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"/data", "/data"},
		{"%2Fdata", "/data"},
		{"%2Fvar%2Fwww%2Fhtml%2Fdata", "/var/www/html/data"},
		// %2F is the only encoded char we see in practice (encodeURIComponent
		// covers more, but operators always pass paths starting with `/`).
		{"not-percent-anything", "not-percent-anything"},
		// Malformed escape is returned verbatim rather than panicking.
		{"%ZZ", "%ZZ"},
	}
	for _, c := range cases {
		if got := decodeRestoreMountTarget(c.in); got != c.want {
			t.Errorf("decodeRestoreMountTarget(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
