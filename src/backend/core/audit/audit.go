// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package audit records who did what to which resource, chaining rows by hash so tampering is detectable.
package audit

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Entry describes an auditable event.
type Entry struct {
	Action        string
	Severity      string
	EntityType    string
	EntityID      string
	EntityName    string
	Details       string
	StateBefore   string
	StateAfter    string
	ChangedKeys   string
	EnvironmentID string
}

// Logger writes audit log entries to the database.
type Logger struct {
	db         *sql.DB
	hmacSecret string
	mu         sync.Mutex
}

// APIKeyResolver names the API key a request authenticated with, or returns
// empty strings for a session request.
type APIKeyResolver func(ctx context.Context) (id, name string)

var apiKeyResolver atomic.Pointer[APIKeyResolver]

var nodeID atomic.Pointer[string]

// SetNodeID records which McHarbor node writes this process's audit rows, so a
// cluster's shared audit log says where each action was served.
func SetNodeID(id string) {
	nodeID.Store(&id)
}

func currentNodeID() *string {
	if p := nodeID.Load(); p != nil && *p != "" {
		return p
	}
	return nil
}

// SetAPIKeyResolver installs how the logger learns which API key made a
// request. It is injected because core/rbac, which knows, imports this package.
func SetAPIKeyResolver(fn APIKeyResolver) {
	if fn == nil {
		apiKeyResolver.Store(nil)
		return
	}
	apiKeyResolver.Store(&fn)
}

// requestRef is what a row inherits from the request that caused it: the API key it
// authenticated with, empty for a session request, and the node that executed its
// remote work, empty when that was this node.
type requestRef struct {
	id, name string
	execNode *string
}

func requestRefOf(r *http.Request) requestRef {
	if r == nil {
		return requestRef{}
	}
	ref := requestRef{execNode: executorOf(r)}
	fn := apiKeyResolver.Load()
	if fn == nil {
		return ref
	}
	ref.id, ref.name = (*fn)(r.Context())
	return ref
}

// NewLogger creates a new audit logger.
func NewLogger(db *sql.DB, hmacSecret string) *Logger {
	return &Logger{db: db, hmacSecret: hmacSecret}
}

// Log writes an audit entry, extracting user and IP from the request.
func (l *Logger) Log(r *http.Request, e Entry) {
	var userID, username *string
	if r != nil {
		if user := auth.UserFromContext(r.Context()); user != nil {
			userID = &user.ID
			username = &user.Username
		}
	}

	ip := clientIP(r)

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	ctx := context.Background() //nolint:contextcheck // a nil request means there is no caller context to inherit
	if r != nil {
		ctx = detachedContext(r.Context())
	}
	severity := l.auditSeverity(ctx, e)

	if err := l.insert(ctx, id, userID, username, requestRefOf(r), e, severity, ip, now); err != nil {
		slog.Error("failed to write audit log", "action", e.Action, "error", err)
		return
	}

	logInAppNotification(ctx, l.db, username, e)
}

// LogWithUser writes an audit entry with explicit user info (for login before user is in context).
//
// An empty username is resolved from userID rather than stored blank: a failed
// MFA login knows only the id, and an actor column reading as an xid is what
// the trail is supposed to prevent.
func (l *Logger) LogWithUser(r *http.Request, userID, username string, e Entry) {
	ctx := context.Background() //nolint:contextcheck // a nil request means there is no caller context to inherit
	if r != nil {
		ctx = detachedContext(r.Context())
	}
	if strings.TrimSpace(username) == "" && strings.TrimSpace(userID) != "" {
		if resolved := l.usernameFor(ctx, userID); resolved != nil {
			username = *resolved
		}
	}
	ip := clientIP(r)
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	severity := l.auditSeverity(ctx, e)

	if err := l.insert(ctx, id, &userID, &username, requestRefOf(r), e, severity, ip, now); err != nil {
		slog.Error("failed to write audit log", "action", e.Action, "error", err)
		return
	}

	logInAppNotification(ctx, l.db, &username, e)
}

