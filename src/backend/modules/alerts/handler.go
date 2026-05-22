// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coreauth "github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Alert represents an alert rule.
type Alert struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Severity  string `json:"severity"`
	Type      string `json:"type"`      // cpu, memory, disk, container_down, image_update
	Condition string `json:"condition"` // threshold expression, e.g. "> 80%"
	Target    string `json:"target"`    // container name/pattern, or "*" for all
	ChannelID string `json:"channelId"` // notification channel to fire
	SendInApp bool   `json:"sendInApp"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// CreateAlertInput is the request body for creating an alert rule.
type CreateAlertInput struct {
	Name      string `json:"name"`
	Severity  string `json:"severity"`
	Type      string `json:"type"`
	Condition string `json:"condition"`
	Target    string `json:"target"`
	ChannelID string `json:"channelId"`
	SendInApp bool   `json:"sendInApp"`
}

// UpdateAlertInput is the request body for updating an alert rule.
type UpdateAlertInput struct {
	Name      *string `json:"name"`
	Severity  *string `json:"severity"`
	Type      *string `json:"type"`
	Condition *string `json:"condition"`
	Target    *string `json:"target"`
	ChannelID *string `json:"channelId"`
	SendInApp *bool   `json:"sendInApp"`
	Enabled   *bool   `json:"enabled"`
}

var validAlertSeverities = map[string]bool{
	"critical": true,
	"warning":  true,
	"info":     true,
}

// Handler holds dependencies for alert HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new alerts handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB)
	return &Handler{app: app, service: svc}
}

// HandleList returns a paginated list of alert rules.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(r.Context(), page, perPage)
	if err != nil {
		h.app.Logger.Error("alerts: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrAlertListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleCreate creates a new alert rule.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var input CreateAlertInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrAlertNameRequired)
		return
	}
	if input.Type == "" {
		response.BadRequestCode(w, r, i18n.ErrAlertTypeRequired)
		return
	}
	if input.Severity != "" && !validAlertSeverities[input.Severity] {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	alert, err := h.service.Create(r.Context(), input)
	if err != nil {
		if errors.Is(err, errAlertDestinationRequired) {
			response.BadRequestCode(w, r, i18n.ErrAlertDestinationRequired)
			return
		}
		h.app.Logger.Error("alerts: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrAlertCreateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "alert.created",
		EntityType: "alert",
		EntityID:   alert.ID,
		EntityName: alert.Name,
	})

	response.Created(w, alert)
}

// HandleUpdate updates an alert rule.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")

	var input UpdateAlertInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if input.Severity != nil && *input.Severity != "" && !validAlertSeverities[*input.Severity] {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	alert, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, errAlertDestinationRequired) {
			response.BadRequestCode(w, r, i18n.ErrAlertDestinationRequired)
			return
		}
		h.app.Logger.Error("alerts: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrAlertUpdateFailed)
		return
	}
	if alert == nil {
		response.NotFoundCode(w, r, i18n.ErrAlertNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "alert.updated",
		EntityType: "alert",
		EntityID:   alert.ID,
		EntityName: alert.Name,
	})

	response.OK(w, alert)
}

// HandleDelete removes an alert rule.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")

	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("alerts: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrAlertRemoveFailed)
		return
	}

	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrAlertNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "alert.deleted",
		EntityType: "alert",
		EntityID:   id,
	})

	response.NoContent(w)
}
