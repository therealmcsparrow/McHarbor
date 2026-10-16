// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package volumes serves the Docker volume list, create, inspect and remove routes.
package volumes

import (
	"errors"
	cerrdefs "github.com/containerd/errdefs"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// Handler holds dependencies for volume HTTP handlers.
type Handler struct {
	svc *Service
	app *router.AppDeps
}

// NewHandler creates a new volume handler.
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

// HandleList returns all volumes.
// GET /volumes?env=envId
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	volumes, err := h.svc.List(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("list volumes failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrVolumeListFailed, err)
		return
	}

	response.OK(w, volumes)
}

// HandleCreate creates a new volume.
// POST /volumes
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req CreateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrVolumeNameRequired)
		return
	}

	vol, err := h.svc.Create(r.Context(), envID, req)
	if err != nil {
		var ref *selfguard.Refusal
		if errors.As(err, &ref) {
			selfguard.Refuse(w, r, h.app.AuditLog, ref, audit.Entry{
				EntityType: "volume", EntityName: req.Name, EnvironmentID: envID, Details: "op=create",
			})
			return
		}
		h.app.Logger.Error("create volume failed", "env", envID, "name", req.Name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrVolumeCreateFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "create",
		EntityType:    "volume",
		EntityName:    req.Name,
		EnvironmentID: envID,
	})

	response.Created(w, vol)
}

// HandleInspect returns detailed volume information.
// GET /volumes/{name}
func (h *Handler) HandleInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	name := chi.URLParam(r, "name")

	vol, err := h.svc.Inspect(r.Context(), envID, name)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrVolumeNotFound)
			return
		}
		h.app.Logger.Error("inspect volume failed", "env", envID, "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrVolumeInspectFailed, err)
		return
	}

	response.OK(w, vol)
}

// HandleRemove removes a volume.
// DELETE /volumes/{name}?force=true
func (h *Handler) HandleRemove(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	name := chi.URLParam(r, "name")
	force := r.URL.Query().Get("force") == "true"

	if err := h.svc.Remove(r.Context(), envID, name, force); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrVolumeNotFound)
			return
		}
		h.app.Logger.Error("remove volume failed", "env", envID, "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrVolumeRemoveFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "delete",
		EntityType:    "volume",
		EntityID:      name,
		EntityName:    name,
		EnvironmentID: envID,
	})

	response.NoContent(w)
}

// HandlePrune removes unused volumes.
// POST /volumes/prune
func (h *Handler) HandlePrune(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	report, err := h.svc.Prune(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("prune volumes failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrVolumePruneFailed, err)
		return
	}

	response.OK(w, report)
}
