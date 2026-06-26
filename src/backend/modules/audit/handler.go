// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for audit HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new audit handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB)
	return &Handler{app: app, service: svc}
}

// HandleList returns paginated audit logs with optional filtering.
//
// Supported query parameters:
//
//	action      - filter by action string (exact match)
//	entity_type - filter by entity type (exact match)
//	from        - RFC3339 timestamp, inclusive lower bound
//	to          - RFC3339 timestamp, inclusive upper bound
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	page, perPage := response.ParsePagination(r)
	q := r.URL.Query()
	action := strings.TrimSpace(q.Get("action"))
	entityType := strings.TrimSpace(q.Get("entity_type"))

	from, fromOK := parseTimeParam(q.Get("from"))
	to, toOK := parseTimeParam(q.Get("to"))
	if q.Has("from") && !fromOK {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if q.Has("to") && !toOK {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	logs, total, err := h.service.List(page, perPage, action, entityType, from, to)
	if err != nil {
		h.app.Logger.Error("failed to list audit logs", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Paginated(w, logs, total, page, perPage)
}

// parseTimeParam parses an RFC3339 (or date-only) timestamp query value.
// Returns ok=false if the value is non-empty but unparseable.
func parseTimeParam(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, true
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// HandleCreate inserts a new audit log entry (internal use).
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req CreateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Action == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	// Auto-fill user info from auth context if not provided
	if req.UserID == nil {
		req.UserID = &user.ID
	}
	if req.Username == nil {
		req.Username = &user.Username
	}

	log, err := h.service.Create(req)
	if err != nil {
		h.app.Logger.Error("failed to create audit log", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Created(w, log)
}

// HandlePurge deletes every audit log row older than the configured
// retention cutoff. The action is itself recorded as a new audit
// entry so the user can see when the purge happened.
//
// The DELETE on the underlying table is gated by the SettingsManage
// RBAC permission since it is a settings-level destructive action.
//
// When the ?vacuum=true query parameter is set, the handler also
// triggers a synchronous WAL checkpoint and a background VACUUM so
// the freed pages are returned to the OS. Without this, the main
// database file does not shrink after a DELETE — SQLite marks pages
// as free in the free list but the file size stays the same.
func (h *Handler) HandlePurge(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	deleted, days, err := h.service.PurgeByRetention(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to purge audit logs", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "purge",
		EntityType: "audit_log",
		Details: fmt.Sprintf(
			"cleared %d audit log entries older than %d days", deleted, days,
		),
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
