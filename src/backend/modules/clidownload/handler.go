// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package clidownload

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Local and image directories, relative to the working directory like
// agent-local and agent-bin; the local folder is a bind mount of ./cli.
const (
	localDir = "cli-local"
	imageDir = "cli-bin"
)

// Handler serves the client archives.
type Handler struct {
	svc *Service
}

// Mount registers the routes. They need a session but no permission key: the
// archives are the same bytes for every user, and nothing in them is secret.
func Mount(app *router.AppDeps) {
	h := &Handler{svc: NewService(localDir, imageDir)}
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/cli", func(r *apispec.Router) {
			r.Get("/downloads", h.HandleList, apispec.Response([]Archive{}))
			r.Get("/download/{os}/{arch}", h.HandleDownload, apispec.Binary())
		})
	})
}

// HandleList returns the archives available for download.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	archives := h.svc.List()
	if archives == nil {
		archives = []Archive{}
	}
	response.OK(w, archives)
}

// HandleDownload streams one archive under its own file name.
func (h *Handler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	archive, err := h.svc.Find(chi.URLParam(r, "os"), chi.URLParam(r, "arch"))
	if errors.Is(err, ErrNotFound) {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	contentType := "application/gzip"
	if archive.OS == "windows" {
		contentType = "application/zip"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+archive.File+`"`)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, archive.Path())
}
