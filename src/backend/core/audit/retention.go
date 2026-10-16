// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/rs/xid"
)

// Severities is the closed set of values audit rows are stored with; auditSeverity
// folds every other input into one of these, so retention is keyed on exactly them.
var Severities = []string{"info", "success", "warning", "error"}

// MaxRetentionDays bounds a per-severity override the same way the settings
// module bounds the single audit retention value.
const MaxRetentionDays = 3650

const (
	retentionCategory = "audit"
	pruneBatchSize    = 20000
	pruneMaxBatches   = 50
	pruneDeleteChunk  = 500
)

// RetentionPolicy is the effective audit retention: DefaultDays applies to every
// severity without its own entry in BySeverity. Zero days means keep forever.
type RetentionPolicy struct {
	DefaultDays int            `json:"defaultDays"`
	BySeverity  map[string]int `json:"bySeverity"`
}

// RetentionSettingKey names the settings row holding a severity's override, so
// the store needs no schema of its own and rides along with instance backups.
func RetentionSettingKey(severity string) string {
	return "audit.retention." + severity + "_days"
}

// ValidSeverity reports whether s is one of the stored audit severities.
func ValidSeverity(s string) bool {
	for _, candidate := range Severities {
		if candidate == s {
			return true
		}
	}
	return false
}

// DaysFor returns the retention that applies to severity, falling back to the
// default for a severity without an override or one outside the known set.
func (p RetentionPolicy) DaysFor(severity string) int {
	if days, ok := p.BySeverity[severity]; ok {
		return days
	}
	return p.DefaultDays
}

// ReadRetentionPolicy loads the per-severity overrides on top of defaultDays; an
// unparseable or out-of-range stored value is treated as absent, never as zero.
func ReadRetentionPolicy(ctx context.Context, db *sql.DB, defaultDays int) (RetentionPolicy, error) {
	policy := RetentionPolicy{DefaultDays: defaultDays, BySeverity: map[string]int{}}
	keys := make([]any, 0, len(Severities))
	for _, severity := range Severities {
		keys = append(keys, RetentionSettingKey(severity))
	}
	rows, err := db.QueryContext(ctx,
		"SELECT key, value FROM settings WHERE key IN (?, ?, ?, ?) LIMIT 1000", keys...)
	if err != nil {
		return policy, fmt.Errorf("reading audit retention overrides: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close audit rows", "error", err)
		}
	}()
	for rows.Next() {
		var key string
		var value sql.NullString
		if err := rows.Scan(&key, &value); err != nil {
			return policy, fmt.Errorf("scanning audit retention override: %w", err)
		}
		days, err := strconv.Atoi(strings.TrimSpace(value.String))
		if err != nil || days < 0 || days > MaxRetentionDays {
			continue
		}
		for _, severity := range Severities {
			if key == RetentionSettingKey(severity) {
				policy.BySeverity[severity] = days
			}
		}
	}
	return policy, rows.Err()
}