// Record writes an entry and returns the failure instead of logging it, for
// a caller that must not continue unless the row exists. An empty userID is
// stored as NULL, so a row whose actor is not a McHarbor user still inserts.
func (l *Logger) Record(userID, username string, e Entry) error {
	return l.RecordContext(context.Background(), userID, username, e)
}

// RecordContext is Record bound to the caller's context, so its values and deadline reach the audit write.
func (l *Logger) RecordContext(ctx context.Context, userID, username string, e Entry) error {
	var uid, uname *string
	if strings.TrimSpace(userID) != "" {
		uid = &userID
	}
	if strings.TrimSpace(username) != "" {
		uname = &username
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return l.insert(ctx, xid.New().String(), uid, uname, requestRef{}, e, l.auditSeverity(ctx, e), "", now)
}

// EmitPlugin writes a plugin-origin audit entry. slug identifies the
// plugin (its catalog slug, e.g. "comm-telegram"); event is a free-form
// action name scoped to the plugin ("access.plugin.capability_violation",
// "plugin.install", etc.); fields carries arbitrary structured context
// rendered into the Details column as JSON. Plugin rows are NOT
// attributable to a human user (the plugin acts on behalf of the
// operator who installed it), so user_id / username stay NULL.
func (l *Logger) EmitPlugin(slug, event string, fields map[string]any) error {
	return l.emitPluginSeverity(context.Background(), slug, event, "", fields)
}

// EmitPluginSeverity is EmitPlugin with the severity decided by the
// caller rather than inferred from the action name.
//
// The adapter audit wrappers in core/pluginruntime know the outcome of
// the call they just made, so they say so directly instead of encoding
// it in the action name. That keeps one action per operation —
// "plugin.storage.upload" whether it worked or not — which is what
// makes the Actions catalog readable and the audit filters useful.
// Pass an empty severity to fall back to the catalog / heuristic path.
func (l *Logger) EmitPluginSeverity(slug, event, severity string, fields map[string]any) error {
	return l.emitPluginSeverity(context.Background(), slug, event, severity, fields)
}

// EmitPluginSeverityContext is EmitPluginSeverity for a caller that holds a context, so its values reach the audit write.
func (l *Logger) EmitPluginSeverityContext(ctx context.Context, slug, event, severity string, fields map[string]any) error {
	return l.emitPluginSeverity(ctx, slug, event, severity, fields)
}

// emitPluginSeverity is EmitPluginSeverity with the caller's context, so a request-scoped caller keeps its values without its cancellation.
func (l *Logger) emitPluginSeverity(ctx context.Context, slug, event, severity string, fields map[string]any) error {
	if l == nil || l.db == nil {
		return fmt.Errorf("audit logger not initialised")
	}
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	// Render the fields map as a stable, compact JSON string. nil and
	// empty inputs produce the empty string so the column stays NULL.
	var details *string
	if len(fields) > 0 {
		b, err := json.Marshal(fields)
		if err != nil {
			slog.Error("failed to marshal plugin audit fields", "slug", slug, "event", event, "error", err)
			return err
		}
		s := string(b)
		details = &s
	}

	entityID := slug
	entry := Entry{Action: event, Severity: severity, EntityType: "plugin", EntityID: entityID, EntityName: slug}
	if details != nil {
		entry.Details = *details
	}
	if err := l.insert(ctx, id, nil, nil, requestRef{}, entry, l.auditSeverity(ctx, entry), "", now); err != nil {
		slog.Error("failed to write plugin audit log", "slug", slug, "event", event, "error", err)
		return fmt.Errorf("insert plugin audit: %w", err)
	}
	return nil
}

// PluginEvent is a plugin-origin audit entry that knows who asked for it.
//
// EmitPlugin cannot express an actor, which left every row an add-on wrote —
// a VM destroy included — with a blank Actor column.
type PluginEvent struct {
	Action string
	// UserID is the account that made the request. Empty falls back to
	// EmitPlugin's behaviour of an unattributed row.
	UserID string
	// Severity empty defers to the action catalog, then to the heuristic.
	Severity string
	Fields   map[string]any
}

// EmitPluginEvent writes a plugin-origin audit row attributed to ev.UserID.
//
// The username is resolved here rather than taken from the plugin: a handle a
// plugin can put any name into is not an audit trail.
func (l *Logger) EmitPluginEvent(slug string, ev PluginEvent) error {
	if l == nil || l.db == nil {
		return fmt.Errorf("audit logger not initialised")
	}
	if strings.TrimSpace(ev.UserID) == "" {
		return l.emitPluginSeverity(context.Background(), slug, ev.Action, ev.Severity, ev.Fields)
	}

	var details *string
	if len(ev.Fields) > 0 {
		b, err := json.Marshal(ev.Fields)
		if err != nil {
			slog.Error("failed to marshal plugin audit fields", "slug", slug, "event", ev.Action, "error", err)
			return err
		}
		s := string(b)
		details = &s
	}

	userID := ev.UserID
	ctx := context.Background()
	username := l.usernameFor(ctx, userID)

	// entity_type stays "plugin" and entity_id stays the slug: the
	// audit_logs_by_plugin view and the audit filters both key on them, and a
	// per-subject entity type would drop these rows out of both.
	entry := Entry{Action: ev.Action, Severity: ev.Severity, EntityType: "plugin", EntityID: slug, EntityName: slug}
	if details != nil {
		entry.Details = *details
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if err := l.insert(ctx, id, &userID, username, requestRef{}, entry, l.auditSeverity(ctx, entry), "", now); err != nil {
		slog.Error("failed to write plugin audit log", "slug", slug, "event", ev.Action, "error", err)
		return fmt.Errorf("insert plugin audit: %w", err)
	}
	return nil
}

// environmentNameFor resolves an environment id to its name, or nil when it
// cannot, so the row still names its host after the environment is deleted.
//
// Resolved here rather than at the call site: every caller already passes the
// id, and 234 call sites each looking the name up themselves would drift.
func (l *Logger) environmentNameFor(parent context.Context, envID string) *string {
	if strings.TrimSpace(envID) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	var name string
	if err := l.db.QueryRowContext(ctx,
		`SELECT name FROM environments WHERE id = ? LIMIT 1`, envID,
	).Scan(&name); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("failed to resolve audit environment name", "environmentId", envID, "error", err)
		}
		return nil
	}
	return &name
}

