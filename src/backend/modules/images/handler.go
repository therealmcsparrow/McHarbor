// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package images lists, pulls, inspects, exports and imports Docker images per environment.
package images

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
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

func writeProtectedError(w http.ResponseWriter, r *http.Request, err error) bool {
	if !errors.Is(err, coredocker.ErrProtectedResource) {
		return false
	}
	response.ForbiddenCode(w, r, i18n.ErrProtectedTarget)
	return true
}

func imageIDParam(r *http.Request) string {
	id := chi.URLParam(r, "id")
	decoded, err := url.PathUnescape(id)
	if err != nil {
		return id
	}
	return decoded
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
		response.InternalErrorCause(w, r, i18n.ErrImageListFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.Image == "" {
		response.BadRequestCode(w, r, i18n.ErrImageRefRequired)
		return
	}

	output, err := h.svc.Pull(r.Context(), envID, req)
	if err != nil {
		h.app.Logger.Error("pull image failed", "env", envID, "image", req.Image, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrImagePullFailed, err)
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

// HandleInspect returns detailed image information with credential-looking Config.Env values
// replaced by envredact.Marker; ?reveal=true returns them to a caller holding images.manage
// and is audit-logged.
// GET /images/{id}
func (h *Handler) HandleInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := imageIDParam(r)
	reveal := r.URL.Query().Get("reveal") == "true"
	if reveal && !h.mayRevealEnv(w, r, envID) {
		return
	}

	info, err := h.svc.Inspect(r.Context(), envID, id)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrImageNotFound)
			return
		}
		h.app.Logger.Error("inspect image failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrImageInspectFailed, err)
		return
	}

	withheld := redactImage(h.envPolicy(reveal), &info)
	if reveal {
		h.auditEnvReveal(r, envID, info.ID, imageName(info), "image env values", withheld)
	}
	response.NoStore(w)
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
	id := imageIDParam(r)
	force := r.URL.Query().Get("force") == "true"
	noPrune := r.URL.Query().Get("noprune") == "true"

	resp, err := h.svc.Remove(r.Context(), envID, id, force, noPrune)
	if err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrImageNotFound)
			return
		}
		h.app.Logger.Error("remove image failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrImageRemoveFailed, err)
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
	id := imageIDParam(r)

	var req TagRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.Repo == "" {
		response.BadRequestCode(w, r, i18n.ErrImageRepoRequired)
		return
	}

	if err := h.svc.Tag(r.Context(), envID, id, req); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("tag image failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrImageTagFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "tag",
		EntityType:    "image",
		EntityID:      id,
		EntityName:    req.Repo + ":" + req.Tag,
		Details:       req.Repo + ":" + req.Tag,
		EnvironmentID: envID,
	})

	response.OKMsg(w, r, i18n.MsgImageTagged)
}

// HandleHistory returns the history of an image with the values of credential-looking ENV
// steps and build arguments masked; ?reveal=true follows HandleInspect's rule.
// GET /images/{id}/history
func (h *Handler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := imageIDParam(r)
	reveal := r.URL.Query().Get("reveal") == "true"
	if reveal && !h.mayRevealEnv(w, r, envID) {
		return
	}

	history, err := h.svc.History(r.Context(), envID, id)
	if err != nil {
		h.app.Logger.Error("image history failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrImageHistoryFailed, err)
		return
	}

	history, withheld := redactHistory(h.envPolicy(reveal), history)
	if reveal {
		h.auditEnvReveal(r, envID, id, id, "image history env values", withheld)
	}
	response.NoStore(w)
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
		response.InternalErrorCause(w, r, i18n.ErrImagePruneFailed, err)
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
	id := imageIDParam(r)

	reader, filename, err := h.svc.Export(r.Context(), envID, id)
	if err != nil {
		h.app.Logger.Error("export image failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrImageExportFailed, err)
		return
	}
	defer func() {
		if err := reader.Close(); err != nil {
			h.app.Logger.Debug("closing image export reader failed", "env", envID, "id", id, "error", err)
		}
	}()

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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			h.app.Logger.Debug("closing image import upload failed", "env", envID, "error", err)
		}
	}()

	output, err := h.svc.Import(r.Context(), envID, file)
	if err != nil {
		h.app.Logger.Error("import image failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrImageImportFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "import",
		EntityType:    "image",
		EnvironmentID: envID,
	})

	response.OK(w, map[string]string{"output": output})
}
