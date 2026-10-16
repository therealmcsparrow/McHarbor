// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

const (
	auditActionAccessed = "audit.accessed"
	auditActionExported = "audit.exported"
	auditActionVerified = "audit.verified"
)

// Handler holds dependencies for audit HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
	// logging owns the plugin-backed audit forwarding surface. Kept
	// on the same handler because the routes live under /audit and
	// share its permission gates, but it holds no state from Service:
	// the two are independent, and forwarding being unconfigured
	// never affects reading or exporting the trail.
	logging *LoggingService
}

// NewHandler creates a new audit handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB, app.Encryption, app.Logger, app.Config.Secret)
	return &Handler{
		app:     app,
		service: svc,
		logging: NewLoggingService(app.DB, app.Encryption, app.PluginAdapters),
	}
}

// HandleList returns paginated audit logs with optional filtering.
//
// Supported query parameters:
//
//	action      - filter by action string (exact match, repeatable)
//	entity_type - filter by entity type (exact match)
//	entity_id   - filter by entity ID (exact match)
//	env         - filter by environment ID (exact match)
//	from        - RFC3339 timestamp, inclusive lower bound
//	to          - RFC3339 timestamp, inclusive upper bound
//	search      - free-text query over user, action, entity and environment
//	search_mode - `contains` (default) or `exact`; regex stays client-side
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	page, perPage := response.ParsePagination(r)
	expand := strings.TrimSpace(r.URL.Query().Get("expand"))
	if expand != "" && expand != "diff" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	filter, ok := h.parseListFilter(w, r, page, perPage)
	if !ok {
		return
	}
	filter.ExpandDiff = expand == "diff"

	logs, total, err := h.service.List(r.Context(), filter)
	if err != nil {
		h.app.Logger.Error("failed to list audit logs", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.logAuditAccess(r, user, filter, total)

	response.Paginated(w, logs, total, page, perPage)
}

// HandleActions lists the audit action catalog, optionally narrowed to one module.
func (h *Handler) HandleActions(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	module := strings.TrimSpace(r.URL.Query().Get("module"))
	if len(module) > 255 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	actions, err := h.service.ListActions(r.Context(), module)
	if err != nil {
		h.app.Logger.Error("failed to list audit action catalog", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, actions)
}

// HandlePluginSummary returns per-plugin audit event counts with each plugin's current install state.
func (h *Handler) HandlePluginSummary(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	summary, err := h.service.PluginSummary(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to list plugin audit summary", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, summary)
}

// HandleCoverage returns which audit-required routes have an audit mapping.
func (h *Handler) HandleCoverage(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	summary, err := h.service.CoverageSummary(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to read audit coverage summary", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "audit.coverage.accessed",
		Severity:   "info",
		EntityType: "audit_coverage",
		Details:    fmt.Sprintf("coverage %.1f%% mapped=%d required=%d", summary.CoveragePercent, summary.MappedRoutes, summary.RequiredRoutes),
	})
	response.OK(w, summary)
}

// HandleCoverageExport streams the audit coverage report as a download and audits the export.
func (h *Handler) HandleCoverageExport(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	summary, err := h.service.CoverageSummary(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to export audit coverage summary", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "audit.coverage.exported",
		Severity:   "success",
		EntityType: "audit_coverage",
		Details:    fmt.Sprintf("exported coverage report mapped=%d required=%d", summary.MappedRoutes, summary.RequiredRoutes),
	})
	response.OK(w, summary)
}

// HandleExport returns a signed CSV or JSON audit export with checksum metadata.
func (h *Handler) HandleExport(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	filter, ok := h.parseListFilter(w, r, 1, maxExportRows)
	if !ok {
		return
	}
	fields, fieldsOK := parseExportFields(r.URL.Query()["fields"])
	if !fieldsOK {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	filter.Fields = fields
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" && format != "jsonl" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	payload, meta, err := h.service.Export(r.Context(), filter, format)
	if err != nil {
		h.app.Logger.Error("failed to export audit logs", "format", format, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionExported,
		Severity:   "success",
		EntityType: "audit_log",
		Details:    fmt.Sprintf("exported %d audit rows as %s columns=%s checksum=%s", meta.RowCount, format, strings.Join(meta.Manifest.Columns, ","), meta.ChecksumSHA256),
	})

	w.Header().Set("X-McHarbor-Checksum-SHA256", meta.ChecksumSHA256)
	w.Header().Set("X-McHarbor-Signature", meta.Signature)
	w.Header().Set("X-McHarbor-Export-Generated-At", meta.GeneratedAt)
	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"mcharbor-audit-export.csv\"")
	case "jsonl":
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"mcharbor-audit-export.jsonl\"")
	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"mcharbor-audit-export.json\"")
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(payload); err != nil {
		h.app.Logger.Error("failed to write audit export", "error", err)
	}
}