// usernameFor resolves an account id to its username, or nil when it cannot.
//
// A row whose user_id is known but whose username lookup failed is still worth
// writing — losing the audit entry over a display string would be the worse
// trade.
func (l *Logger) usernameFor(parent context.Context, userID string) *string {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	var name string
	if err := l.db.QueryRowContext(ctx,
		`SELECT username FROM users WHERE id = ? LIMIT 1`, userID,
	).Scan(&name); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("failed to resolve audit actor username", "userId", userID, "error", err)
		}
		return nil
	}
	return &name
}

// auditScope is the environment an entry belongs to, resolved once before the
// transaction opens so a busy-retry does not re-query it.
type auditScope struct {
	id   *string
	name *string
}

// isEnvironmentSubject reports whether an entity type is addressed by an
// environment id. An agent belongs to exactly one environment and is named
// after it, so both resolve the same way.
func isEnvironmentSubject(entityType string) bool {
	return entityType == "environment" || entityType == "agent"
}

// resolveScope names the environment the entry is scoped to.
//
// An entry whose subject *is* an environment (or its agent) carries the id in
// EntityID rather than EnvironmentID, so it is taken as the scope too — but only
// when it resolves to a real environment, because some callers put a host
// address there.
func (l *Logger) resolveScope(ctx context.Context, e Entry) auditScope {
	if name := l.environmentNameFor(ctx, e.EnvironmentID); name != nil {
		return auditScope{id: nullIfEmpty(e.EnvironmentID), name: name}
	}
	if isEnvironmentSubject(e.EntityType) {
		if name := l.environmentNameFor(ctx, e.EntityID); name != nil {
			return auditScope{id: nullIfEmpty(e.EntityID), name: name}
		}
	}
	return auditScope{id: nullIfEmpty(e.EnvironmentID)}
}

