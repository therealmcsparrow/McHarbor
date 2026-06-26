// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package activity

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

// Handler holds dependencies for activity HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new activity handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB)
	return &Handler{app: app, service: svc}
}

// HandleList returns paginated container events with optional filtering.
//
// Supported query parameters:
//
//	env - environment ID (empty = default)
//	from - RFC3339 timestamp, inclusive lower bound
//	to   - RFC3339 timestamp, inclusive upper bound
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	page, perPage := response.ParsePagination(r)
	envID := response.ParseEnvID(r)

	q := r.URL.Query()
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

	events, total, err := h.service.List(page, perPage, envID, from, to)
	if err != nil {
		h.app.Logger.Error("failed to list activity events", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Paginated(w, events, total, page, perPage)
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

// HandleCreate records a new container event.
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

	if req.ContainerID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if req.EventType == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if req.Action == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	evt, err := h.service.Create(req)
	if err != nil {
		h.app.Logger.Error("failed to create activity event", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Created(w, evt)
}

// HandlePurge deletes every container event older than the configured
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
		h.app.Logger.Error("failed to purge activity events", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "purge",
		EntityType: "container_events",
		Details: fmt.Sprintf(
			"cleared %d activity events older than %d days", deleted, days,
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
