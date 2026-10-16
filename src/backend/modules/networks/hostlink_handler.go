// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"errors"
	"net/http"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// writeHostLinkError answers a host link failure and reports whether err was one. The reason key
// travels in data.reason, on err.invalid_body for a refused request and on failed for an execution
// failure, because core/i18n is linked into every plugin and gets no new code for this.
func (h *Handler) writeHostLinkError(w http.ResponseWriter, r *http.Request, failed i18n.MsgCode, op, envID string, err error) bool {
	if reason, ok := hostLinkRefusal(err); ok {
		h.app.Logger.Warn(op+": host link refused", "env", envID, "error", err)
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]string{"reason": reason})
		return true
	}
	var unavailable *HostLinkUnavailableError
	switch {
	case errors.As(err, &unavailable):
		h.app.Logger.Warn(op+": host link unavailable", "env", envID, "reason", unavailable.Reason)
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody,
			map[string]string{"reason": "host_link_unavailable", "unavailable": unavailable.Reason})
	case errors.Is(err, ErrHostLinkNameTaken):
		h.app.Logger.Warn(op+": host link name taken", "env", envID, "error", err)
		response.ErrCodeCauseWithData(w, r, http.StatusConflict, failed, err, map[string]string{"reason": "host_link_name_taken"})
	case errors.Is(err, ErrHostLinkFailed):
		h.app.Logger.Error(op+": host link failed", "env", envID, "error", err)
		response.ErrCodeCauseWithData(w, r, http.StatusInternalServerError, failed, err, map[string]string{"reason": "host_link_failed"})
	default:
		return false
	}
	return true
}

// HandleHostLink reports the state of a network's host interface.
// GET /networks/{id}/host-link?env=envId
func (h *Handler) HandleHostLink(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	status, err := h.svc.HostLinkStatus(r.Context(), envID, id)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrNetworkNotFound)
			return
		}
		h.app.Logger.Error("host link status failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrNetworkInspectFailed, err)
		return
	}
	response.OK(w, status)
}

// HandleHostLinkApply re-creates a network's host interface from its labels.
// POST /networks/{id}/host-link/apply?env=envId
func (h *Handler) HandleHostLinkApply(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	status, err := h.svc.ReapplyHostLink(r.Context(), envID, id)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrNetworkNotFound)
			return
		}
		if h.writeHostLinkError(w, r, i18n.ErrNetworkCreateFailed, "reapply host link", envID, err) {
			return
		}
		h.app.Logger.Error("reapply host link failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrNetworkInspectFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "host_link_apply",
		EntityType:    "network",
		EntityID:      id,
		Details:       "link=" + status.HostLink.Name,
		EnvironmentID: envID,
	})

	response.OK(w, status)
}
