// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// handleCatalogIcon serves the icon a remote catalog publishes for a plugin that is not
// installed, so store cards stop falling back to a puzzle piece. The browser never contacts
// the catalog host; the URL comes only from an entry an operator-configured catalog listed.
func (h *Handler) handleCatalogIcon(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if !iconSlugRE.MatchString(slug) {
		response.BadRequestCode(w, r, i18n.ErrPluginSlugRequired)
		return
	}
	variant := ""
	if r.URL.Query().Get("variant") == "dark" {
		variant = "dark"
	}
	cacheDir := ""
	if h.dataDir != "" {
		cacheDir = filepath.Join(h.dataDir, "cache", "plugin-icons")
	}
	file, err := h.svc.RemoteIcon(r.Context(), cacheDir, slug, variant)
	if err != nil {
		if !errors.Is(err, ErrIconNotFound) {
			slog.Warn("plugin catalog icon unavailable", "slug", slug, "error", err)
		}
		response.NotFoundCause(w, r, i18n.ErrNotFound, err)
		return
	}
	f, err := os.Open(file)
	if err != nil {
		slog.Warn("plugin catalog icon: opening cached copy", "slug", slug, "error", err)
		response.NotFoundCause(w, r, i18n.ErrNotFound, err)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("plugin catalog icon: closing cached copy", "error", err)
		}
	}()
	st, err := f.Stat()
	if err != nil {
		slog.Warn("plugin catalog icon: stat cached copy", "slug", slug, "error", err)
		response.NotFoundCause(w, r, i18n.ErrNotFound, err)
		return
	}
	contentType := "image/svg+xml; charset=utf-8"
	if strings.HasSuffix(strings.ToLower(file), ".png") {
		contentType = "image/png"
	}
	// The SVG is third-party markup served from this origin; the sandbox keeps any script in
	// it inert even if someone opens the URL directly instead of through an <img>.
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}
