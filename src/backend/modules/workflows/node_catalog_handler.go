// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HandleListNodeAvailability returns workflow node availability overrides.
func (h *Handler) HandleListNodeAvailability(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	state, err := h.nodeCatalog.List()
	if err != nil {
		h.app.Logger.Error("workflows: list node availability failed", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrWorkflowNodeListFailed)
		return
	}

	if state == nil {
		state = map[string]bool{}
	}

	response.OK(w, state)
}

// HandleUpdateNodeAvailability stores workflow node availability overrides.
func (h *Handler) HandleUpdateNodeAvailability(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	key := chi.URLParam(r, "key")
	if key == "" {
		response.BadRequestCode(w, r, i18n.ErrWorkflowNodeKeyRequired)
		return
	}

	var input UpdateNodeAvailabilityInput
	if err := response.DecodeBody(r, &input); err != nil || input.Enabled == nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if err := h.nodeCatalog.SetEnabled(key, *input.Enabled); err != nil {
		h.app.Logger.Error("workflows: update node availability failed", "key", key, "error", err)
		if strings.Contains(err.Error(), "node key is required") {
			response.BadRequestCode(w, r, i18n.ErrWorkflowNodeKeyRequired)
			return
		}
		if strings.Contains(err.Error(), "invalid node key") {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		response.InternalErrorCode(w, r, i18n.ErrWorkflowNodeUpdateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     map[bool]string{true: "enable", false: "disable"}[*input.Enabled],
		EntityType: "workflow-node",
		EntityID:   key,
	})

	response.OK(w, map[string]any{
		"key":     key,
		"enabled": *input.Enabled,
	})
}
