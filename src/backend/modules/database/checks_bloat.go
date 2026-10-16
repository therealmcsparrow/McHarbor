// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"fmt"
	"os"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	coresettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// retentionWindow is one retention-governed table: the column that carries
// its age and the configured number of days it may keep.
type retentionWindow struct {
	Table  string
	Column string
	Days   func(coresettings.RetentionSettings) int
}

// retentionWindows is the explicit set of tables measured against a configured
// window. There is no per-table retention config — core/settings holds one
// record — and the age column differs per table, so both are named here.
//
// audit_logs is deliberately absent: its retention is a compliance decision,
// not bloat. See governedTables in relations.go.
var retentionWindows = []retentionWindow{
	{"container_events", "timestamp", func(s coresettings.RetentionSettings) int { return s.ActivityRetentionDays }},
	{"lifecycle_events", "timestamp", func(s coresettings.RetentionSettings) int { return s.LifecycleRetentionDays }},
	{"container_backup_log", "created_at", func(s coresettings.RetentionSettings) int { return s.BackupLogRetentionDays }},
	{"scans", "created_at", func(s coresettings.RetentionSettings) int { return s.ScanRetentionDays }},
}

// checkBloat reports per-table size and the space a VACUUM would reclaim.
func (c *Checker) checkBloat(ctx context.Context, driver coredb.Driver, tables []string) {
	c.setPhase(checkPhaseBloat)

	known := map[string]bool{}
	var (
		totalRows int64
		counted   int
	)
	for i, table := range tables {
		if ctx.Err() != nil {
			return
		}
		known[table] = true
		c.setTable(table, i+1, len(tables))

		countCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		n, err := coredb.RowCount(countCtx, c.source, driver, table)
		cancel()
		if err != nil {
			c.add(Finding{Key: "row_count_failed", Family: familyBloat, Severity: severityWarn, Table: table, Detail: sanitize(err, "")})
			continue
		}
		counted++
		totalRows += n
		c.add(Finding{Key: "table_rows", Family: familyBloat, Severity: severityInfo, Table: table, Count: n})
	}

	// Gated on counted, not on totalRows: if every RowCount failed the total is
	// also zero, and an unconditional emit would report "0 rows" as a fact about
	// the database rather than an absence of measurement.
	if counted > 0 {
		c.add(Finding{Key: "total_rows", Family: familyBloat, Severity: severityInfo, Count: totalRows})
	}

	c.reportTableSizes(ctx, driver)

	if driver == coredb.DriverSQLite {
		c.sqliteReclaimable(ctx)
	} else {
		c.add(Finding{
			Key:      "reclaim_unavailable",
			Family:   familyBloat,
			Severity: severitySkip,
			Reason:   "driver_server_managed",
		})
	}

	c.checkRetentionOverrun(ctx, driver, known)
}

// reportTableSizes adds a byte size per table.
//
// SQLite needs the dbstat virtual table, which is a compile-time option and is
// absent in some modernc builds, so a failure there is a skip rather than a
// warning — the row counts above already carry most of the signal.
func (c *Checker) reportTableSizes(ctx context.Context, driver coredb.Driver) {
	runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	var query string
	switch driver {
	case coredb.DriverPostgres:
		query = `SELECT c.relname, pg_total_relation_size(c.oid)
		           FROM pg_class c
		           JOIN pg_namespace n ON n.oid = c.relnamespace
		          WHERE c.relkind = 'r' AND n.nspname = current_schema()`
	case coredb.DriverMySQL:
		query = `SELECT table_name, COALESCE(data_length, 0) + COALESCE(index_length, 0)
		           FROM information_schema.tables
		          WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'`
	default:
		query = `SELECT name, SUM(pgsize) FROM dbstat GROUP BY name`
	}

	rows, err := c.source.QueryContext(runCtx, query)
	if err != nil {
		c.add(Finding{Key: "table_size_unavailable", Family: familyBloat, Severity: severitySkip, Reason: "size_stats_unavailable"})
		return
	}
	defer func() { logCloseErr(rows.Close()) }()

	var (
		total   int64
		scanErr error
	)
	for rows.Next() {
		var name string
		var bytes int64
		if err := rows.Scan(&name, &bytes); err != nil {
			scanErr = err
			break
		}
		total += bytes
		c.add(Finding{Key: "table_size", Family: familyBloat, Severity: severityInfo, Table: name, Bytes: bytes})
	}
	// rows.Next() returns false on a mid-stream error as well as on clean
	// completion, so without this the total would silently understate the
	// database and read as authoritative.
	if scanErr == nil {
		scanErr = rows.Err()
	}
	if scanErr != nil {
		c.add(Finding{Key: "table_size_unavailable", Family: familyBloat, Severity: severitySkip, Reason: "size_stats_unavailable"})
		return
	}
	c.add(Finding{Key: "total_size", Family: familyBloat, Severity: severityInfo, Bytes: total})
}

