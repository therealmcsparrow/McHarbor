// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	coreinstancebackup "github.com/therealmcsparrow/mcharbor/core/instancebackup"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Log represents a single audit log entry.
type Log struct {
	ID            string   `json:"id"`
	UserID        *string  `json:"userId,omitempty"`
	Username      *string  `json:"username,omitempty"`
	Action        string   `json:"action"`
	Severity      string   `json:"severity"`
	EntityType    *string  `json:"entityType,omitempty"`
	EntityID      *string  `json:"entityId,omitempty"`
	EntityName    *string  `json:"entityName,omitempty"`
	Details       *string  `json:"details,omitempty"`
	StateBefore   any      `json:"stateBefore,omitempty"`
	StateAfter    any      `json:"stateAfter,omitempty"`
	ChangedKeys   []string `json:"changedKeys,omitempty"`
	IPAddress     *string  `json:"ipAddress,omitempty"`
	EnvironmentID *string  `json:"environmentId,omitempty"`
	// EnvironmentName is the name the environment carried when the event was
	// written, so the row still names its host after the environment is gone.
	EnvironmentName *string `json:"environmentName,omitempty"`
	// APIKeyID and APIKeyName name the key a change was made with; both are
	// empty for a change made through a browser session.
	APIKeyID   *string `json:"apiKeyId,omitempty"`
	APIKeyName *string `json:"apiKeyName,omitempty"`
	// NodeID is the cluster node that served the action; empty on rows written
	// before nodes recorded themselves.
	NodeID *string `json:"nodeId,omitempty"`
	// ExecNodeID is the node that actually ran the action's remote work, set only
	// when it was not the node that served the request — an agent call forwarded
	// across an active/active cluster.
	ExecNodeID *string `json:"execNodeId,omitempty"`
	Timestamp  string  `json:"timestamp"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

// Service handles audit log operations against the database.
type Service struct {
	db         *sql.DB
	enc        *encryption.Service
	logger     *slog.Logger
	hmacSecret string
}

const maxVerifyRows = 1000000

// instanceRestoreAnchor is the action of the row an instance restore writes
// into the chain it replaced.
const instanceRestoreAnchor = coreinstancebackup.AnchorAction

type listFilter struct {
	Page    int
	PerPage int
	// Action is the first requested action, kept for the export metadata and
	// the access-log line; Actions is the full set the query filters on.
	Action        string
	Actions       []string
	EntityType    string
	EntityID      string
	EnvironmentID string
	From          time.Time
	To            time.Time
	Search        string
	SearchMode    string
	ExpandDiff    bool
	// Actor matches a username (case-insensitively) or a user id; ActorIDs are
	// the ids it resolved to, so a renamed user's older rows still match.
	Actor    string
	ActorIDs []string
	// ActorInDetails also matches `user_id=<id>` inside details, for rows such
	// as abac.denied that are written without a request and so carry no user_id.
	ActorInDetails bool
	// Severities is an OR set over the stored severity column.
	Severities []string
	// DetailsAny is an OR set of LIKE patterns (escaped with '!') over details.
	DetailsAny []string
	// EnvironmentExact drops the "global rows match every environment" rule, for
	// a tab where the reader picked one environment on purpose.
	EnvironmentExact bool
	// Fields is the export column selection; empty exports every column.
	Fields []string
}

// searchColumns are the fields the free-text query matches. They mirror the
// columns the audit table renders, so a search over the wire narrows the same
// rows the reader would have narrowed by eye.
var searchColumns = []string{
	"al.username",
	"al.action",
	"al.severity",
	"al.entity_type",
	"al.entity_id",
	"al.entity_name",
	"al.details",
	"al.environment_name",
	"al.api_key_name",
}

// NewService creates a new audit service.
func NewService(db *sql.DB, enc *encryption.Service, logger *slog.Logger, hmacSecret string) *Service {
	return &Service{db: db, enc: enc, logger: logger, hmacSecret: hmacSecret}
}

// List returns paginated audit logs with optional filtering.
//
// from and to are inclusive lower/upper bounds on the timestamp column.
// A zero time.Time disables the corresponding bound.
func (s *Service) List(ctx context.Context, filter listFilter) ([]Log, int64, error) {
	var total int64
	offset := (filter.Page - 1) * filter.PerPage
	rows, total, err := s.listRows(ctx, filter, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { logRowsClose(rows.Close()) }()

	var logs []Log
	for rows.Next() {
		log, err := scanAuditLog(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, log)
	}
	if logs == nil {
		logs = []Log{}
	}
	return logs, total, rows.Err()
}

// ListActions returns the audit action catalog, optionally filtered by module.
func (s *Service) ListActions(ctx context.Context, module string) ([]ActionCatalogEntry, error) {
	where := ""
	args := []any{}
	if module != "" {
		where = " WHERE module = ?"
		args = append(args, module)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, module, action, severity_default, description, created_at, updated_at
		FROM audit_action_catalog`+where+`
		ORDER BY module ASC, action ASC
		LIMIT 1000
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("querying audit action catalog: %w", err)
	}
	defer func() { logRowsClose(rows.Close()) }()

	actions := []ActionCatalogEntry{}
	for rows.Next() {
		var entry ActionCatalogEntry
		if err := rows.Scan(&entry.ID, &entry.Module, &entry.Action, &entry.SeverityDefault, &entry.Description, &entry.CreatedAt, &entry.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning audit action catalog: %w", err)
		}
		actions = append(actions, entry)
	}
	return actions, rows.Err()
}

// PluginSummary returns the audit activity of every plugin slug the log has
// ever seen, flagged with whether that plugin is still installed.
//
// The rows are never filtered down to the installed set: an uninstall is itself
// a plugin audit event, so dropping gone plugins would delete the evidence of
// their removal. The join answers the question the flat list could not — this
// slug is history, not activity.
func (s *Service) PluginSummary(ctx context.Context) ([]PluginAuditSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT v.plugin_slug, v.count, COALESCE(v.last_seen, ''),
		       v.info_count, v.success_count, v.warning_count, v.error_count,
		       CASE WHEN p.slug IS NULL THEN 0 ELSE 1 END AS installed,
		       COALESCE(p.enabled, 0) AS enabled
		FROM audit_logs_by_plugin v
		LEFT JOIN appstore_plugins p ON p.slug = v.plugin_slug
		ORDER BY v.count DESC, v.plugin_slug ASC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("querying plugin audit summary: %w", err)
	}
	defer func() { logRowsClose(rows.Close()) }()

	summary := []PluginAuditSummary{}
	for rows.Next() {
		var item PluginAuditSummary
		var infoCount, successCount, warningCount, errorCount int64
		var installed, enabled int
		if err := rows.Scan(&item.PluginSlug, &item.Count, &item.LastSeen, &infoCount, &successCount, &warningCount, &errorCount, &installed, &enabled); err != nil {
			return nil, fmt.Errorf("scanning plugin audit summary: %w", err)
		}
		item.Installed = installed != 0
		item.Enabled = enabled != 0
		item.LastSeen = normalizeAuditTime(item.LastSeen)
		item.SeverityBreakdown = map[string]int64{
			"info":    infoCount,
			"success": successCount,
			"warning": warningCount,
			"error":   errorCount,
		}
		summary = append(summary, item)
	}
	return summary, rows.Err()
}

// CoverageSummary joins the route audit map with event counts to compute coverage.
func (s *Service) CoverageSummary(ctx context.Context) (CoverageSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.module, m.method, m.path, m.handler, m.action, m.entity_type,
		       m.entity_id_source, m.audit_required, m.rationale, COUNT(l.id) AS event_count
		FROM audit_action_route_map m
		LEFT JOIN audit_logs l ON l.action = m.action
		GROUP BY m.id, m.module, m.method, m.path, m.handler, m.action, m.entity_type,
		         m.entity_id_source, m.audit_required, m.rationale
		ORDER BY m.module ASC, m.path ASC, m.method ASC
		LIMIT 1000
	`)
	if err != nil {
		return CoverageSummary{}, fmt.Errorf("querying audit coverage route map: %w", err)
	}
	defer func() { logRowsClose(rows.Close()) }()

	summary := CoverageSummary{Routes: []CoverageRoute{}}
	for rows.Next() {
		var route CoverageRoute
		var required int
		if err := rows.Scan(&route.ID, &route.Module, &route.Method, &route.Path, &route.Handler, &route.Action, &route.EntityType, &route.EntityIDSource, &required, &route.Rationale, &route.EventCount); err != nil {
			return CoverageSummary{}, fmt.Errorf("scanning audit coverage route map: %w", err)
		}
		route.AuditRequired = required != 0
		summary.TotalRoutes++
		if route.AuditRequired {
			summary.RequiredRoutes++
		}
		if route.Action != "" {
			summary.MappedRoutes++
		}
		summary.Routes = append(summary.Routes, route)
	}
	if err := rows.Err(); err != nil {
		return CoverageSummary{}, err
	}
	if summary.RequiredRoutes > 0 {
		summary.UnmappedRequired = summary.RequiredRoutes - summary.MappedRoutes
		summary.CoveragePercent = float64(summary.MappedRoutes) / float64(summary.RequiredRoutes) * 100
	}
	return summary, nil
}

