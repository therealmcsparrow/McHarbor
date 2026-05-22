// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// Entry describes an auditable event.
type Entry struct {
	Action        string
	EntityType    string
	EntityID      string
	EntityName    string
	Details       string
	EnvironmentID string
}

// Logger writes audit log entries to the database.
type Logger struct {
	db *sql.DB
}

// NewLogger creates a new audit logger.
func NewLogger(db *sql.DB) *Logger {
	return &Logger{db: db}
}

// Log writes an audit entry, extracting user and IP from the request.
func (l *Logger) Log(r *http.Request, e Entry) {
	var userID, username *string
	if user := auth.UserFromContext(r.Context()); user != nil {
		userID = &user.ID
		username = &user.Username
	}

	ip := clientIP(r)

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := l.db.Exec(`
		INSERT INTO audit_logs (id, user_id, username, action, entity_type, entity_id,
		                        entity_name, details, ip_address, environment_id,
		                        timestamp, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, username, e.Action,
		nullIfEmpty(e.EntityType), nullIfEmpty(e.EntityID), nullIfEmpty(e.EntityName),
		nullIfEmpty(e.Details), nullIfEmpty(ip), nullIfEmpty(e.EnvironmentID),
		now, now, now); err != nil {
		slog.Error("failed to write audit log", "action", e.Action, "error", err)
		return
	}

	logInAppNotification(l.db, username, e)
}

// LogWithUser writes an audit entry with explicit user info (for login before user is in context).
func (l *Logger) LogWithUser(r *http.Request, userID, username string, e Entry) {
	ip := clientIP(r)
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := l.db.Exec(`
		INSERT INTO audit_logs (id, user_id, username, action, entity_type, entity_id,
		                        entity_name, details, ip_address, environment_id,
		                        timestamp, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, username, e.Action,
		nullIfEmpty(e.EntityType), nullIfEmpty(e.EntityID), nullIfEmpty(e.EntityName),
		nullIfEmpty(e.Details), nullIfEmpty(ip), nullIfEmpty(e.EnvironmentID),
		now, now, now); err != nil {
		slog.Error("failed to write audit log", "action", e.Action, "error", err)
		return
	}

	logInAppNotification(l.db, &username, e)
}

// Prune removes audit log entries older than the specified number of days.
// If days is 0 or negative, no pruning is performed.
func (l *Logger) Prune(days int) {
	if days <= 0 {
		return
	}
	_, err := l.db.Exec("DELETE FROM audit_logs WHERE timestamp < datetime('now', '-' || ? || ' days')", days)
	if err != nil {
		slog.Error("failed to prune audit logs", "error", err)
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return xri
	}
	// Strip port from RemoteAddr
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
