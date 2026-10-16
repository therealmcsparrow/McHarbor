// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"database/sql"
	"strings"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// checkIntegrity runs the storage engine's own consistency check.
//
// Postgres has no in-server equivalent without the amcheck extension, which
// McHarbor cannot assume is installed, so it reports the honest subset —
// invalid indexes — and skips the rest with a stated reason.
func (c *Checker) checkIntegrity(ctx context.Context, driver coredb.Driver, tables []string) {
	c.setPhase(checkPhaseIntegrity)

	switch driver {
	case coredb.DriverSQLite:
		c.sqliteIntegrity(ctx, driver)
	case coredb.DriverMySQL:
		c.mysqlIntegrity(ctx, driver, tables)
	default:
		c.postgresIntegrity(ctx)
	}
}

// sqliteIntegrity runs integrity_check and foreign_key_check.
func (c *Checker) sqliteIntegrity(ctx context.Context, driver coredb.Driver) {
	runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	var problems []string
	rows, err := c.source.QueryContext(runCtx, "PRAGMA integrity_check")
	if err != nil {
		c.add(Finding{Key: "integrity_unavailable", Family: familyIntegrity, Severity: severityFail, Detail: sanitize(err, "")})
	} else {
		var scanErr error
		problems, scanErr = readIntegrityLines(rows)
		if len(problems) > 0 {
			c.add(Finding{
				Key:      "integrity_corrupt",
				Family:   familyIntegrity,
				Severity: severityFail,
				Count:    int64(len(problems)),
				Detail:   strings.Join(problems, "\n"),
			})
		}
		switch {
		case scanErr != nil:
			c.add(Finding{Key: "integrity_unavailable", Family: familyIntegrity, Severity: severityWarn, Detail: sanitize(scanErr, "")})
		case len(problems) == 0:
			c.add(Finding{Key: "integrity_ok", Family: familyIntegrity, Severity: severityOK})
		}
	}

	fkCtx, fkCancel := context.WithTimeout(ctx, checkTimeout)
	defer fkCancel()

	fkRows, err := c.source.QueryContext(fkCtx, "PRAGMA foreign_key_check")
	if err != nil {
		c.add(Finding{Key: "fk_check_unavailable", Family: familyIntegrity, Severity: severityWarn, Detail: sanitize(err, "")})
		return
	}
	defer func() { logCloseErr(fkRows.Close()) }()

	violations := map[string]int64{}
	var scanErr error
	for fkRows.Next() {
		var table, rowid, parent, fkid any
		if err := fkRows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			scanErr = err
			break
		}
		if name, ok := table.(string); ok {
			violations[name]++
		}
	}
	if scanErr == nil {
		scanErr = fkRows.Err()
	}
	for table, n := range violations {
		c.add(Finding{
			Key:      "fk_violation",
			Family:   familyIntegrity,
			Severity: severityFail,
			Table:    table,
			Count:    n,
		})
	}
	switch {
	case scanErr != nil:
		c.add(Finding{Key: "fk_check_unavailable", Family: familyIntegrity, Severity: severityWarn, Detail: sanitize(scanErr, "")})
	case len(violations) == 0:
		c.add(Finding{Key: "fk_check_ok", Family: familyIntegrity, Severity: severityOK})
	}
}

// mysqlIntegrity runs CHECK TABLE on each table.
func (c *Checker) mysqlIntegrity(ctx context.Context, driver coredb.Driver, tables []string) {
	bad := 0
	errored := false
	// bounded by the tables of the checked database; CHECK TABLE takes one table per statement.
	for i, table := range tables {
		if ctx.Err() != nil {
			return
		}
		c.setTable(table, i+1, len(tables))

		runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		rows, err := c.source.QueryContext(runCtx, "CHECK TABLE "+coredb.QuoteIdent(driver, table))
		if err != nil {
			cancel()
			c.add(Finding{Key: "integrity_table_failed", Family: familyIntegrity, Severity: severityWarn, Table: table, Detail: sanitize(err, "")})
			errored = true
			continue
		}
		problems, scanErr := c.readCheckTable(rows, table)
		bad += problems
		cancel()
		if scanErr != nil {
			c.add(Finding{Key: "integrity_table_failed", Family: familyIntegrity, Severity: severityWarn, Table: table, Detail: sanitize(scanErr, "")})
			errored = true
		}
	}
	if bad == 0 && !errored {
		c.add(Finding{Key: "integrity_ok", Family: familyIntegrity, Severity: severityOK, Count: int64(len(tables))})
	}
}

// postgresIntegrity reports invalid indexes and skips the rest.
func (c *Checker) postgresIntegrity(ctx context.Context) {
	runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	rows, err := c.source.QueryContext(runCtx, `
		SELECT c.relname, t.relname
		  FROM pg_index i
		  JOIN pg_class c ON c.oid = i.indexrelid
		  JOIN pg_class t ON t.oid = i.indrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE i.indisvalid = false
		   AND n.nspname = current_schema()`)
	if err != nil {
		c.add(Finding{Key: "integrity_unavailable", Family: familyIntegrity, Severity: severityWarn, Detail: sanitize(err, "")})
		return
	}
	defer func() { logCloseErr(rows.Close()) }()

	invalid := 0
	var scanErr error
	for rows.Next() {
		var index, table string
		if err := rows.Scan(&index, &table); err != nil {
			scanErr = err
			break
		}
		invalid++
		c.add(Finding{
			Key:      "invalid_index",
			Family:   familyIntegrity,
			Severity: severityWarn,
			Table:    table,
			Index:    index,
		})
	}
	if scanErr == nil {
		scanErr = rows.Err()
	}
	switch {
	case scanErr != nil:
		c.add(Finding{Key: "integrity_unavailable", Family: familyIntegrity, Severity: severityWarn, Detail: sanitize(scanErr, "")})
	case invalid == 0:
		c.add(Finding{Key: "integrity_ok", Family: familyIntegrity, Severity: severityOK})
	}

	c.add(Finding{
		Key:      "integrity_partial",
		Family:   familyIntegrity,
		Severity: severitySkip,
		Reason:   "driver_no_native_integrity",
	})
}

// readIntegrityLines collects every non-"ok" line of PRAGMA integrity_check and closes the cursor before the caller queries again.
func readIntegrityLines(rows *sql.Rows) (problems []string, scanErr error) {
	defer func() { logCloseErr(rows.Close()) }()
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return problems, err
		}
		if !strings.EqualFold(strings.TrimSpace(line), "ok") {
			problems = append(problems, line)
		}
	}
	return problems, rows.Err()
}

// readCheckTable records a finding per error or warning row of CHECK TABLE and closes the cursor, returning how many problems it found.
func (c *Checker) readCheckTable(rows *sql.Rows, table string) (bad int, scanErr error) {
	defer func() { logCloseErr(rows.Close()) }()
	for rows.Next() {
		var name, op, msgType, msgText string
		if err := rows.Scan(&name, &op, &msgType, &msgText); err != nil {
			return bad, err
		}
		if strings.EqualFold(msgType, "error") || strings.EqualFold(msgType, "warning") {
			bad++
			c.add(Finding{
				Key:      "integrity_table_problem",
				Family:   familyIntegrity,
				Severity: severityFail,
				Table:    table,
				Detail:   msgText,
			})
		}
	}
	return bad, rows.Err()
}
