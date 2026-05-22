// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package images

import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for image HTTP handlers.
type Handler struct {
	svc *Service
	app *router.AppDeps
}

// NewHandler creates a new image handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		svc: NewService(app.DockerPool),
		app: app,
	}
}

// HandleList returns all images.
// GET /images?all=true&env=envId
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	all := r.URL.Query().Get("all") == "true"

	images, err := h.svc.List(r.Context(), envID, all)
	if err != nil {
		h.app.Logger.Error("list images failed", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrImageListFailed)
		return
	}

	response.OK(w, images)
}

// HandlePull pulls an image from a registry.
// POST /images
func (h *Handler) HandlePull(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req PullRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Image == "" {
		response.BadRequestCode(w, r, i18n.ErrImageRefRequired)
		return
	}

	output, err := h.svc.Pull(r.Context(), envID, req)
	if err != nil {
		h.app.Logger.Error("pull image failed", "env", envID, "image", req.Image, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrImagePullFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "pull",
		EntityType:    "image",
		EntityName:    req.Image,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]string{"output": output})
}

// HandleInspect returns detailed image information.
// GET /images/{id}
func (h *Handler) HandleInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	info, err := h.svc.Inspect(r.Context(), envID, id)
	if err != nil {
		h.app.Logger.Error("inspect image failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrImageInspectFailed)
		return
	}

	response.OK(w, info)
}

// HandleRemove removes an image.
// DELETE /images/{id}?force=true&noprune=true
func (h *Handler) HandleRemove(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	force := r.URL.Query().Get("force") == "true"
	noPrune := r.URL.Query().Get("noprune") == "true"

	resp, err := h.svc.Remove(r.Context(), envID, id, force, noPrune)
	if err != nil {
		h.app.Logger.Error("remove image failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrImageRemoveFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "delete",
		EntityType:    "image",
		EntityID:      id,
		EnvironmentID: envID,
	})

	response.OK(w, resp)
}

// HandleTag tags an image with a new repository and tag.
// POST /images/{id}/tag
func (h *Handler) HandleTag(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req TagRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Repo == "" {
		response.BadRequestCode(w, r, i18n.ErrImageRepoRequired)
		return
	}

	if err := h.svc.Tag(r.Context(), envID, id, req); err != nil {
		h.app.Logger.Error("tag image failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrImageTagFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "tag",
		EntityType:    "image",
		EntityID:      id,
		Details:       req.Repo + ":" + req.Tag,
		EnvironmentID: envID,
	})

	response.OKMsg(w, r, i18n.MsgImageTagged)
}

// HandleHistory returns the history of an image.
// GET /images/{id}/history
func (h *Handler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	history, err := h.svc.History(r.Context(), envID, id)
	if err != nil {
		h.app.Logger.Error("image history failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrImageHistoryFailed)
		return
	}

	response.OK(w, history)
}

// HandlePrune removes unused images.
// POST /images/prune
func (h *Handler) HandlePrune(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	report, err := h.svc.Prune(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("prune images failed", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrImagePruneFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "prune",
		EntityType:    "image",
		EnvironmentID: envID,
	})

	response.OK(w, report)
}

// HandleExport streams an image as a tar archive download.
// GET /images/{id}/export
func (h *Handler) HandleExport(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	reader, filename, err := h.svc.Export(r.Context(), envID, id)
	if err != nil {
		h.app.Logger.Error("export image failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrImageExportFailed)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	if _, err := io.Copy(w, reader); err != nil {
		h.app.Logger.Error("streaming image export failed", "env", envID, "id", id, "error", err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "export",
		EntityType:    "image",
		EntityID:      id,
		EnvironmentID: envID,
	})
}

// HandleImport loads an image from an uploaded tar archive.
// POST /images/import
func (h *Handler) HandleImport(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	// Limit upload to 4 GB.
	r.Body = http.MaxBytesReader(w, r.Body, 4<<30)

	file, _, err := r.FormFile("file")
	if err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	defer file.Close()

	output, err := h.svc.Import(r.Context(), envID, file)
	if err != nil {
		h.app.Logger.Error("import image failed", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrImageImportFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "import",
		EntityType:    "image",
		EnvironmentID: envID,
	})

	response.OK(w, map[string]string{"output": output})
}