// WriteRetentionOverrides stores or clears each severity's override: a nil value
// deletes the row so that severity inherits the default again.
func WriteRetentionOverrides(ctx context.Context, db *sql.DB, overrides map[string]*int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning audit retention update: %w", err)
	}
	defer rollbackLogged(tx)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, severity := range Severities {
		days, present := overrides[severity]
		if !present {
			continue
		}
		key := RetentionSettingKey(severity)
		if days == nil {
			if _, err := tx.ExecContext(ctx, "DELETE FROM settings WHERE key = ?", key); err != nil {
				return fmt.Errorf("clearing audit retention override: %w", err)
			}
			continue
		}
		if *days < 0 || *days > MaxRetentionDays {
			return fmt.Errorf("audit retention for %s out of range", severity)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settings (id, key, value, category, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, xid.New().String(), key, strconv.Itoa(*days), retentionCategory, now, now); err != nil {
			return fmt.Errorf("storing audit retention override: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing audit retention update: %w", err)
	}
	return nil
}

// PruneResult reports what one retention pass removed and how many sealed gaps
// it recorded so the hash chain stays verifiable across them.
type PruneResult struct {
	Deleted    int64            `json:"deleted"`
	Tombstones int              `json:"tombstones"`
	BySeverity map[string]int64 `json:"bySeverity"`
}

// pruneClause builds the WHERE clause selecting every row past its severity's
// horizon; ok is false when nothing is configured to expire.
func pruneClause(policy RetentionPolicy, now time.Time) (string, []any, bool) {
	parts := []string{}
	args := []any{}
	for _, severity := range Severities {
		days := policy.DaysFor(severity)
		if days <= 0 {
			continue
		}
		parts = append(parts, "(severity = ? AND timestamp < ?)")
		args = append(args, severity, now.AddDate(0, 0, -days).UTC().Format(time.RFC3339))
	}
	if policy.DefaultDays > 0 {
		parts = append(parts, "(severity NOT IN (?, ?, ?, ?) AND timestamp < ?)")
		for _, severity := range Severities {
			args = append(args, severity)
		}
		args = append(args, now.AddDate(0, 0, -policy.DefaultDays).UTC().Format(time.RFC3339))
	}
	if len(parts) == 0 {
		return "", nil, false
	}
	return strings.Join(parts, " OR "), args, true
}

// PruneWithPolicy deletes expired rows in bounded batches and, in the same
// transaction as each delete, seals a tombstone over every run of removed chain links.
func PruneWithPolicy(ctx context.Context, db *sql.DB, secret string, policy RetentionPolicy, now time.Time) (PruneResult, error) {
	result := PruneResult{BySeverity: map[string]int64{}}
	clause, args, ok := pruneClause(policy, now)
	if !ok {
		return result, nil
	}
	for batch := 0; batch < pruneMaxBatches; batch++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var victims []PrunedRow
		err := WithSanctionedDelete(ctx, db, func(tx *sql.Tx) error {
			var err error
			victims, err = selectPruneVictims(ctx, tx, clause, args)
			if err != nil || len(victims) == 0 {
				return err
			}
			tombstones, err := PlanTombstones(secret, victims, now)
			if err != nil {
				return err
			}
			if err := insertTombstones(ctx, tx, tombstones); err != nil {
				return err
			}
			result.Tombstones += len(tombstones)
			return deleteVictims(ctx, tx, victims)
		})
		if err != nil {
			return result, fmt.Errorf("pruning audit logs: %w", err)
		}
		for _, victim := range victims {
			result.Deleted++
			result.BySeverity[victim.Severity]++
		}
		if len(victims) < pruneBatchSize {
			break
		}
	}
	return result, nil
}

func selectPruneVictims(ctx context.Context, tx *sql.Tx, clause string, args []any) ([]PrunedRow, error) {
	queryArgs := append(append([]any{}, args...), pruneBatchSize)
	query := "SELECT id, severity, timestamp, COALESCE(prev_hash, ''), COALESCE(row_hash, '') FROM audit_logs WHERE " + clause + " ORDER BY timestamp ASC, id ASC LIMIT ?" // SQL identifier: clause is assembled by pruneClause from fixed fragments with ? placeholders
	rows, err := tx.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("selecting expired audit rows: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close audit rows", "error", err)
		}
	}()
	victims := []PrunedRow{}
	for rows.Next() {
		var v PrunedRow
		if err := rows.Scan(&v.ID, &v.Severity, &v.Timestamp, &v.PrevHash, &v.RowHash); err != nil {
			return nil, fmt.Errorf("scanning expired audit row: %w", err)
		}
		victims = append(victims, v)
	}
	return victims, rows.Err()
}

func insertTombstones(ctx context.Context, tx *sql.Tx, tombstones []Tombstone) error {
	for _, t := range tombstones {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO audit_chain_tombstones (id, entry_hash, exit_hash, row_count, first_row_id, last_row_id,
			                                    first_timestamp, last_timestamp, severities, seal, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, t.ID, t.EntryHash, t.ExitHash, t.RowCount, t.FirstRowID, t.LastRowID,
			t.FirstTimestamp, t.LastTimestamp, t.Severities, t.Seal, t.CreatedAt, t.CreatedAt); err != nil {
			return fmt.Errorf("recording audit chain tombstone: %w", err)
		}
	}
	return nil
}

func deleteVictims(ctx context.Context, tx *sql.Tx, victims []PrunedRow) error {
	for start := 0; start < len(victims); start += pruneDeleteChunk {
		end := min(start+pruneDeleteChunk, len(victims))
		chunk := victims[start:end]
		args := make([]any, 0, len(chunk))
		for _, v := range chunk {
			args = append(args, v.ID)
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		query := "DELETE FROM audit_logs WHERE id IN (" + placeholders + ")" // SQL identifier: placeholders is only ? marks sized to the chunk
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("deleting expired audit rows: %w", err)
		}
	}
	return nil
}
