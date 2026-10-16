// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	_ "modernc.org/sqlite"
)

// Kinds of buffered rows. Each maps to one table in the local store and to one
// value of OfflineBatchPayload.Kind on the wire.
const (
	offlineKindLogs    = "logs"
	offlineKindEvents  = "events"
	offlineKindMetrics = "metrics"
)

// Row caps per table. The store is a bounded ring: once a table exceeds its
// cap the oldest rows are dropped, so a host that stays offline for weeks
// keeps the most recent history rather than filling the disk. At roughly
// 200 bytes per row the whole file stays well under 100 MB.
const (
	offlineMaxLogs    = 20_000
	offlineMaxEvents  = 50_000
	offlineMaxMetrics = 200_000
)

// OfflineStore is the agent's local SQLite buffer. It receives rows while the
// agent has no server connection and hands them back in batches for replay.
// modernc.org/sqlite is pure Go, so the agent keeps building with CGO off.
type OfflineStore struct {
	db     *sql.DB
	path   string
	logger *slog.Logger
}

// OpenOfflineStore opens (creating if needed) the buffer at path.
func OpenOfflineStore(path string, logger *slog.Logger) (*OfflineStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("creating offline store directory: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("opening offline store: %w", err)
	}
	// One writer at a time keeps SQLite's lock contention out of the picture.
	db.SetMaxOpenConns(1)

	s := &OfflineStore{db: db, path: path, logger: logger}
	if err := s.migrate(); err != nil {
		errlog.Close(db)
		return nil, err
	}
	return s, nil
}

func (s *OfflineStore) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS offline_logs (
			id        TEXT PRIMARY KEY,
			level     TEXT NOT NULL,
			message   TEXT NOT NULL,
			attrs     TEXT,
			logged_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS offline_events (
			id         TEXT PRIMARY KEY,
			type       TEXT NOT NULL,
			action     TEXT NOT NULL,
			actor_id   TEXT NOT NULL,
			actor_name TEXT,
			attributes TEXT,
			timestamp  TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS offline_metrics (
			id             TEXT PRIMARY KEY,
			container_id   TEXT NOT NULL,
			container_name TEXT,
			container_key  TEXT,
			cpu_percent    REAL NOT NULL DEFAULT 0,
			mem_used       INTEGER NOT NULL DEFAULT 0,
			mem_limit      INTEGER NOT NULL DEFAULT 0,
			net_rx         INTEGER NOT NULL DEFAULT 0,
			net_tx         INTEGER NOT NULL DEFAULT 0,
			block_read     INTEGER NOT NULL DEFAULT 0,
			block_write    INTEGER NOT NULL DEFAULT 0,
			pids           INTEGER NOT NULL DEFAULT 0,
			sampled_at     TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(context.Background(), stmt); err != nil {
			return fmt.Errorf("migrating offline store: %w", err)
		}
	}
	return nil
}

// Close releases the database handle.
func (s *OfflineStore) Close() error {
	return s.db.Close()
}

// Path reports where the buffer lives, for log lines.
func (s *OfflineStore) Path() string { return s.path }

// newOfflineID returns a random 24-hex identifier. The server stores these as
// the primary key of the ingested rows, so a replayed batch collides with the
// copy it already wrote instead of duplicating it.
func newOfflineID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is a broken host; fall back to a time-derived id
		// so buffering keeps working rather than panicking the agent.
		return fmt.Sprintf("%024x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// AddLog appends one line of the agent's own log.
func (s *OfflineStore) AddLog(ctx context.Context, entry OfflineLogEntry) error {
	if entry.ID == "" {
		entry.ID = newOfflineID()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO offline_logs (id, level, message, attrs, logged_at) VALUES (?, ?, ?, ?, ?)`,
		entry.ID, entry.Level, entry.Message, nullIfEmptyString(entry.Attrs), entry.LoggedAt,
	)
	if err != nil {
		return fmt.Errorf("buffering log line: %w", err)
	}
	return nil
}

// AddEvent appends one Docker daemon event.
func (s *OfflineStore) AddEvent(ctx context.Context, entry OfflineEventEntry) error {
	if entry.ID == "" {
		entry.ID = newOfflineID()
	}
	var attrs any
	if len(entry.Attributes) > 0 {
		b, err := json.Marshal(entry.Attributes)
		if err != nil {
			return fmt.Errorf("encoding event attributes: %w", err)
		}
		attrs = string(b)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO offline_events (id, type, action, actor_id, actor_name, attributes, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Type, entry.Action, entry.ActorID, nullIfEmptyString(entry.ActorName), attrs, entry.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("buffering docker event: %w", err)
	}
	return nil
}

// AddMetrics appends every sample of one stats tick in a single transaction.
func (s *OfflineStore) AddMetrics(ctx context.Context, entries []OfflineMetricEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning metrics transaction: %w", err)
	}
	defer func() { errlog.Log(tx.Rollback()) }()

	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO offline_metrics
		(id, container_id, container_name, container_key, cpu_percent, mem_used, mem_limit,
		 net_rx, net_tx, block_read, block_write, pids, sampled_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing metrics insert: %w", err)
	}
	defer func() { errlog.Close(stmt) }()

	for _, m := range entries {
		if m.ID == "" {
			m.ID = newOfflineID()
		}
		if _, err := stmt.ExecContext(ctx,
			m.ID, m.ContainerID, nullIfEmptyString(m.ContainerName), nullIfEmptyString(m.ContainerKey),
			m.CPUPercent, m.MemUsed, m.MemLimit, m.NetRx, m.NetTx, m.BlockRead, m.BlockWrite, m.PIDs, m.SampledAt,
		); err != nil {
			return fmt.Errorf("buffering metric sample for %s: %w", m.ContainerID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing metrics: %w", err)
	}
	return nil
}

// Counts reports how many rows of each kind are waiting.
func (s *OfflineStore) Counts(ctx context.Context) (logs, events, metrics int64) {
	errlog.Log(s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM offline_logs`).Scan(&logs))
	errlog.Log(s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM offline_events`).Scan(&events))
	errlog.Log(s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM offline_metrics`).Scan(&metrics))
	return logs, events, metrics
}