func (s *Service) listRows(ctx context.Context, filter listFilter, offset int) (*sql.Rows, int64, error) {
	var (
		rows  *sql.Rows
		err   error
		total int64
	)

	whereParts := []string{}
	args := []any{}
	switch {
	case len(filter.Actions) > 1:
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(filter.Actions)), ",")
		whereParts = append(whereParts, "al.action IN ("+placeholders+")")
		for _, action := range filter.Actions {
			args = append(args, action)
		}
	case len(filter.Actions) == 1:
		whereParts = append(whereParts, "al.action = ?")
		args = append(args, filter.Actions[0])
	case filter.Action != "":
		whereParts = append(whereParts, "al.action = ?")
		args = append(args, filter.Action)
	}
	if filter.EntityType != "" {
		whereParts = append(whereParts, "al.entity_type = ?")
		args = append(args, filter.EntityType)
	}
	if filter.EntityID != "" {
		whereParts = append(whereParts, "al.entity_id = ?")
		args = append(args, filter.EntityID)
	}
	if filter.EnvironmentID != "" && filter.EnvironmentExact {
		whereParts = append(whereParts, "al.environment_id = ?")
		args = append(args, filter.EnvironmentID)
	} else if filter.EnvironmentID != "" {
		// One expression rather than three OR'd equalities: SQLite without
		// statistics turned the OR into a MULTI-INDEX OR and sorted every
		// env-less row. The same rows match; migration 120 indexes the expression.
		whereParts = append(whereParts, "COALESCE(al.environment_id, '') IN (?, '')")
		args = append(args, filter.EnvironmentID)
	}
	if !filter.From.IsZero() {
		whereParts = append(whereParts, "al.timestamp >= ?")
		args = append(args, filter.From.UTC().Format(time.RFC3339))
	}
	if !filter.To.IsZero() {
		whereParts = append(whereParts, "al.timestamp <= ?")
		args = append(args, filter.To.UTC().Format(time.RFC3339))
	}
	if clause, clauseArgs := db.SearchClause(filter.Search, filter.SearchMode, searchColumns); clause != "" {
		whereParts = append(whereParts, clause)
		args = append(args, clauseArgs...)
	}
	if clause, clauseArgs := actorClause(filter); clause != "" {
		whereParts = append(whereParts, clause)
		args = append(args, clauseArgs...)
	}
	if len(filter.Severities) > 0 {
		whereParts = append(whereParts, "al.severity IN ("+placeholders(len(filter.Severities))+")")
		for _, severity := range filter.Severities {
			args = append(args, severity)
		}
	}
	if len(filter.DetailsAny) > 0 {
		parts := make([]string, 0, len(filter.DetailsAny))
		for _, pattern := range filter.DetailsAny {
			parts = append(parts, "al.details LIKE ? ESCAPE '!'")
			args = append(args, pattern)
		}
		whereParts = append(whereParts, "("+strings.Join(parts, " OR ")+")")
	}

	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " WHERE " + strings.Join(whereParts, " AND ")
	}

	// No join. The environment name is stored on the row (migration 136), so a
	// free-text search matches the name the event was written under -- which
	// still works once the environment has been deleted or renamed.
	const fromClause = " FROM audit_logs al"

	countQuery := "SELECT COUNT(*)" + fromClause + whereClause
	if err = s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting audit logs: %w", err)
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, filter.PerPage, offset)
	diffSelect := "NULL AS state_before, NULL AS state_after, NULL AS changed_keys"
	if filter.ExpandDiff {
		diffSelect = "al.state_before, al.state_after, al.changed_keys"
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT al.id, al.user_id, al.username, al.action, al.severity, al.entity_type,
		       al.entity_id, al.entity_name, al.details, `+diffSelect+`,
		       al.ip_address, al.environment_id, al.environment_name,
		       al.api_key_id, al.api_key_name, al.node_id, al.exec_node_id,
		       al.timestamp, al.created_at, al.updated_at`+
		fromClause+whereClause+`
		ORDER BY al.timestamp DESC
		LIMIT ? OFFSET ?
	`, queryArgs...)

	if err != nil {
		return nil, 0, fmt.Errorf("querying audit logs: %w", err)
	}

	return rows, total, nil
}

func scanAuditLog(rows *sql.Rows) (Log, error) {
	var log Log
	var userID, username, entityType, entityID, entityName sql.NullString
	var details, stateBefore, stateAfter, changedKeys, ipAddr, envID sql.NullString
	var envName, apiKeyID, apiKeyName, nodeID, execNodeID sql.NullString

	err := rows.Scan(
		&log.ID, &userID, &username, &log.Action, &log.Severity,
		&entityType, &entityID, &entityName,
		&details, &stateBefore, &stateAfter, &changedKeys, &ipAddr, &envID, &envName,
		&apiKeyID, &apiKeyName, &nodeID, &execNodeID,
		&log.Timestamp, &log.CreatedAt, &log.UpdatedAt,
	)
	if err != nil {
		return log, fmt.Errorf("scanning audit log: %w", err)
	}

	log.UserID = nullStringPtr(userID)
	log.Username = nullStringPtr(username)
	log.EntityType = nullStringPtr(entityType)
	log.EntityID = nullStringPtr(entityID)
	log.EntityName = nullStringPtr(entityName)
	log.Details = nullStringPtr(details)
	if stateBefore.Valid {
		log.StateBefore = json.RawMessage(stateBefore.String)
	}
	if stateAfter.Valid {
		log.StateAfter = json.RawMessage(stateAfter.String)
	}
	if changedKeys.Valid {
		if err := json.Unmarshal([]byte(changedKeys.String), &log.ChangedKeys); err != nil {
			return log, fmt.Errorf("decoding audit changed keys: %w", err)
		}
	}
	log.IPAddress = nullStringPtr(ipAddr)
	log.EnvironmentID = nullStringPtr(envID)
	log.EnvironmentName = nullStringPtr(envName)
	log.APIKeyID = nullStringPtr(apiKeyID)
	log.APIKeyName = nullStringPtr(apiKeyName)
	log.NodeID = nullStringPtr(nodeID)
	log.ExecNodeID = nullStringPtr(execNodeID)
	log.Timestamp = normalizeAuditTime(log.Timestamp)
	log.CreatedAt = normalizeAuditTime(log.CreatedAt)
	log.UpdatedAt = normalizeAuditTime(log.UpdatedAt)

	return log, nil
}

func normalizeAuditTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.UTC); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05.999", raw, time.UTC); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return raw
}

func nullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// PurgeByRetention applies the audit retention policy now: the single audit
// retention is the default and each severity may override it.
//
// It shares PruneWithPolicy with the scheduled pruner, so a manual purge also
// seals a tombstone over every run of removed chain links and verification still
// passes afterwards. The returned days is the default horizon, kept for the
// settings card that reports it.
func (s *Service) PurgeByRetention(ctx context.Context) (int64, int, error) {
	days := coreSettings.ReadRetentionSettingsContext(ctx, s.db).AuditRetentionDays
	policy, err := coreAudit.ReadRetentionPolicy(ctx, s.db, days)
	if err != nil {
		return 0, days, err
	}
	result, err := coreAudit.PruneWithPolicy(ctx, s.db, s.hmacSecret, policy, time.Now())
	if err != nil {
		return result.Deleted, days, fmt.Errorf("purging audit logs: %w", err)
	}
	slog.Info("audit logs purged", "retention_days", days, "rows_deleted", result.Deleted, "tombstones", result.Tombstones)
	return result.Deleted, days, nil
}

// VerifyHashChain validates the HMAC chain for sealed audit rows.
func (s *Service) VerifyHashChain(ctx context.Context) (VerifyResult, error) {
	result := VerifyResult{
		Verified:      true,
		KeyConfigured: strings.TrimSpace(s.hmacSecret) != "",
		CheckedAt:     time.Now().UTC().Format(time.RFC3339),
		Anchors:       []RestoreAnchor{},
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM audit_logs").Scan(&result.TotalRows); err != nil {
		return result, fmt.Errorf("counting audit rows for verification: %w", err)
	}
	bridge, err := coreAudit.LoadChainBridge(ctx, s.db, s.hmacSecret)
	if err != nil {
		return result, err
	}
	result.Tombstones = bridge.Total
	result.InvalidTombstones = bridge.Invalid

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, username, action, severity, entity_type, entity_id, entity_name,
		       details, state_before, state_after, changed_keys, ip_address, environment_id,
		       api_key_id, timestamp, created_at, updated_at, prev_hash, row_hash
		FROM audit_logs
		ORDER BY timestamp ASC, id ASC
		LIMIT ?
	`, maxVerifyRows)
	if err != nil {
		return result, fmt.Errorf("querying audit rows for verification: %w", err)
	}
	defer func() { logRowsClose(rows.Close()) }()

	previousHash := ""
	for rows.Next() {
		result.CheckedRows++
		row, storedPrevHash, storedRowHash, err := scanHashChainRow(rows)
		if err != nil {
			return result, err
		}
		if storedRowHash == "" {
			result.UnsealedRows++
			continue
		}
		result.HashedRows++
		if storedPrevHash != previousHash {
			// A retention gap is legitimate only when sealed tombstones lead
			// from the last hash seen to exactly the link this row names.
			pruned, bridged := bridge.Cross(previousHash, storedPrevHash)
			if !bridged {
				result.Verified = false
				result.FirstBrokenID = &row.ID
				result.Reason = "previous hash mismatch"
				break
			}
			result.PrunedGaps++
			result.PrunedRows += pruned
		}
		row.PrevHash = storedPrevHash
		expectedHash, err := coreAudit.ComputeRowHash(s.hmacSecret, row)
		if err != nil {
			return result, err
		}
		if !hashStringEqual(storedRowHash, expectedHash) {
			result.Verified = false
			result.FirstBrokenID = &row.ID
			result.Reason = "row hash mismatch"
			break
		}
		previousHash = storedRowHash
		if row.Action == instanceRestoreAnchor {
			a := RestoreAnchor{ID: row.ID, Timestamp: row.Timestamp}
			if row.Username != nil {
				a.Actor = *row.Username
			}
			if row.Details != nil {
				a.Details = *row.Details
			}
			result.Anchors = append(result.Anchors, a)
		}
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterating audit rows for verification: %w", err)
	}
	result.Truncated = result.CheckedRows < result.TotalRows
	result.Complete = result.UnsealedRows == 0 && !result.Truncated
	return result, nil
}

