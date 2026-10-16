// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pwa

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func newHandler(t *testing.T) (*handler, *sql.DB) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "pwa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &handler{db: database}, database
}

func fetchIcon(t *testing.T, h *handler, name string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/pwa/icons/"+name, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", name)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.icon(rec, req)
	return rec
}

func decode(t *testing.T, body []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	return img
}

func TestManifestOpensTheOnCallSurfaceStandalone(t *testing.T) {
	h, _ := newHandler(t)
	rec := httptest.NewRecorder()
	h.manifest(rec, httptest.NewRequest(http.MethodGet, "/api/pwa/manifest.webmanifest", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "application/manifest+json" {
		t.Fatalf("content type %q", ct)
	}
	var m Manifest
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.StartURL != "/oncall" || m.Display != "standalone" || m.Scope != "/" {
		t.Fatalf("manifest = %+v", m)
	}
	var maskable bool
	for _, i := range m.Icons {
		if i.Purpose == "maskable" {
			maskable = true
		}
		if icons[filepath.Base(i.Src)].size == 0 {
			t.Fatalf("manifest names an icon the server does not serve: %s", i.Src)
		}
	}
	if !maskable {
		t.Fatal("no maskable icon, Android will letterbox the launcher icon")
	}
}

func TestIconsAreServedAtTheirSizesAndPlatformIconsAreOpaque(t *testing.T) {
	h, _ := newHandler(t)
	for name, spec := range icons {
		rec := fetchIcon(t, h, name)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", name, rec.Code)
		}
		img := decode(t, rec.Body.Bytes())
		if img.Bounds().Dx() != spec.size || img.Bounds().Dy() != spec.size {
			t.Fatalf("%s is %v, want %d square", name, img.Bounds(), spec.size)
		}
		_, _, _, a := img.At(0, 0).RGBA()
		if (name == "maskable-512.png" || name == "apple-touch-icon.png") && a != 0xffff {
			t.Fatalf("%s has a transparent corner; iOS and maskable icons render it black", name)
		}
	}
	if rec := fetchIcon(t, h, "../../etc/passwd"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown icon name returned %d", rec.Code)
	}
}

func TestIconsFollowTheUploadedBranding(t *testing.T) {
	h, database := newHandler(t)
	before := fetchIcon(t, h, "512.png")

	red := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			red.Set(x, y, color.RGBA{R: 0xff, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, red); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO settings (id, key, value, category) VALUES ('b1', 'branding_logo_icon_png', ?, 'branding')`,
		base64.StdEncoding.EncodeToString(buf.Bytes())); err != nil {
		t.Fatalf("seed branding: %v", err)
	}
	after := fetchIcon(t, h, "512.png")
	if after.Header().Get("ETag") == before.Header().Get("ETag") {
		t.Fatal("the ETag did not change with the branding, installed apps keep the old icon")
	}
	r, g, _, _ := decode(t, after.Body.Bytes()).At(256, 256).RGBA()
	if r < 0xf000 || g > 0x1000 {
		t.Fatal("the icon did not use the uploaded branding")
	}
	cached := fetchIconWith(t, h, "512.png", after.Header().Get("ETag"))
	if cached.Code != http.StatusNotModified {
		t.Fatalf("a matching If-None-Match returned %d", cached.Code)
	}
}

func fetchIconWith(t *testing.T, h *handler, name, etag string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/pwa/icons/"+name, nil)
	req.Header.Set("If-None-Match", etag)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", name)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.icon(rec, req)
	return rec
}
