// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package debugshell

import (
	"errors"
	"net/http"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler serves /containers/{id}/debug-shell.
type Handler struct {
	svc      *Service
	auditLog *audit.Logger
	app      *router.AppDeps
}

// NewHandler binds the routes to the process-wide Service.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{svc: Shared(app), auditLog: app.AuditLog, app: app}
}

// HandleInstall uploads busybox into a shell-less container and returns the
// session the terminal WebSocket attaches with. The install is the audited act.
func (h *Handler) HandleInstall(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrContainerNotFound)
		return
	}
	envID := response.ParseEnvID(r)
	if !h.guardSelf(w, r, envID, id) {
		return
	}
	req := InstallRequest{EnvironmentID: envID, ContainerID: id, UserID: user.ID}
	if key := rbac.APIKeyFromContext(r.Context()); key != nil {
		req.APIKeyID = key.KeyID
	}

	sess, err := h.svc.Install(r.Context(), req)
	if err != nil {
		h.writeInstallError(w, r, envID, id, err)
		return
	}
	if h.auditLog != nil {
		h.auditLog.Log(r, InstallAuditEntry(sess))
	}
	response.Created(w, sess)
}

func (h *Handler) writeInstallError(w http.ResponseWriter, r *http.Request, envID, id string, err error) {
	switch {
	case errors.Is(err, ErrHasShell):
		response.ConflictCode(w, r, i18n.ErrContainerHasShell)
	case errors.Is(err, ErrReadonlyRootfs):
		response.ConflictCode(w, r, i18n.ErrContainerReadonlyRootfs)
	case errors.Is(err, ErrNotRunning):
		response.ConflictCode(w, r, i18n.ErrContainerNotRunning)
	case errors.Is(err, ErrProtected):
		response.ForbiddenCode(w, r, i18n.ErrProtectedTarget)
	case errors.Is(err, ErrUnsupportedArch):
		h.logger().Warn("debug shell: unsupported daemon architecture", "container", id, "env", envID, "error", err)
		response.BadRequestCause(w, r, i18n.ErrDebugShellUnsupportedArch, err)
	case errors.Is(err, ErrBinaryUnavailable):
		h.logger().Error("debug shell: bundled busybox missing", "container", id, "env", envID, "error", err)
		response.ServiceUnavailableCause(w, r, i18n.ErrDebugShellUnavailable, err)
	case errdefs.IsNotFound(err):
		response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
	default:
		h.logger().Error("debug shell: install failed", "container", id, "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrDebugShellInstallFailed, err)
	}
}

// HandleStatus lists the container's open debug sessions and whether any of them
// left a busybox behind, gated on containers.view because that is who must see it.
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	status, err := h.svc.Status(r.Context(), envID, id)
	if err != nil {
		h.logger().Error("debug shell: status failed", "container", id, "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrDebugShellStatusFailed, err)
		return
	}
	response.OK(w, status)
}

// HandleRemove closes the container's debug sessions now, or only ?session=<id>.
func (h *Handler) HandleRemove(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	removed, err := h.svc.Close(r.Context(), envID, id, strings.TrimSpace(r.URL.Query().Get("session")))
	if err != nil {
		h.logger().Error("debug shell: removal failed", "container", id, "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrDebugShellRemoveFailed, err)
		return
	}
	response.OK(w, RemoveResult{Removed: removed})
}

func (h *Handler) logger() interface {
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
} {
	if h.app != nil && h.app.Logger != nil {
		return h.app.Logger
	}
	return h.svc.logger
}