func scanHashChainRow(rows *sql.Rows) (coreAudit.HashRow, string, string, error) {
	var row coreAudit.HashRow
	var userID, username, entityType, entityID, entityName sql.NullString
	var details, stateBefore, stateAfter, changedKeys, ipAddr, envID sql.NullString
	var prevHash, rowHash, apiKeyID sql.NullString
	err := rows.Scan(
		&row.ID, &userID, &username, &row.Action, &row.Severity,
		&entityType, &entityID, &entityName,
		&details, &stateBefore, &stateAfter, &changedKeys, &ipAddr, &envID,
		&apiKeyID, &row.Timestamp, &row.CreatedAt, &row.UpdatedAt, &prevHash, &rowHash,
	)
	row.APIKeyID = nullStringPtr(apiKeyID)
	if err != nil {
		return row, "", "", fmt.Errorf("scanning audit hash row: %w", err)
	}
	row.UserID = nullStringPtr(userID)
	row.Username = nullStringPtr(username)
	row.EntityType = nullStringPtr(entityType)
	row.EntityID = nullStringPtr(entityID)
	row.EntityName = nullStringPtr(entityName)
	row.Details = nullStringPtr(details)
	row.StateBefore = nullStringPtr(stateBefore)
	row.StateAfter = nullStringPtr(stateAfter)
	row.ChangedKeys = nullStringPtr(changedKeys)
	row.IPAddress = nullStringPtr(ipAddr)
	row.EnvironmentID = nullStringPtr(envID)
	return row, nullStringValue(prevHash), nullStringValue(rowHash), nil
}

func nullStringValue(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func hashStringEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var mismatch byte
	for i := range a {
		mismatch |= a[i] ^ b[i]
	}
	return mismatch == 0
}
