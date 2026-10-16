// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// CallRequest is what the chat loop hands to a single tool invocation.
type CallRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Scope     Scope           `json:"scope"`

	// IdempotencyKey (Phase 4) is set by the action plane to dedupe
	// retry of the same action within 24h.
	IdempotencyKey string `json:"idempotencyKey,omitempty"`

	// DryRun (Phase 4) returns the planned action without executing.
	DryRun bool `json:"dryRun,omitempty"`

	// ConsentGranted is set only by the action plane after a persisted
	// action has been approved by the owning user. Direct tool callers
	// must leave this false.
	ConsentGranted bool `json:"-"`
}

// CallResult is the success envelope. JSON-RPC shape per §4.1.
type CallResult struct {
	Data       any        `json:"data"`
	Truncated  bool       `json:"truncated"`
	Bytes      int        `json:"bytes"`
	DurationMS int64      `json:"duration_ms"`
	Cached     bool       `json:"cached"`
	Citations  []Citation `json:"citations,omitempty"`
}

// Citation points at an entity referenced by a tool's output. The
// frontend renders these as clickable chips.
type Citation struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label"`
}

// CallError carries a JSON-RPC error code. Negative codes follow the
// JSON-RPC 2.0 spec; -32001..-32007 are McHarbor-specific per §4.1.
type CallError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	// Details is server-side context for logs and tests only; it never reaches the
	// caller or the model, because it may carry a raw error from a lower layer.
	Details string `json:"-"`
}

func (e *CallError) Error() string { return e.Message }

const (
	// ErrUnauthorized is the code for a failed permission or credential check.
	ErrUnauthorized = -32001
	// ErrForbidden is the code for a caller who lacks the required permission or consent.
	ErrForbidden = -32002
	// ErrNotFound is the code for an unknown tool, entity or route.
	ErrNotFound = -32003
	// ErrToolDisabled is the code (-32004 rate limit and -32005 timeout are reserved but unused) for a tool whose backing service is not wired.
	ErrToolDisabled = -32006
	// ErrOutputTooBig is the code for a tool result that exceeds its byte budget.
	ErrOutputTooBig = -32007
)

// Authorize is the per-tool permission check. Returns nil when the
// tool is callable, *CallError(ErrForbidden) otherwise.
func (t *Tool) Authorize(ctx context.Context, authz Authorizer, userID, envID string) *CallError {
	if len(t.Required) == 0 {
		return nil
	}
	for _, perm := range t.Required {
		allowed, err := authz.HasPermission(ctx, userID, envID, perm)
		if err != nil {
			slog.Error("ai tool: rbac check failed", "tool", t.Name, "user", userID, "permission", string(perm), "error", err)
			return &CallError{Code: ErrUnauthorized, Message: "rbac check failed"}
		}
		if !allowed {
			return &CallError{Code: ErrForbidden, Message: "missing required permission", Details: string(perm)}
		}
	}
	return nil
}

// Call dispatches a tool invocation with timeout, audit hook, cache,
// redaction, and truncation. The audit hook is invoked only when an
// audit logger is configured (see SetAudit).
func Call(ctx context.Context, authz Authorizer, audit AuditHook, cache Cache, req CallRequest) (*CallResult, *CallError) {
	return CallWithSanitizer(ctx, authz, audit, cache, nil, req)
}

// CallWithSanitizer is Call plus a SanitizationObserver. The observer
// is fired when SanitizeResult strips a tool output because it
// contained a prompt-injection phrase. Pass nil for the observer
// to keep the standard Call semantics.
func CallWithSanitizer(ctx context.Context, authz Authorizer, audit AuditHook, cache Cache, obs SanitizationObserver, req CallRequest) (*CallResult, *CallError) {
	t, ok := Get(req.Name)
	if !ok {
		return nil, &CallError{Code: -32601, Message: "tool not found", Details: req.Name}
	}
	if (t.RequiresConsent || !t.ReadOnly) && !req.DryRun && !req.ConsentGranted {
		return nil, &CallError{Code: ErrForbidden, Message: "action requires user consent", Details: req.Name}
	}
	if err := t.Authorize(ctx, authz, req.Scope.UserID, req.Scope.EnvID); err != nil {
		return nil, err
	}
	cacheable := cache != nil && t.ReadOnly && !t.RequiresConsent && !req.DryRun
	if cacheable {
		if cached := cache.Get(ctx, req.Scope.UserID, req.Name, req.Arguments); cached != nil {
			return cached, nil
		}
	}
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	scope := req.Scope
	scope.DryRun = req.DryRun
	started := time.Now()
	result, runErr := t.Handler(runCtx, scope, req.Arguments)
	dur := time.Since(started)

	if runErr != nil {
		if ce, ok := errors.AsType[*CallError](runErr); ok {
			return nil, ce
		}
		slog.Error("ai tool: handler failed", "tool", req.Name, "user", req.Scope.UserID, "error", runErr)
		return nil, &CallError{Code: -32603, Message: "internal error"}
	}

	redacted := Redact(result, RedactStandard)
	// Phase 7: scan for prompt-injection + extra secret patterns.
	final := redacted
	var matched string
	if obs != nil {
		var ok bool
		final, ok, matched = SanitizeResult(redacted)
		if ok {
			obs.LogSanitization(req, matched)
		}
	}
	size := approxSize(final)
	truncated := false
	if t.MaxBytes > 0 && size > t.MaxBytes {
		final = truncateEnvelope(final, t.MaxBytes)
		truncated = true
	}

	out := &CallResult{
		Data:       final,
		Truncated:  truncated,
		Bytes:      size,
		DurationMS: dur.Milliseconds(),
	}
	if cacheable {
		cache.Put(ctx, req.Scope.UserID, req.Name, req.Arguments, out)
	}
	if audit != nil {
		audit.LogToolCall(req, out, dur)
	}
	return out, nil
}