func (l *Logger) insert(parent context.Context, id string, userID, username *string, key requestRef, e Entry, severity, ip, now string) error {
	if l == nil || l.db == nil {
		return fmt.Errorf("audit logger not initialised")
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	// Read the hash-chain flag once, before the transaction opens. It runs
	// on a separate pooled connection, so issuing it mid-transaction would
	// widen the window between the snapshot-pinning SELECT below and the
	// INSERT — exactly the window that produces SQLITE_BUSY_SNAPSHOT.
	hashChain := coreSettings.ReadAuditSettingBoolContext(ctx, l.db, "audit.hash_chain.enabled", true)

	// Resolved outside the transaction for the same reason, and once rather
	// than per retry.
	scope := l.resolveScope(ctx, e)

	// insertOnce reads (previousRowHash) before it writes, which SQLite
	// cannot upgrade to a writer if another connection commits in between.
	// That failure is not covered by busy_timeout, so re-run the whole
	// transaction instead. Container start/stop is the common trigger: it
	// fans out concurrent writes to container_events and notifications.
	return coredb.RetryOnBusy(ctx, 5, func() error {
		return l.insertOnce(ctx, hashChain, id, userID, username, key, scope, e, severity, ip, now)
	})
}

// insertOnce is one attempt at writing an audit row: open the transaction,
// extend the hash chain, insert, commit. Every failure path rolls back, so
// the caller may simply call it again.
func (l *Logger) insertOnce(ctx context.Context, hashChain bool, id string, userID, username *string, key requestRef, scope auditScope, e Entry, severity, ip, now string) (err error) {
	tx, err := l.beginWrite(ctx)
	if err != nil {
		return fmt.Errorf("begin audit insert transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				slog.Error("failed to rollback audit insert", "error", rollbackErr)
			}
		}
	}()

	// An environment-subject row inherits the resolved name, so the entity
	// column reads as a name rather than an xid without every such call site
	// having to look one up. The same for a row whose subject is the actor.
	entityName := e.EntityName
	if entityName == "" && scope.name != nil && isEnvironmentSubject(e.EntityType) {
		entityName = *scope.name
	}
	if entityName == "" && e.EntityType == "user" && userID != nil && username != nil && e.EntityID == *userID {
		entityName = *username
	}

	prevHash := ""
	if hashChain {
		prevHash, err = previousRowHash(ctx, tx)
		if err != nil {
			return err
		}
	}
	row := HashRow{
		ID:            id,
		PrevHash:      prevHash,
		UserID:        userID,
		Username:      username,
		Action:        e.Action,
		Severity:      severity,
		EntityType:    nullIfEmpty(e.EntityType),
		EntityID:      nullIfEmpty(e.EntityID),
		EntityName:    nullIfEmpty(entityName),
		Details:       nullIfEmpty(e.Details),
		StateBefore:   nullIfEmpty(e.StateBefore),
		StateAfter:    nullIfEmpty(e.StateAfter),
		ChangedKeys:   nullIfEmpty(e.ChangedKeys),
		IPAddress:     nullIfEmpty(ip),
		EnvironmentID: scope.id,
		APIKeyID:      nullIfEmpty(key.id),
		Timestamp:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	rowHash := ""
	if hashChain {
		rowHash, err = ComputeRowHash(l.hmacSecret, row)
		if err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_logs (id, user_id, username, action, severity, entity_type, entity_id,
		                        entity_name, details, state_before, state_after, changed_keys,
		                        ip_address, environment_id, environment_name, timestamp, created_at,
		                        updated_at, prev_hash, row_hash, api_key_id, api_key_name, node_id,
		                        exec_node_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, username, e.Action, severity,
		row.EntityType, row.EntityID, row.EntityName,
		row.Details, row.StateBefore, row.StateAfter, row.ChangedKeys,
		row.IPAddress, row.EnvironmentID, scope.name,
		now, now, now, nullIfEmpty(prevHash), rowHash, row.APIKeyID, nullIfEmpty(key.name), currentNodeID(),
		// Outside ComputeRowHash on purpose: the hash chain covers what happened, and
		// adding a column to it would invalidate every row already written.
		key.execNode)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit audit insert: %w", err)
	}
	return nil
}

// auditTx is the part of a transaction one audit insert uses, so the SQLite
// path can run it on a connection that began with BEGIN IMMEDIATE.
type auditTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	Commit() error
	Rollback() error
}

