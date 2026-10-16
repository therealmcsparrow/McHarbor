// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package pwa serves the web app manifest and the install icons for the on-call
// surface. Both are generated per request because they follow the instance's
// custom branding, which a static file in the frontend build cannot.
package pwa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

//go:embed fallback-icon.png
var fallbackIcon []byte

// backgroundColor is the dark shell colour the installed app opens on, and the
// fill behind maskable and Apple icons, which platforms require to be opaque.
var backgroundColor = color.RGBA{R: 0x0b, G: 0x0f, B: 0x14, A: 0xff}

// accentColors maps the appearance accent ids to the colour the OS paints the
// app's title bar with; the backend holds no theme colours, only these ids.
var accentColors = map[string]string{
	"theme": "#0ea5b7", "blue": "#3b82f6", "green": "#22c55e", "purple": "#a855f7",
	"orange": "#f97316", "rose": "#f43f5e", "cyan": "#06b6d4",
}

// iconSpec is one icon the manifest or iOS asks for.
type iconSpec struct {
	size    int
	opaque  bool
	content float64
}

var icons = map[string]iconSpec{
	"192.png":              {size: 192, content: 1},
	"512.png":              {size: 512, content: 1},
	"maskable-512.png":     {size: 512, opaque: true, content: 0.8},
	"apple-touch-icon.png": {size: 180, opaque: true, content: 0.86},
}

// Manifest is the web app manifest document.
type Manifest struct {
	Name            string         `json:"name"`
	ShortName       string         `json:"short_name"`
	Description     string         `json:"description"`
	ID              string         `json:"id"`
	StartURL        string         `json:"start_url"`
	Scope           string         `json:"scope"`
	Display         string         `json:"display"`
	Orientation     string         `json:"orientation"`
	ThemeColor      string         `json:"theme_color"`
	BackgroundColor string         `json:"background_color"`
	Icons           []ManifestIcon `json:"icons"`
}

// ManifestIcon is one entry of Manifest.Icons.
type ManifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

// Mount registers the public manifest and icon routes; the browser fetches a
// manifest without credentials, and the branding they expose is already public.
func Mount(app *router.AppDeps) {
	h := &handler{db: app.DB}
	app.RegisterPublicRoutes(func(r *apispec.Router) {
		r.Route("/pwa", func(r *apispec.Router) {
			r.Get("/manifest.webmanifest", h.manifest, apispec.Response(Manifest{}))
			r.Get("/icons/{name}", h.icon, apispec.Binary())
		})
	})
}

type handler struct {
	db *sql.DB
}

func (h *handler) manifest(w http.ResponseWriter, r *http.Request) {
	accent := coreSettings.ReadAppearanceSettingsContext(r.Context(), h.db).Accent
	theme, ok := accentColors[accent]
	if !ok {
		theme = accentColors["theme"]
	}
	m := Manifest{
		Name: "McHarbor", ShortName: "McHarbor", Description: "On-call view of your McHarbor fleet",
		ID: "/oncall", StartURL: "/oncall", Scope: "/", Display: "standalone", Orientation: "portrait",
		ThemeColor: theme, BackgroundColor: hexColor(backgroundColor),
		Icons: []ManifestIcon{
			{Src: "/api/pwa/icons/192.png", Sizes: "192x192", Type: "image/png", Purpose: "any"},
			{Src: "/api/pwa/icons/512.png", Sizes: "512x512", Type: "image/png", Purpose: "any"},
			{Src: "/api/pwa/icons/maskable-512.png", Sizes: "512x512", Type: "image/png", Purpose: "maskable"},
		},
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	if err := json.NewEncoder(w).Encode(m); err != nil {
		return
	}
}

func (h *handler) icon(w http.ResponseWriter, r *http.Request) {
	spec, ok := icons[chi.URLParam(r, "name")]
	if !ok {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	src := brandSource(r.Context(), h.db)
	sum := sha256.Sum256(append(append([]byte{}, src...), byte(spec.size)))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	out, err := render(src, spec)
	if err != nil {
		out, err = render(fallbackIcon, spec)
	}
	if err != nil {
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", etag)
	if _, err := w.Write(out); err != nil {
		return
	}
}

// brandSource returns the uploaded icon's PNG rendition, or the McHarbor mark.
func brandSource(ctx context.Context, database *sql.DB) []byte {
	raw := strings.TrimSpace(coreSettings.ReadBrandingSettingsContext(ctx, database).LogoIconPNG)
	if raw == "" {
		return fallbackIcon
	}
	if i := strings.Index(raw, ","); strings.HasPrefix(raw, "data:") && i > 0 {
		raw = raw[i+1:]
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(b) == 0 {
		return fallbackIcon
	}
	return b
}

// render scales src into a square of spec.size, centred at spec.content of the
// width, over an opaque background where the platform demands one.
func render(src []byte, spec iconSpec) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decoding icon: %w", err)
	}
	canvas := image.NewRGBA(image.Rect(0, 0, spec.size, spec.size))
	if spec.opaque {
		draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: backgroundColor}, image.Point{}, draw.Src)
	}
	b := img.Bounds()
	inner := int(float64(spec.size) * spec.content)
	scale := float64(inner) / float64(max(b.Dx(), b.Dy()))
	w, h := int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)
	offX, offY := (spec.size-w)/2, (spec.size-h)/2
	scaled := scaleBilinear(img, w, h)
	draw.Draw(canvas, image.Rect(offX, offY, offX+w, offY+h), scaled, image.Point{}, draw.Over)
	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		return nil, fmt.Errorf("encoding icon: %w", err)
	}
	return buf.Bytes(), nil
}

// scaleBilinear resizes with the standard library alone, which is enough for a
// launcher icon and keeps an image dependency out of the backend module.
func scaleBilinear(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	sx := float64(b.Dx()) / float64(w)
	sy := float64(b.Dy()) / float64(h)
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*sy - 0.5
		y0 := clampInt(int(fy), 0, b.Dy()-1)
		y1 := clampInt(y0+1, 0, b.Dy()-1)
		ty := fy - float64(y0)
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*sx - 0.5
			x0 := clampInt(int(fx), 0, b.Dx()-1)
			x1 := clampInt(x0+1, 0, b.Dx()-1)
			tx := fx - float64(x0)
			c00 := color.RGBA64Model.Convert(src.At(b.Min.X+x0, b.Min.Y+y0)).(color.RGBA64)
			c10 := color.RGBA64Model.Convert(src.At(b.Min.X+x1, b.Min.Y+y0)).(color.RGBA64)
			c01 := color.RGBA64Model.Convert(src.At(b.Min.X+x0, b.Min.Y+y1)).(color.RGBA64)
			c11 := color.RGBA64Model.Convert(src.At(b.Min.X+x1, b.Min.Y+y1)).(color.RGBA64)
			lerp := func(a, b, c, d uint16) uint16 {
				top := float64(a)*(1-tx) + float64(b)*tx
				bottom := float64(c)*(1-tx) + float64(d)*tx
				return uint16(top*(1-ty) + bottom*ty)
			}
			dst.SetRGBA64(x, y, color.RGBA64{
				R: lerp(c00.R, c10.R, c01.R, c11.R), G: lerp(c00.G, c10.G, c01.G, c11.G),
				B: lerp(c00.B, c10.B, c01.B, c11.B), A: lerp(c00.A, c10.A, c01.A, c11.A),
			})
		}
	}
	return dst
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func hexColor(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