// truncateEnvelope wraps the redacted output in a small envelope that
// tells the LLM to narrow its query (e.g. logs.tail).
func truncateEnvelope(v any, maxBytes int) map[string]any {
	return map[string]any{
		"truncated": true,
		"maxBytes":  maxBytes,
		"hint":      "Output too large. Narrow the query — e.g. logs.query with a smaller tail, or containers.logs with fewer lines.",
		"preview":   previewString(v, 512),
	}
}

func previewString(v any, n int) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

func approxSize(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(b)
}

// --- Audit hook ---

// AuditHook is invoked once per tool call. Implementations write to
// the core/audit logger.
type AuditHook interface {
	LogToolCall(req CallRequest, result *CallResult, dur time.Duration)
}

// SanitizationObserver is an optional Phase 7 hook fired when a tool
// output is sanitized because it contained a likely prompt-injection
// phrase. Implementations should log a warning so admins can review
// the underlying entity. Passing nil disables the observer (the
// sanitization still happens in either case).
type SanitizationObserver interface {
	LogSanitization(req CallRequest, matched string)
}

// --- Cache ---

// Cache stores tool results keyed by (userID, tool, args-hash) with a
// per-tool TTL. The Phase 1 implementation is in cache.go; tests can
// pass nil to disable caching.
type Cache interface {
	Get(ctx context.Context, userID, name string, args json.RawMessage) *CallResult
	Put(ctx context.Context, userID, name string, args json.RawMessage, result *CallResult)
}

// HashArgs returns a short hex digest of the arguments JSON.
func HashArgs(args json.RawMessage) string {
	sum := sha256.Sum256(args)
	return hex.EncodeToString(sum[:8])
}

// --- DB cache implementation ---

// DBCache is the SQLite-backed implementation of Cache. Phase 1
// scaffold; richer behaviour (per-tool TTLs, wipe-on-mutate) lands in
// Phase 7.
type DBCache struct {
	db       *sql.DB
	ttl      time.Duration
	maxBytes int
}

// NewDBCache constructs a DBCache. ttl and maxBytes may be zero to use
// defaults (60s and 256 KiB respectively).
func NewDBCache(db *sql.DB, ttl time.Duration, maxBytes int) *DBCache {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	if maxBytes <= 0 {
		maxBytes = 256 * 1024
	}
	return &DBCache{db: db, ttl: ttl, maxBytes: maxBytes}
}

// Get returns a cached CallResult or nil.
func (c *DBCache) Get(ctx context.Context, userID, name string, args json.RawMessage) *CallResult {
	if c == nil || c.db == nil {
		return nil
	}
	hash := HashArgs(args)
	var (
		raw   []byte
		bytes int
	)
	row := c.db.QueryRowContext(ctx,
		`SELECT response, bytes FROM ai_assistant_tool_cache
		  WHERE cache_key = ? AND expires_at > ?`,
		fmt.Sprintf("%s|%s|%s", userID, name, hash),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err := row.Scan(&raw, &bytes); err != nil {
		return nil
	}
	var out CallResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	out.Cached = true
	return &out
}

// Put stores a CallResult. Skips when result is nil or exceeds budget.
func (c *DBCache) Put(ctx context.Context, userID, name string, args json.RawMessage, result *CallResult) {
	if c == nil || c.db == nil || result == nil {
		return
	}
	if result.Bytes > c.maxBytes {
		return
	}
	hash := HashArgs(args)
	raw, err := json.Marshal(result)
	if err != nil {
		return
	}
	expires := time.Now().Add(c.ttl).UTC().Format(time.RFC3339Nano)
	key := fmt.Sprintf("%s|%s|%s", userID, name, hash)
	if _, err := c.db.ExecContext(ctx,
		`INSERT INTO ai_assistant_tool_cache (cache_key, user_id, tool, args_hash, response, bytes, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(cache_key) DO UPDATE SET
		   response = excluded.response,
		   bytes = excluded.bytes,
		   expires_at = excluded.expires_at`,
		key, userID, name, hash, raw, result.Bytes, expires,
	); err != nil {
		slog.Warn("ai tools: caching result failed", "tool", name, "error", err)
	}
}
