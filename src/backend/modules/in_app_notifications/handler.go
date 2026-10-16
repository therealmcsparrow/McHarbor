// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package inappnotifications stores in-app notifications and each user's read state.
package inappnotifications

import (
	"errors"
	"net/http"
	"strings"

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
	q := r.URL.Query()
	f := ListFilter{Query: q.Get("q"), Mode: q.Get("mode"), Status: q.Get("status")}
	for _, l := range strings.Split(q.Get("level"), ",") {
		switch l = strings.TrimSpace(l); l {
		case "info", "success", "warning", "error":
			f.Levels = append(f.Levels, l)
		}
	}
	items, total, err := h.service.ListForUser(r.Context(), user.ID, page, perPage, f)
	if errors.Is(err, ErrInvalidPattern) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err != nil {
		h.app.Logger.Error("in-app notifications: list error", "error", err, "userId", user.ID)
		response.InternalErrorCause(w, r, i18n.ErrInAppNotificationListFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrInAppNotificationCountFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrInAppNotificationReadFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrInAppNotificationReadFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrInAppNotificationDeleteFailed, err)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrInAppNotificationNotFound)
		return
	}

	response.NoContent(w)
}

// HandleCounts returns the caller's notification counts for the page's filter chips.
func (h *Handler) HandleCounts(w http.ResponseWriter, r *http.Request) {
	user := coreauth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	counts, err := h.service.CountsForUser(r.Context(), user.ID)
	if err != nil {
		h.app.Logger.Error("in-app notifications: counts error", "error", err, "userId", user.ID)
		response.InternalErrorCause(w, r, i18n.ErrInAppNotificationListFailed, err)
		return
	}
	response.OK(w, counts)
}