// HandleVerify walks the hash chain on demand, crossing retention gaps only
// through sealed tombstones, and audits the outcome.
func (h *Handler) HandleVerify(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	result, err := h.service.VerifyHashChain(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to verify audit hash chain", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	severity := "success"
	if !result.Verified || !result.Complete {
		severity = "warning"
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionVerified,
		Severity:   severity,
		EntityType: "audit_log",
		Details: fmt.Sprintf(
			"verified=%t complete=%t hashed=%d unsealed=%d truncated=%t pruned_gaps=%d pruned_rows=%d",
			result.Verified, result.Complete, result.HashedRows, result.UnsealedRows, result.Truncated, result.PrunedGaps, result.PrunedRows,
		),
	})
	response.OK(w, result)
}

func (h *Handler) logAuditAccess(r *http.Request, user *auth.User, filter listFilter, total int64) {
	if filter.Page != 1 {
		return
	}

	parts := []string{
		"listed audit logs",
		"matches=" + strconv.FormatInt(total, 10),
	}
	if len(filter.Actions) > 0 {
		parts = append(parts, "action="+strings.Join(filter.Actions, ","))
	} else if filter.Action != "" {
		parts = append(parts, "action="+filter.Action)
	}
	if filter.Search != "" {
		mode := filter.SearchMode
		if mode == "" {
			mode = "contains"
		}
		parts = append(parts, "search_mode="+mode, "search_hash="+h.app.Encryption.StableHash(filter.Search))
	}
	if filter.EntityType != "" {
		parts = append(parts, "entity_type="+filter.EntityType)
	}
	if filter.EntityID != "" {
		parts = append(parts, "entity_id_hash="+h.app.Encryption.StableHash(filter.EntityID))
	}
	if filter.EnvironmentID != "" {
		parts = append(parts, "environment_id_hash="+h.app.Encryption.StableHash(filter.EnvironmentID))
	}
	if !filter.From.IsZero() {
		parts = append(parts, "from="+filter.From.UTC().Format(time.RFC3339))
	}
	if !filter.To.IsZero() {
		parts = append(parts, "to="+filter.To.UTC().Format(time.RFC3339))
	}
	if filter.ExpandDiff {
		parts = append(parts, "expand=diff")
	}
	if filter.Actor != "" {
		parts = append(parts, "actor_hash="+h.app.Encryption.StableHash(filter.Actor))
	}
	if len(filter.Severities) > 0 {
		parts = append(parts, "severity="+strings.Join(filter.Severities, ","))
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionAccessed,
		Severity:   "info",
		EntityType: "audit_log",
		Details:    strings.Join(parts, " "),
	})
}

func (h *Handler) parseListFilter(w http.ResponseWriter, r *http.Request, page, perPage int) (listFilter, bool) {
	q := r.URL.Query()
	// The action filter is repeatable: the UI lets the reader tick several
	// actions at once, and every one of them has to reach the WHERE clause or
	// the pager reports a total the table never shows.
	actions := make([]string, 0, len(q["action"]))
	for _, raw := range q["action"] {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			actions = append(actions, trimmed)
		}
	}
	action := ""
	if len(actions) > 0 {
		action = actions[0]
	}
	entityType := strings.TrimSpace(q.Get("entity_type"))
	entityID := strings.TrimSpace(q.Get("entity_id"))
	environmentID := strings.TrimSpace(q.Get("env"))
	search := strings.TrimSpace(q.Get("search"))
	searchMode := strings.TrimSpace(q.Get("search_mode"))
	actor := strings.TrimSpace(q.Get("actor"))
	severities, severitiesOK := parseSeverities(q["severity"])
	if !severitiesOK || len(actor) > 255 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return listFilter{}, false
	}
	if len(entityType) > 255 || len(entityID) > 255 || len(environmentID) > 255 || len(search) > 255 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return listFilter{}, false
	}
	if len(actions) > 50 || !db.ValidSearchMode(searchMode) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return listFilter{}, false
	}
	for _, entry := range actions {
		if len(entry) > 255 {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return listFilter{}, false
		}
	}

	from, fromOK := parseTimeParam(q.Get("from"))
	to, toOK := parseTimeParam(q.Get("to"))
	if q.Has("from") && !fromOK {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return listFilter{}, false
	}
	if q.Has("to") && !toOK {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return listFilter{}, false
	}

	return listFilter{
		Page:          page,
		PerPage:       perPage,
		Action:        action,
		Actions:       actions,
		Search:        search,
		SearchMode:    searchMode,
		EntityType:    entityType,
		EntityID:      entityID,
		EnvironmentID: environmentID,
		From:          from,
		To:            to,
		Actor:         actor,
		ActorIDs:      h.service.resolveActorIDs(r.Context(), actor),
		Severities:    severities,
	}, true
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

// HandlePurge deletes audit rows older than the configured retention window and audits the purge.
func (h *Handler) HandlePurge(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	deleted, days, err := h.service.PurgeByRetention(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to purge audit logs", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