// beginWrite opens the insert's transaction. On SQLite it takes the write lock
// up front: the insert reads the chain's last hash before it writes, and SQLite
// never runs the busy handler for a reader asking to become a writer, so a
// deferred transaction fails at once with SQLITE_BUSY whenever another
// connection is writing, and an audit entry is lost after a 75 ms retry budget.
// BEGIN IMMEDIATE waits under busy_timeout instead.
func (l *Logger) beginWrite(ctx context.Context) (auditTx, error) {
	if !l.immediateWrites() {
		tx, err := l.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		return tx, nil
	}
	conn, err := l.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		closeLogged(conn, "audit connection")
		return nil, err
	}
	return &immediateTx{conn: conn}, nil
}

// immediateWrites reports whether this logger writes to SQLite. The driver
// is unset before core/db opens the database, which is only ever in tests,
// where the handle's own driver says.
func (l *Logger) immediateWrites() bool {
	switch coredb.DriverOf() {
	case coredb.DriverSQLite:
		return true
	case "":
		return strings.Contains(strings.ToLower(fmt.Sprintf("%T", l.db.Driver())), "sqlite")
	default:
		return false
	}
}

// immediateTx is an audit transaction on one pinned connection. database/sql
// cannot begin an IMMEDIATE transaction, so this one is driven by statement and
// must never hand its connection back to the pool still inside it.
type immediateTx struct {
	conn *sql.Conn
	done bool
}

// ExecContext runs a statement inside the transaction.
func (t *immediateTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.conn.ExecContext(ctx, query, args...)
}

// QueryRowContext runs a query inside the transaction.
func (t *immediateTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.conn.QueryRowContext(ctx, query, args...)
}

// Commit ends the transaction; a failed COMMIT is rolled back so the
// connection is not returned to the pool with the write lock still held.
func (t *immediateTx) Commit() error {
	if t.done {
		return sql.ErrTxDone
	}
	t.done = true
	if _, err := t.conn.ExecContext(context.Background(), "COMMIT"); err != nil {
		t.release("ROLLBACK")
		return err
	}
	return t.conn.Close()
}

// Rollback abandons the transaction and returns the connection.
func (t *immediateTx) Rollback() error {
	if t.done {
		return sql.ErrTxDone
	}
	t.done = true
	t.release("ROLLBACK")
	return nil
}

// release ends the transaction with statement and returns the connection,
// discarding it instead when even that fails, so a connection still inside a
// transaction never reaches another caller.
func (t *immediateTx) release(statement string) {
	if _, err := t.conn.ExecContext(context.Background(), statement); err != nil {
		slog.Error("failed to end an audit transaction; discarding its connection", "error", err)
		if rawErr := t.conn.Raw(func(any) error { return driver.ErrBadConn }); rawErr != nil && !errors.Is(rawErr, driver.ErrBadConn) {
			slog.Error("failed to discard an audit connection", "error", rawErr)
		}
	}
	closeLogged(t.conn, "audit connection")
}

