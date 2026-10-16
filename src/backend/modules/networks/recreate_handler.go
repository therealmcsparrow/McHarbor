// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HandleRecreate replaces a network's definition and reattaches its containers on their previous
// addresses, MAC and aliases.
// POST /networks/{id}/recreate?env=envId
func (h *Handler) HandleRecreate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req CreateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrNetworkNameRequired)
		return
	}

	result, err := h.svc.Recreate(r.Context(), envID, id, req)
	if err != nil {
		h.writeRecreateError(w, r, envID, id, err)
		return
	}

	changed := 0
	for _, res := range result.Results {
		if !res.AddressKept {
			changed++
		}
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "recreate",
		EntityType:    "network",
		EntityID:      result.ID,
		EntityName:    req.Name,
		Details:       fmt.Sprintf("previous=%s containers=%d address_changed=%d", id, len(result.Results), changed),
		EnvironmentID: envID,
	})
	response.OK(w, result)
}

func (h *Handler) writeRecreateError(w http.ResponseWriter, r *http.Request, envID, id string, err error) {
	if reason, ok := ipamRefusalReason(err); ok {
		h.app.Logger.Warn("recreate network: invalid ipam", "env", envID, "network", id, "error", err)
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]string{"reason": reason})
		return
	}
	if h.writeSelfRefusal(w, r, envID, id, "", "recreate", err) {
		return
	}
	if h.writeHostLinkError(w, r, i18n.ErrNetworkCreateFailed, "recreate network", envID, err) {
		return
	}
	switch {
	case errors.Is(err, ErrBuiltIn):
		response.BadRequestCause(w, r, i18n.ErrNetworkBuiltIn, err)
	case errors.Is(err, ErrRecreateRestored):
		h.app.Logger.Warn("recreate network: refused, original restored", "env", envID, "network", id, "error", err)
		response.ErrCodeCauseWithData(w, r, http.StatusConflict, i18n.ErrNetworkCreateFailed, err, map[string]string{"reason": "recreate_restored"})
	case errors.Is(err, ErrRecreateLost):
		h.app.Logger.Error("recreate network: original lost", "env", envID, "network", id, "error", err)
		response.ErrCodeCauseWithData(w, r, http.StatusInternalServerError, i18n.ErrNetworkCreateFailed, err, map[string]string{"reason": "recreate_lost"})
	default:
		h.app.Logger.Error("recreate network failed", "env", envID, "network", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrNetworkCreateFailed, err)
	}
}
