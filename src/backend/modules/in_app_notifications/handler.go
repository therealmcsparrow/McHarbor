// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package inappnotifications

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	coreauth "github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for in-app notification HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new in-app notifications handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB),
	}
}

// HandleList returns a paginated list of in-app notifications for the current user.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := coreauth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	page, perPage := response.ParsePagination(r)
	items, total, err := h.service.ListForUser(r.Context(), user.ID, page, perPage)
	if err != nil {
		h.app.Logger.Error("in-app notifications: list error", "error", err, "userId", user.ID)
		response.InternalErrorCode(w, r, i18n.ErrInAppNotificationListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleUnreadCount returns the unread notification count for the current user.
func (h *Handler) HandleUnreadCount(w http.ResponseWriter, r *http.Request) {
	user := coreauth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	count, err := h.service.UnreadCount(r.Context(), user.ID)
	if err != nil {
		h.app.Logger.Error("in-app notifications: unread count error", "error", err, "userId", user.ID)
		response.InternalErrorCode(w, r, i18n.ErrInAppNotificationCountFailed)
		return
	}

	response.OK(w, map[string]int64{"count": count})
}

// HandleMarkRead marks a single notification as read for the current user.
func (h *Handler) HandleMarkRead(w http.ResponseWriter, r *http.Request) {
	user := coreauth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	marked, err := h.service.MarkRead(r.Context(), id, user.ID)
	if err != nil {
		h.app.Logger.Error("in-app notifications: mark read error", "error", err, "userId", user.ID, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInAppNotificationReadFailed)
		return
	}
	if !marked {
		response.NotFoundCode(w, r, i18n.ErrInAppNotificationNotFound)
		return
	}

	response.NoContent(w)
}

// HandleMarkAllRead marks all notifications as read for the current user.
func (h *Handler) HandleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	user := coreauth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	count, err := h.service.MarkAllRead(r.Context(), user.ID)
	if err != nil {
		h.app.Logger.Error("in-app notifications: mark all read error", "error", err, "userId", user.ID)
		response.InternalErrorCode(w, r, i18n.ErrInAppNotificationReadFailed)
		return
	}

	response.OK(w, map[string]int64{"count": count})
}

// HandleDelete hides a single notification from the current user's inbox.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user := coreauth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	deleted, err := h.service.DeleteForUser(r.Context(), id, user.ID)
	if err != nil {
		h.app.Logger.Error("in-app notifications: delete error", "error", err, "userId", user.ID, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInAppNotificationDeleteFailed)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrInAppNotificationNotFound)
		return
	}

	response.NoContent(w)
}