// Trim enforces the per-table row caps, dropping the oldest rows first.
func (s *OfflineStore) Trim(ctx context.Context) {
	for _, t := range []struct {
		table string
		max   int
	}{
		{"offline_logs", offlineMaxLogs},
		{"offline_events", offlineMaxEvents},
		{"offline_metrics", offlineMaxMetrics},
	} {
		res, err := s.db.ExecContext(ctx, fmt.Sprintf( // SQL identifier: t.table ranges over the three fixed offline table names above
			`DELETE FROM %s WHERE rowid IN (
				SELECT rowid FROM %s ORDER BY rowid DESC LIMIT -1 OFFSET ?)`, t.table, t.table), t.max)
		if err != nil {
			s.logger.Warn("offline store trim failed", "table", t.table, "error", err)
			continue
		}
		if n := rowsAffected(res); n > 0 {
			s.logger.Warn("offline store trimmed oldest rows", "table", t.table, "dropped", n, "cap", t.max)
		}
	}
}

// rowsAffected reads the deleted-row count and logs, rather than drops, a driver error.
func rowsAffected(res sql.Result) int64 {
	n, err := res.RowsAffected() //nolint:gocritic // this is the helper the ruleguard points callers at
	if err != nil {
		slog.Warn("reading rows affected failed", "error", err)
		return 0
	}
	return n
}

