// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package notifications

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Channel represents a notification channel.
type Channel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`   // email, slack, discord, webhook, telegram
	Config    string `json:"config"` // JSON config object (encrypted sensitive fields)
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// CreateChannelInput is the request body for creating a notification channel.
type CreateChannelInput struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Config string `json:"config"`
}

// UpdateChannelInput is the request body for updating a notification channel.
type UpdateChannelInput struct {
	Name    *string `json:"name"`
	Type    *string `json:"type"`
	Config  *string `json:"config"`
	Enabled *bool   `json:"enabled"`
}

// Handler holds dependencies for notification HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new notifications handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB, app.Encryption)
	return &Handler{app: app, service: svc}
}

// HandleConfiguredTypes returns the distinct channel types that have at least one enabled channel.
func (h *Handler) HandleConfiguredTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.service.ConfiguredTypes(r.Context())
	if err != nil {
		h.app.Logger.Error("notifications: configured types error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrNotificationListFailed)
		return
	}
	if types == nil {
		types = []string{}
	}

	response.OK(w, types)
}

// HandleList returns a paginated list of notification channels.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(r.Context(), page, perPage)
	if err != nil {
		h.app.Logger.Error("notifications: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrNotificationListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleCreate creates a new notification channel.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateChannelInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrNotificationNameRequired)
		return
	}
	if input.Type == "" {
		response.BadRequestCode(w, r, i18n.ErrNotificationTypeRequired)
		return
	}
	if !validChannelTypes[input.Type] {
		response.BadRequestCode(w, r, i18n.ErrNotificationInvalidType)
		return
	}

	ch, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("notifications: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrNotificationCreateFailed)
		return
	}

	response.Created(w, ch)
}

// HandleGet returns a single notification channel.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ch, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("notifications: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrNotificationListFailed)
		return
	}
	if ch == nil {
		response.NotFoundCode(w, r, i18n.ErrNotificationNotFound)
		return
	}

	response.OK(w, ch)
}

// HandleUpdate updates an existing notification channel.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateChannelInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	ch, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("notifications: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrNotificationUpdateFailed)
		return
	}
	if ch == nil {
		response.NotFoundCode(w, r, i18n.ErrNotificationNotFound)
		return
	}

	response.OK(w, ch)
}

// HandleDelete removes a notification channel.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("notifications: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrNotificationRemoveFailed)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrNotificationNotFound)
		return
	}

	response.NoContent(w)
}

// HandleTestNotification sends a test notification to the channel.
func (h *Handler) HandleTestNotification(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.service.TestNotification(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("notifications: test notification error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrNotificationListFailed)
		return
	}
	if result == nil {
		response.NotFoundCode(w, r, i18n.ErrNotificationNotFound)
		return
	}

	response.OK(w, result)
}
