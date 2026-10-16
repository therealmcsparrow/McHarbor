// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

type auditResponseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *auditResponseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Unwrap exposes the underlying writer to http.ResponseController.
//
// Without this the controller cannot reach the connection through this
// middleware, so SetReadDeadline and SetWriteDeadline silently report
// ErrNotSupported — which is what made every large upload on a mutating route
// die on the server's 30s ReadTimeout no matter what the handler asked for.
func (r *auditResponseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *auditResponseRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// FallbackMiddleware emits a safe audit row for mutating protected API routes.
// Handlers should still emit richer resource-specific rows for high-value actions.
func FallbackMiddleware(db *sql.DB, log *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if log == nil || !methodNeedsFallbackAudit(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			recorder := &auditResponseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)

			pattern := chi.RouteContext(r.Context()).RoutePattern()
			if pattern == "" {
				pattern = r.URL.Path
			}
			pattern = strings.TrimPrefix(pattern, "/api")
			action, entityType := fallbackAction(detachedContext(r.Context()), db, r.Method, pattern)
			severity := "success"
			if recorder.status >= 400 {
				severity = "warning"
			}
			entry := Entry{
				Action:     action,
				Severity:   severity,
				EntityType: entityType,
				EntityID:   chi.URLParam(r, "id"),
				Details:    fmt.Sprintf("fallback route audit method=%s path=%s status=%d", r.Method, pattern, recorder.status),
			}
			if user := auth.RequireAuth(r); user != nil {
				log.LogWithUser(r, user.ID, user.Username, entry)
				return
			}
			log.Log(r, entry)
		})
	}
}

// PluginFallbackMiddleware emits a plugin-scoped fallback audit row for plugin
// routes that declare audit metadata in plugin.yaml.
func PluginFallbackMiddleware(log *Logger, slug, action, entityType string, required bool) func(http.Handler) http.Handler {
	return PluginFallbackMiddlewareWithSeverity(log, slug, action, entityType, "", required)
}

// PluginFallbackMiddlewareWithSeverity is PluginFallbackMiddleware for a route whose manifest
// declares audit_severity: a successful call is stored at that severity rather than inferred,
// so a plugin reveal lands at warning like the host's own. A failed call stays at least warning.
func PluginFallbackMiddlewareWithSeverity(log *Logger, slug, action, entityType, declared string, required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if log == nil || !required || action == "" || entityType == "" {
				next.ServeHTTP(w, r)
				return
			}
			recorder := &auditResponseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)

			severity := "success"
			if declared != "" {
				severity = declared
			}
			if recorder.status >= 400 && severity != "error" {
				severity = "warning"
			}
			// Without a declaration the row keeps its inferred severity, as it always has.
			rowSeverity := ""
			if declared != "" {
				rowSeverity = severity
			}
			fields := map[string]any{
				"plugin_slug": slug,
				"method":      r.Method,
				"path":        r.URL.Path,
				"status":      recorder.status,
				"severity":    severity,
				"entity_type": entityType,
				"fallback":    true,
			}
			if err := log.emitPluginSeverity(detachedContext(r.Context()), slug, action, rowSeverity, fields); err != nil {
				slog.Error("failed to write plugin fallback audit", "plugin", slug, "action", action, "error", err)
			}
		})
	}
}

func methodNeedsFallbackAudit(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func fallbackAction(ctx context.Context, db *sql.DB, method, path string) (string, string) {
	if db != nil {
		var action, entityType string
		err := db.QueryRowContext(ctx, `
			SELECT action, entity_type
			FROM audit_action_route_map
			WHERE method = ? AND path = ? AND audit_required = 1
			LIMIT 1
		`, method, path).Scan(&action, &entityType)
		if err == nil && action != "" {
			return action, entityType
		}
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	module := "api"
	if len(parts) > 0 && parts[0] != "" {
		module = strings.ReplaceAll(parts[0], "-", "_")
	}
	verb := strings.ToLower(method)
	switch method {
	case http.MethodPost:
		verb = "created"
	case http.MethodPut, http.MethodPatch:
		verb = "updated"
	case http.MethodDelete:
		verb = "deleted"
	}
	return module + ".route." + verb, module
}