// sqliteReclaimable reports the free pages a VACUUM would return to the disk,
// and the write-ahead log sitting beside the database file.
func (c *Checker) sqliteReclaimable(ctx context.Context) {
	if path := c.svc.sqliteFilePath(); path != "" {
		if info, err := os.Stat(path + "-wal"); err == nil && info.Size() > 0 {
			c.add(Finding{
				Key:      "wal_size",
				Family:   familyBloat,
				Severity: severityInfo,
				Bytes:    info.Size(),
				Remedy:   remedyVacuum,
			})
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	var freelist, pageSize int64
	if err := c.source.QueryRowContext(runCtx, "PRAGMA freelist_count").Scan(&freelist); err != nil {
		c.add(Finding{Key: "reclaim_unavailable", Family: familyBloat, Severity: severitySkip, Reason: "pragma_failed"})
		return
	}
	if err := c.source.QueryRowContext(runCtx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		pageSize = 4096
	}

	bytes := freelist * pageSize
	if bytes == 0 {
		c.add(Finding{Key: "reclaim_none", Family: familyBloat, Severity: severityOK})
		return
	}
	c.add(Finding{
		Key:      "reclaimable",
		Family:   familyBloat,
		Severity: severityInfo,
		Bytes:    bytes,
		Count:    freelist,
		Remedy:   remedyVacuum,
	})
}

// checkRetentionOverrun reports tables holding rows older than the policy.
//
// Advice only: the correct action is adjusting the policy or letting the
// retention job run, both of which live in the Retention tab.
func (c *Checker) checkRetentionOverrun(ctx context.Context, driver coredb.Driver, known map[string]bool) {
	settings := coresettings.ReadRetentionSettingsContext(ctx, c.source)

	// bounded by retentionWindows, a fixed list whose entries each name their own table and column.
	for _, w := range retentionWindows {
		// A cancelled context fails every QueryRowContext below instantly, and
		// each failure would emit a retention_query_failed warning the
		// cancellation itself manufactured. Stop instead of inventing findings.
		if ctx.Err() != nil {
			return
		}
		if !known[w.Table] {
			c.add(Finding{Key: "retention_table_missing", Family: familyBloat, Severity: severitySkip, Table: w.Table, Reason: "table_absent"})
			continue
		}
		days := w.Days(settings)
		if days <= 0 {
			// 0 means keep forever throughout this settings group, so every
			// row being "older than the window" would be a false alarm.
			c.add(Finding{Key: "retention_disabled", Family: familyBloat, Severity: severitySkip, Table: w.Table, Reason: "retention_disabled"})
			continue
		}

		cutoff := time.Now().UTC().AddDate(0, 0, -days)
		runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		query := fmt.Sprintf( // SQL identifier: every %s is a coredb.QuoteIdent result or Placeholder
			"SELECT COUNT(*) FROM %s WHERE %s < %s",
			coredb.QuoteIdent(driver, w.Table),
			coredb.QuoteIdent(driver, w.Column),
			coredb.Placeholder(driver, 1),
		)
		var n int64
		err := c.source.QueryRowContext(runCtx, query, cutoff).Scan(&n)
		cancel()
		if err != nil {
			c.add(Finding{Key: "retention_query_failed", Family: familyBloat, Severity: severityWarn, Table: w.Table, Detail: sanitize(err, "")})
			continue
		}
		if n == 0 {
			continue
		}
		c.add(Finding{
			Key:      "retention_overrun",
			Family:   familyBloat,
			Severity: severityInfo,
			Table:    w.Table,
			Count:    n,
		})
	}
}
