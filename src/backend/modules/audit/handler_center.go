// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// logViewAccess records that a reader opened one of the Audit Center views; like
// the list, only the first page is logged so paging does not flood the trail.
func (h *Handler) logViewAccess(r *http.Request, user *auth.User, view string, page int, total int64) {
	if page != 1 {
		return
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionAccessed,
		Severity:   "info",
		EntityType: "audit_log",
		Details:    "listed audit " + view + " matches=" + strconv.FormatInt(total, 10),
	})
}

// HandleDenials pages RBAC and ABAC denials with their details parsed into
// actor, permission, route and environment.
func (h *Handler) HandleDenials(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	page, perPage := response.ParsePagination(r)
	filter, ok := h.parseListFilter(w, r, page, perPage)
	if !ok {
		return
	}
	permission := strings.TrimSpace(r.URL.Query().Get("permission"))
	environment := strings.TrimSpace(r.URL.Query().Get("environment"))
	if len(permission) > 255 || len(environment) > 255 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	filter.EnvironmentID = environment
	filter.EnvironmentExact = environment != ""
	rows, total, err := h.service.ListDenials(r.Context(), filter, permission)
	if err != nil {
		h.app.Logger.Error("failed to list audit denials", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.logViewAccess(r, user, "denials", page, total)
	response.Paginated(w, rows, total, page, perPage)
}

// HandleAIActions pages AI actions grouped by action id, each with its preview,
// consent and execution steps and redacted payloads.
func (h *Handler) HandleAIActions(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	page, perPage := response.ParsePagination(r)
	q := r.URL.Query()
	actor := strings.TrimSpace(q.Get("actor"))
	from, fromOK := parseTimeParam(q.Get("from"))
	to, toOK := parseTimeParam(q.Get("to"))
	if len(actor) > 255 || !fromOK || !toOK {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	filter := aiTrailFilter{Page: page, PerPage: perPage, From: from, To: to}
	if actor != "" {
		filter.ActorIDs = uniqueStrings(append(h.service.resolveActorIDs(r.Context(), actor), actor))
	}
	groups, total, err := h.service.ListAIActions(r.Context(), filter)
	if err != nil {
		h.app.Logger.Error("failed to list ai action trail", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.logViewAccess(r, user, "ai-actions", page, total)
	response.Paginated(w, groups, total, page, perPage)
}

// HandleAIToolEvents pages ai.tool.* events with their details redacted.
func (h *Handler) HandleAIToolEvents(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	page, perPage := response.ParsePagination(r)
	filter, ok := h.parseListFilter(w, r, page, perPage)
	if !ok {
		return
	}
	logs, total, err := h.service.ListAIToolEvents(r.Context(), filter)
	if err != nil {
		h.app.Logger.Error("failed to list ai tool events", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.logViewAccess(r, user, "ai-tools", page, total)
	response.Paginated(w, logs, total, page, perPage)
}

// HandleRetention returns the per-severity audit retention policy.
func (h *Handler) HandleRetention(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	view, err := h.service.Retention(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to read audit retention policy", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, view)
}

// HandleUpdateRetention stores per-severity retention overrides; the pruner
// picks them up on its next hourly pass.
func (h *Handler) HandleUpdateRetention(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	var update RetentionUpdate
	if err := response.DecodeBody(r, &update); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if err := validateRetentionUpdate(update); err != nil {
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]any{"reason": "invalid_retention"})
		return
	}
	before, after, err := h.service.UpdateRetention(r.Context(), update)
	if err != nil {
		h.app.Logger.Error("failed to update audit retention policy", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "audit.retention.updated",
		Severity:   "warning",
		EntityType: "audit_retention",
		Details:    fmt.Sprintf("default=%d before=%s after=%s", after.DefaultDays, formatEffective(before), formatEffective(after)),
	})
	response.OK(w, after)
}

func formatEffective(view RetentionView) string {
	parts := make([]string, 0, len(view.Severities))
	for _, severity := range view.Severities {
		marker := ""
		if view.Overrides[severity] == nil {
			marker = "*"
		}
		parts = append(parts, severity+":"+strconv.Itoa(view.Effective[severity])+marker)
	}
	return strings.Join(parts, ",")
}
