// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Compile-time check: the agent service is the transport's offline sink.
var _ coreagent.OfflineBatchSink = (*Service)(nil)

// IngestOfflineBatch persists one batch the agent buffered while it could not
// reach this server. Every write is idempotent on the agent-minted id (or, for
// container_metrics, on the (container_id, sampled_at) pair), because the agent
// only deletes a batch after it sees the ack and a lost ack means a resend.
//
// The per-environment tracking switches are honoured exactly as the live
// collectors honour them: an environment with event tracking off gets its
// events dropped here rather than stored, and the batch is still acknowledged
// so the agent clears it.
func (s *Service) IngestOfflineBatch(ctx context.Context, envID string, batch *coreagent.OfflineBatchPayload) error {
	if batch == nil || envID == "" {
		return fmt.Errorf("offline batch without environment")
	}
	switch batch.Kind {
	case "events":
		return s.ingestOfflineEvents(ctx, envID, batch.Events)
	case "metrics":
		return s.ingestOfflineMetrics(ctx, envID, batch.Metrics)
	case "logs":
		return s.ingestOfflineLogs(ctx, envID, batch.Logs)
	default:
		return fmt.Errorf("unknown offline batch kind %q", batch.Kind)
	}
}

func (s *Service) envFlag(ctx context.Context, envID, column string) bool {
	var enabled int
	// column is one of two literals chosen by the caller, never user input.
	if err := s.db.QueryRowContext(ctx,
		"SELECT "+column+" FROM environments WHERE id = ? LIMIT 1", envID).Scan(&enabled); err != nil {
		return true
	}
	return enabled == 1
}

func (s *Service) ingestOfflineEvents(ctx context.Context, envID string, entries []coreagent.OfflineEventEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if !s.envFlag(ctx, envID, "track_container_events_enabled") {
		return nil
	}
	if s.eventClassifier == nil {
		return nil
	}
	recordExec := coreSettings.ReadRetentionSettingsContext(ctx, s.db).RecordExecEvents

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning event transaction: %w", err)
	}
	defer coredb.RollbackTx(tx)

	lifecycleStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO lifecycle_events
			(id, environment_id, subject_type, subject_id, subject_name,
			 event_type, action, state, severity, metadata, source, timestamp,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'docker', ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("preparing lifecycle insert: %w", err)
	}
	defer s.closeStmt(lifecycleStmt)

	containerStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO container_events (id, environment_id, container_id, container_name,
		                              event_type, action, metadata, timestamp,
		                              created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("preparing container event insert: %w", err)
	}
	defer s.closeStmt(containerStmt)

	now := time.Now().UTC().Format(time.RFC3339)
	for _, e := range entries {
		if e.ID == "" || e.ActorID == "" {
			continue
		}
		subjectType, ok := s.eventClassifier.SubjectTypeForDockerEvent(e.Type)
		if !ok {
			continue
		}
		if strings.HasPrefix(e.Action, "exec_") && !recordExec {
			continue
		}
		ts := normalizeRFC3339(e.Timestamp, now)
		var meta any
		if len(e.Attributes) > 0 {
			if b, err := json.Marshal(e.Attributes); err == nil {
				meta = string(b)
			}
		}
		severity, state := s.eventClassifier.LifecycleSeverity(e.Type, e.Action)
		if _, err := lifecycleStmt.ExecContext(ctx,
			e.ID, envID, subjectType, e.ActorID, nullIfEmpty(e.ActorName),
			e.Type, e.Action, nullIfEmpty(state), severity, meta, ts, now, now,
		); err != nil {
			return fmt.Errorf("inserting lifecycle event %s: %w", e.ID, err)
		}
		if e.Type == "container" {
			if _, err := containerStmt.ExecContext(ctx,
				e.ID, envID, e.ActorID, nullIfEmpty(e.ActorName),
				e.Type, e.Action, meta, ts, now, now,
			); err != nil {
				return fmt.Errorf("inserting container event %s: %w", e.ID, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing events: %w", err)
	}
	return nil
}

func (s *Service) ingestOfflineMetrics(ctx context.Context, envID string, entries []coreagent.OfflineMetricEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if !s.envFlag(ctx, envID, "collect_container_metrics_enabled") {
		return nil
	}
	if !coreSettings.ReadAgentSettingsContext(ctx, s.db).MetricsEnabled {
		return nil
	}
	if s.metricsIngester == nil {
		return nil
	}

	// Rows arrive oldest-first but a batch boundary can split one tick across
	// two batches; grouping by sampled_at inside each batch is still right
	// because INSERT OR REPLACE dedupes per (container, tick) and the host
	// aggregate for a split tick is at worst two partial rows a few bytes
	// apart, which the 10-minute rollup averages away.
	groups := make(map[string][]coreagent.OfflineMetricEntry)
	for _, m := range entries {
		if m.ContainerID == "" || m.SampledAt == "" {
			continue
		}
		groups[m.SampledAt] = append(groups[m.SampledAt], m)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		sampledAt, err := time.Parse(time.RFC3339, k)
		if err != nil {
			continue
		}
		if err := s.metricsIngester.IngestSamples(ctx, envID, sampledAt, groups[k]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ingestOfflineLogs(ctx context.Context, envID string, entries []coreagent.OfflineLogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning log transaction: %w", err)
	}
	defer coredb.RollbackTx(tx)

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO agent_logs (id, environment_id, level, message, attrs, logged_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("preparing agent log insert: %w", err)
	}
	defer s.closeStmt(stmt)

	now := time.Now().UTC().Format(time.RFC3339)
	for _, e := range entries {
		if e.ID == "" || e.Message == "" {
			continue
		}
		level := strings.ToLower(strings.TrimSpace(e.Level))
		if level == "" {
			level = "info"
		}
		if _, err := stmt.ExecContext(ctx,
			e.ID, envID, level, e.Message, nullIfEmpty(e.Attrs), normalizeRFC3339(e.LoggedAt, now), now, now,
		); err != nil {
			return fmt.Errorf("inserting agent log %s: %w", e.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing agent logs: %w", err)
	}
	return nil
}

// ListAgentLogs returns the replayed agent log for one environment, newest
// first, paginated.
func (s *Service) ListAgentLogs(ctx context.Context, envID string, page, perPage int) ([]LogEntry, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM agent_logs WHERE environment_id = ?", envID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting agent logs: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, level, message, COALESCE(attrs, ''), logged_at
		FROM agent_logs
		WHERE environment_id = ?
		ORDER BY logged_at DESC
		LIMIT ? OFFSET ?`, envID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, fmt.Errorf("listing agent logs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()

	items := make([]LogEntry, 0, perPage)
	for rows.Next() {
		var e LogEntry
		var attrs string
		if err := rows.Scan(&e.ID, &e.Level, &e.Message, &attrs, &e.LoggedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning agent log: %w", err)
		}
		if attrs != "" {
			e.Attrs = json.RawMessage(attrs)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating agent logs: %w", err)
	}
	return items, total, nil
}

// normalizeRFC3339 re-formats an agent timestamp to second precision RFC3339
// UTC so the column sorts and compares like every other timestamp in the
// tables it lands beside. Falls back to fallback when unparseable.
func normalizeRFC3339(value, fallback string) string {
	if value == "" {
		return fallback
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return fallback
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// closeStmt closes a prepared statement and logs a failure, since a deferred close has no caller to report to.
func (s *Service) closeStmt(stmt *sql.Stmt) {
	if err := stmt.Close(); err != nil {
		slog.Warn("closing prepared statement failed", "error", err)
	}
}
