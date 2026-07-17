// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package versions

import (
	"errors"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for version handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a version handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB, app.AgentPool),
	}
}

// HandleInfo returns McHarbor and agent version information.
func (h *Handler) HandleInfo(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	info, err := h.service.Info()
	if err != nil {
		h.app.Logger.Error("failed to get version info", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, info)
}

// HandleSelfUpdate triggers a McHarbor self-update by scheduling the
// detached helper container (see core/docker.ScheduleDetachedSelfUpdateHelperForImage).
// The current process exits shortly after the helper starts; the operator
// reconnects once the new image is up. Restricted to the system.manage
// permission so only admins can trigger a self-update.
//
// The optional `image` field in the request body overrides the target
// image; when empty the existing image is reused.
func (h *Handler) HandleSelfUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var req SelfUpdateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if h.app.Config != nil && h.app.Config.AuthDisable && user.ID == "system" {
		// Synthetic admin in AUTH_DISABLE mode: the per-request RBAC
		// middleware already accepted us via the wildcard path; nothing
		// more to check here.
	}

	result, err := h.service.SelfUpdate(r.Context(), req.Image)
	if err != nil {
		switch {
		case errors.Is(err, errSelfUpdateNotLocal):
			response.BadRequestCode(w, r, i18n.ErrSystemSelfUpdateNotLocal)
		default:
			h.app.Logger.Error("versions: self-update failed", "error", err)
			response.BadRequestCode(w, r, i18n.ErrSystemSelfUpdateFailed)
		}
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "self_update",
		EntityType: "mcharbor",
		EntityID:   result.ContainerID,
		EntityName: result.ContainerName,
		Details:    "McHarbor self-update scheduled by " + user.Username,
	})

	response.OK(w, struct {
		Message string          `json:"message"`
		Code    string          `json:"code"`
		Result  SelfUpdateResult `json:"result"`
	}{
		Message: i18n.T(i18n.FromRequest(r), i18n.MsgSystemSelfUpdateScheduled),
		Code:    string(i18n.MsgSystemSelfUpdateScheduled),
		Result:  result,
	})
}