// NextBatch returns up to limit rows of one kind, oldest first, along with the
// ids to delete once the server acknowledges them. Remaining is the count still
// queued after this batch, so the server can log replay progress.
func (s *OfflineStore) NextBatch(ctx context.Context, kind string, limit int) (OfflineBatchPayload, []string, error) {
	payload := OfflineBatchPayload{Kind: kind}
	var ids []string

	switch kind {
	case offlineKindLogs:
		rows, err := s.db.QueryContext(ctx,
			`SELECT id, level, message, COALESCE(attrs, ''), logged_at FROM offline_logs ORDER BY rowid LIMIT ?`, limit)
		if err != nil {
			return payload, nil, fmt.Errorf("reading buffered logs: %w", err)
		}
		defer func() { errlog.Log(rows.Close()) }()
		for rows.Next() {
			var e OfflineLogEntry
			if err := rows.Scan(&e.ID, &e.Level, &e.Message, &e.Attrs, &e.LoggedAt); err != nil {
				return payload, nil, fmt.Errorf("scanning buffered log: %w", err)
			}
			payload.Logs = append(payload.Logs, e)
			ids = append(ids, e.ID)
		}
		if err := rows.Err(); err != nil {
			return payload, nil, fmt.Errorf("reading buffered logs: %w", err)
		}
	case offlineKindEvents:
		rows, err := s.db.QueryContext(ctx,
			`SELECT id, type, action, actor_id, COALESCE(actor_name, ''), COALESCE(attributes, ''), timestamp
			 FROM offline_events ORDER BY rowid LIMIT ?`, limit)
		if err != nil {
			return payload, nil, fmt.Errorf("reading buffered events: %w", err)
		}
		defer func() { errlog.Log(rows.Close()) }()
		for rows.Next() {
			var e OfflineEventEntry
			var attrs string
			if err := rows.Scan(&e.ID, &e.Type, &e.Action, &e.ActorID, &e.ActorName, &attrs, &e.Timestamp); err != nil {
				return payload, nil, fmt.Errorf("scanning buffered event: %w", err)
			}
			if attrs != "" {
				errlog.Log(json.Unmarshal([]byte(attrs), &e.Attributes))
			}
			payload.Events = append(payload.Events, e)
			ids = append(ids, e.ID)
		}
		if err := rows.Err(); err != nil {
			return payload, nil, fmt.Errorf("reading buffered events: %w", err)
		}
	case offlineKindMetrics:
		rows, err := s.db.QueryContext(ctx,
			`SELECT id, container_id, COALESCE(container_name, ''), COALESCE(container_key, ''), cpu_percent,
			        mem_used, mem_limit, net_rx, net_tx, block_read, block_write, pids, sampled_at
			 FROM offline_metrics ORDER BY rowid LIMIT ?`, limit)
		if err != nil {
			return payload, nil, fmt.Errorf("reading buffered metrics: %w", err)
		}
		defer func() { errlog.Log(rows.Close()) }()
		for rows.Next() {
			var m OfflineMetricEntry
			if err := rows.Scan(&m.ID, &m.ContainerID, &m.ContainerName, &m.ContainerKey, &m.CPUPercent,
				&m.MemUsed, &m.MemLimit, &m.NetRx, &m.NetTx, &m.BlockRead, &m.BlockWrite, &m.PIDs, &m.SampledAt); err != nil {
				return payload, nil, fmt.Errorf("scanning buffered metric: %w", err)
			}
			payload.Metrics = append(payload.Metrics, m)
			ids = append(ids, m.ID)
		}
		if err := rows.Err(); err != nil {
			return payload, nil, fmt.Errorf("reading buffered metrics: %w", err)
		}
	default:
		return payload, nil, fmt.Errorf("unknown offline kind %q", kind)
	}

	if len(ids) > 0 {
		var total int64
		errlog.Log(s.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s`, offlineTable(kind))).Scan(&total)) // SQL identifier: offlineTable maps kind onto a closed set of table constants
		payload.Remaining = total - int64(len(ids))
		if payload.Remaining < 0 {
			payload.Remaining = 0
		}
	}
	return payload, ids, nil
}

// DeleteIDs removes acknowledged rows.
func (s *OfflineStore) DeleteIDs(ctx context.Context, kind string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	table := offlineTable(kind)
	if table == "" {
		return fmt.Errorf("unknown offline kind %q", kind)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning delete transaction: %w", err)
	}
	defer func() { errlog.Log(tx.Rollback()) }()
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, table)) // SQL identifier: table comes from offlineTable, a closed set of constants
	if err != nil {
		return fmt.Errorf("preparing delete: %w", err)
	}
	defer func() { errlog.Close(stmt) }()
	for _, id := range ids {
		if _, err := stmt.ExecContext(ctx, id); err != nil {
			return fmt.Errorf("deleting acknowledged row %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing delete: %w", err)
	}
	return nil
}

func offlineTable(kind string) string {
	switch kind {
	case offlineKindLogs:
		return "offline_logs"
	case offlineKindEvents:
		return "offline_events"
	case offlineKindMetrics:
		return "offline_metrics"
	}
	return ""
}

func nullIfEmptyString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
