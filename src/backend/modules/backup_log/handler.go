// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package backup_log

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

// Handler is the HTTP layer for backup log reads.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new backup_log handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB),
	}
}

// HandleList returns the paginated backup log feed. The Logging
// tab in the Backup page calls this with environment, plan, run,
// severity, and free-text filters.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if user := auth.UserFromContext(r.Context()); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	q := r.URL.Query()
	f := ListFilter{
		EnvironmentID: q.Get("envId"),
		PlanID:        q.Get("planId"),
		RunID:         q.Get("runId"),
		Severity:      q.Get("severity"),
		Action:        q.Get("action"),
		Search:        q.Get("search"),
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.From = t
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Until = t
		}
	}
	f.Page, _ = strconv.Atoi(q.Get("page"))
	f.PerPage, _ = strconv.Atoi(q.Get("perPage"))

	items, total, err := h.service.List(r.Context(), f)
	if err != nil {
		h.app.Logger.Error("backup_log: list failed", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	response.OK(w, map[string]any{
		"items":   items,
		"total":   total,
		"page":    f.Page,
		"perPage": f.PerPage,
	})
}

// HandleRecord lets admin tooling (e.g. the McHarbor stacks module)
// push a log entry. Today the writes happen inside the
// container_backups module so this is rarely called from the UI;
// the route exists for scripts and future integrations.
func (h *Handler) HandleRecord(w http.ResponseWriter, r *http.Request) {
	if user := auth.UserFromContext(r.Context()); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var req struct {
		EnvironmentID string `json:"environmentId"`
		PlanID        string `json:"planId"`
		PlanName      string `json:"planName"`
		RunID         string `json:"runId"`
		ContainerID   string `json:"containerId"`
		ContainerName string `json:"containerName"`
		Action        string `json:"action"`
		Phase         string `json:"phase"`
		Severity      string `json:"severity"`
		Message       string `json:"message"`
	}
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if req.Action == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	id, err := h.service.Record(
		r.Context(),
		req.EnvironmentID, req.PlanID, req.PlanName, req.RunID,
		req.ContainerID, req.ContainerName,
		req.Action, req.Phase, req.Severity, req.Message,
	)
	if err != nil {
		h.app.Logger.Error("backup_log: record failed", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	response.OK(w, map[string]any{"id": id})
}

// HandlePurge deletes every container_backup_log row older than
// the configured retention cutoff. The action is recorded as a
// new audit entry so the user can see when the purge happened.
// When the ?vacuum=true query parameter is set the handler also
// triggers a WAL checkpoint + background VACUUM.
func (h *Handler) HandlePurge(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	deleted, days, err := h.service.PurgeByRetention(r.Context())
	if err != nil {
		h.app.Logger.Error("backup_log: purge failed", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "purge",
		EntityType: "container_backup_log",
		Details:    fmt.Sprintf("cleared %d backup log rows older than %d days", deleted, days),
	})
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

// Mount registers backup_log routes. All are protected — the log
// feed is operator-visible but may contain environment ids.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/backup-logs", func(r chi.Router) {
			r.Get("/", h.HandleList)
			r.Post("/", h.HandleRecord)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Delete("/", h.HandlePurge)
		})
	})
}