func previousRowHash(ctx context.Context, tx auditTx) (string, error) {
	var prev sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT row_hash
		FROM audit_logs
		WHERE row_hash IS NOT NULL AND row_hash != ''
		ORDER BY timestamp DESC, id DESC
		LIMIT 1
	`).Scan(&prev)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query previous audit hash: %w", err)
	}
	if !prev.Valid {
		return "", nil
	}
	return prev.String, nil
}

func auditSeverity(e Entry) string {
	severity := strings.ToLower(strings.TrimSpace(e.Severity))
	switch severity {
	case "info", "success", "warning", "error":
		return severity
	}

	action := strings.ToLower(strings.TrimSpace(e.Action))
	if strings.Contains(action, "failed") || strings.Contains(action, "error") || strings.Contains(action, "denied") {
		return "error"
	}
	switch action {
	case "delete", "remove", "purge", "revoke", "disable", "stop", "kill":
		return "warning"
	case "create", "start", "enable", "assign", "login":
		return "success"
	default:
		return "info"
	}
}

func (l *Logger) auditSeverity(ctx context.Context, e Entry) string {
	severity := strings.ToLower(strings.TrimSpace(e.Severity))
	switch severity {
	case "info", "success", "warning", "error":
		return severity
	}
	if catalogSeverity := l.catalogSeverity(ctx, e.Action); catalogSeverity != "" {
		return catalogSeverity
	}
	return auditSeverity(e)
}

func (l *Logger) catalogSeverity(parent context.Context, action string) string {
	action = strings.TrimSpace(action)
	if l == nil || l.db == nil || action == "" {
		return ""
	}
	if !coreSettings.ReadAuditSettingBoolContext(parent, l.db, "audit.action_catalog.enabled", true) {
		return ""
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	var severity string
	err := l.db.QueryRowContext(ctx, `
		SELECT severity_default
		FROM audit_action_catalog
		WHERE action = ?
		LIMIT 1
	`, action).Scan(&severity)
	if err == sql.ErrNoRows {
		return ""
	}
	if err != nil {
		if !strings.Contains(err.Error(), "no such table") {
			slog.Error("failed to read audit action catalog severity", "action", action, "error", err)
		}
		return ""
	}
	severity = strings.ToLower(strings.TrimSpace(severity))
	switch severity {
	case "info", "success", "warning", "error":
		return severity
	default:
		return ""
	}
}

// Prune applies the audit retention policy: days is the default horizon and each
// severity may override it, so a zero default no longer means nothing expires.
//
// Rows are deleted through PruneWithPolicy, which records a sealed tombstone for
// every run of removed chain links; without one, the first surviving row would
// point at a hash that no longer exists and verification would fail after every
// prune. A policy that cannot be read skips the pass rather than falling back
// to the default, which could delete rows an override was keeping longer.
func (l *Logger) Prune(days int) {
	l.PruneContext(context.Background(), days)
}

// PruneContext is Prune bound to the caller's context, so a shutdown stops a running prune; the two-minute cap still applies.
func (l *Logger) PruneContext(parent context.Context, days int) {
	if l == nil || l.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	policy, err := ReadRetentionPolicy(ctx, l.db, days)
	if err != nil {
		slog.Error("failed to read audit retention policy; skipping prune", "error", err)
		return
	}
	result, err := PruneWithPolicy(ctx, l.db, l.hmacSecret, policy, time.Now())
	if err != nil {
		slog.Error("failed to prune audit logs", "error", err)
		return
	}
	if result.Deleted > 0 {
		slog.Info("audit logs pruned", "default_days", days, "rows_deleted", result.Deleted, "tombstones", result.Tombstones)
	}
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	return httpx.ClientIP(r)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// detachedContext keeps the request's values for an audit write but drops its
// cancellation, so a client that hangs up mid-request cannot lose the row.
func detachedContext(parent context.Context) context.Context {
	return context.WithoutCancel(parent)
}

// closeLogged closes c and logs a failure, because a deferred Close has no
// caller to return the error to.
func closeLogged(c io.Closer, what string) {
	if err := c.Close(); err != nil {
		slog.Error("failed to close "+what, "error", err)
	}
}

// rollbackLogged rolls tx back and logs anything but "already finished", which
// is the normal outcome after a commit.
func rollbackLogged(tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		slog.Error("failed to roll back audit transaction", "error", err)
	}
}
