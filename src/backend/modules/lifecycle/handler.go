// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lifecycle

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
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
	page, pageErr := strconv.Atoi(r.URL.Query().Get("page"))
	if pageErr != nil || page < 1 {
		page = 1
	}
	perPage, perPageErr := strconv.Atoi(r.URL.Query().Get("perPage"))
	if perPageErr != nil || perPage < 1 {
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
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.OK(w, map[string]any{
		"items":   items,
		"total":   total,
		"page":    page,
		"perPage": perPage,
	})
}

// HandleRecent returns the last N events without filters. The
// dashboard's "what just happened" panel uses this.
func (h *Handler) HandleRecent(w http.ResponseWriter, r *http.Request) {
	n, nErr := strconv.Atoi(r.URL.Query().Get("n"))
	if nErr != nil || n < 1 {
		n = 10
	}
	items, err := h.service.Recent(r.Context(), n)
	if err != nil {
		h.app.Logger.Error("lifecycle: recent events failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, map[string]any{"items": items})
}

// HandleInsert lets other modules (stack deploys, image updates,
// backup migrations) record an event. Gated by settings.manage at
// global scope in Mount, the same key its sibling purge route carries.
func (h *Handler) HandleInsert(w http.ResponseWriter, r *http.Request) {
	var ev LifecycleEvent
	if err := response.DecodeBody(r, &ev); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if ev.SubjectType == "" || ev.SubjectID == "" || ev.EventType == "" || ev.Action == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := h.service.Insert(r.Context(), ev); err != nil {
		h.app.Logger.Error("lifecycle: insert event failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
//
// The two reads stay ungated: the feed is what the Docker logging tab
// and the dashboard "what just happened" panel render, for every
// signed-in user.
//
// Writing and purging are both settings.manage at global scope. Insert
// carried a doc comment calling itself admin-only while no middleware
// enforced it, which left any authenticated account able to write
// arbitrary lifecycle events into the record other people read.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/lifecycle", func(r *apispec.Router) {
			r.Get("/", h.HandleList, apispec.Query("envId", ""), apispec.Query("from", ""), apispec.Query("perPage", ""), apispec.Query("search", ""), apispec.Query("severity", ""), apispec.Query("subjectType", ""), apispec.Query("until", ""))
			r.Get("/recent", h.HandleRecent, apispec.Query("n", ""))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Post("/", h.HandleInsert, apispec.Request(LifecycleEvent{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Delete("/", h.HandlePurge, apispec.Query("vacuum", ""))
		})
	})
}
