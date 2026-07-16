// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lifecycle

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler is the HTTP layer for the lifecycle module.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new lifecycle handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB),
	}
}

// HandleList returns paginated lifecycle events with optional
// filtering. The same query backs the Logging tab in the Docker
// menu and the per-environment / per-subject dropdowns on it.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
	if perPage < 1 {
		perPage = 50
	}

	f := ListFilter{
		EnvironmentID: r.URL.Query().Get("envId"),
		SubjectType:   r.URL.Query().Get("subjectType"),
		Severity:      r.URL.Query().Get("severity"),
		Search:        r.URL.Query().Get("search"),
	}
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Since = t
		}
	}
	if v := r.URL.Query().Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Until = t
		}
	}

	items, total, err := h.service.List(r.Context(), page, perPage, f)
	if err != nil {
		h.app.Logger.Error("lifecycle: list events failed", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, map[string]any{
		"items": items,
		"total": total,
		"page": page,
		"perPage": perPage,
	})
}

// HandleRecent returns the last N events without filters. The
// dashboard's "what just happened" panel uses this.
func (h *Handler) HandleRecent(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n < 1 {
		n = 10
	}
	items, err := h.service.Recent(r.Context(), n)
	if err != nil {
		h.app.Logger.Error("lifecycle: recent events failed", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	response.OK(w, map[string]any{"items": items})
}

// HandleInsert lets other modules (stack deploys, image updates,
// backup migrations) record an event. Admin-only; protected by
// the standard RBAC permission.
func (h *Handler) HandleInsert(w http.ResponseWriter, r *http.Request) {
	var ev LifecycleEvent
	if err := response.DecodeBody(r, &ev); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if ev.SubjectType == "" || ev.SubjectID == "" || ev.EventType == "" || ev.Action == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := h.service.Insert(r.Context(), ev); err != nil {
		h.app.Logger.Error("lifecycle: insert event failed", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	response.OK(w, ev)
}

// HandlePurge deletes every lifecycle event older than the
// configured retention cutoff. The action is itself recorded as a
// new audit entry so the user can see when the purge happened.
// When the ?vacuum=true query parameter is set, the handler also
// triggers a synchronous WAL checkpoint and a background VACUUM.
func (h *Handler) HandlePurge(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	deleted, days, err := h.service.PurgeByRetention(r.Context())
	if err != nil {
		h.app.Logger.Error("lifecycle: purge failed", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if user != nil {
		h.app.AuditLog.Log(r, audit.Entry{
			Action:     "purge",
			EntityType: "lifecycle_events",
			Details:    fmt.Sprintf("cleared %d lifecycle events older than %d days", deleted, days),
		})
	}
	vacuuming := false
	if r.URL.Query().Get("vacuum") == "true" && h.app.Compact != nil {
		_ = h.app.Compact.CheckpointAndVacuum(r.Context())
		vacuuming = true
	}
	response.OK(w, map[string]any{
		"deleted":       deleted,
		"retentionDays": days,
		"vacuuming":     vacuuming,
	})
}

// Mount registers lifecycle module routes. All protected.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/lifecycle", func(r chi.Router) {
			r.Get("/", h.HandleList)
			r.Get("/recent", h.HandleRecent)
			r.Post("/", h.HandleInsert)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Delete("/", h.HandlePurge)
		})
	})
}
