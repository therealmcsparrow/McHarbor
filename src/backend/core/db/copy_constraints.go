// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// maxRowsPerStatement caps a multi-row INSERT regardless of the bind budget, so
// one statement stays small enough to log, retry and cancel between.
const maxRowsPerStatement = 500

// constraintRestoreTimeout bounds the SQL Server restore, which re-validates every
// suspended foreign key and must run even after the copy's own context is cancelled.
const constraintRestoreTimeout = 2 * time.Minute

// rowsPerStatement sizes one multi-row INSERT for driver: the caller's request when
// it fits, never more rows than the destination's bind budget allows for columns.
func rowsPerStatement(driver Driver, columns, requested int) int {
	if columns < 1 {
		columns = 1
	}
	rows := MaxBindParameters(driver) / columns
	if requested > 0 && requested < rows {
		rows = requested
	}
	if rows > maxRowsPerStatement {
		rows = maxRowsPerStatement
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

// sessionForeignKeyStatements returns the statements that switch foreign-key
// enforcement off and back on for one connection. SQL Server has no such switch,
// so ok is false there and the copier suspends it per table instead.
func sessionForeignKeyStatements(driver Driver) (off, on string, ok bool) {
	switch driver {
	case DriverPostgres:
		return "SET session_replication_role = 'replica'", "SET session_replication_role = 'origin'", true
	case DriverMySQL:
		return "SET FOREIGN_KEY_CHECKS = 0", "SET FOREIGN_KEY_CHECKS = 1", true
	case DriverMSSQL:
		return "", "", false
	default:
		return "PRAGMA foreign_keys = OFF", "PRAGMA foreign_keys = ON", true
	}
}

// constraintSuspension records how a copy switched off the destination's foreign
// keys, so they are switched back on the same way. Postgres only honours
// session_replication_role for a superuser, which a managed database will not
// grant; that is not fatal, because the copy is already ordered parents first.
type constraintSuspension struct {
	driver  Driver
	conn    *sql.Conn
	logger  *slog.Logger
	session bool
	tables  []string
	seen    map[string]bool
	failed  bool
}

// suspendConstraints switches off foreign-key enforcement on the destination for
// the copy. On SQL Server that means NOCHECK and DISABLE TRIGGER on each of tables,
// which also stops the cascade triggers standing in for ON DELETE from firing.
func suspendConstraints(ctx context.Context, conn *sql.Conn, driver Driver, tables []string, logger *slog.Logger) *constraintSuspension {
	s := &constraintSuspension{driver: driver, conn: conn, logger: logger, seen: map[string]bool{}}
	off, _, ok := sessionForeignKeyStatements(driver)
	if !ok {
		for _, table := range tables {
			s.include(ctx, table)
		}
		return s
	}
	if _, err := conn.ExecContext(ctx, off); err != nil {
		logger.Warn("db copy: destination would not suspend foreign key checks; relying on table order",
			"driver", driver, "error", err)
		return s
	}
	s.session = true
	return s
}

// relaxed reports whether enforcement is actually off for everything the copy writes.
func (s *constraintSuspension) relaxed() bool {
	if _, _, ok := sessionForeignKeyStatements(s.driver); ok {
		return s.session
	}
	return !s.failed
}

// include suspends one more table. It is a no-op on the session-switch drivers,
// for a table already suspended, and after a table has refused.
func (s *constraintSuspension) include(ctx context.Context, table string) {
	if _, _, ok := sessionForeignKeyStatements(s.driver); ok || s.failed || s.seen[strings.ToLower(table)] {
		return
	}
	quoted := QuoteIdent(s.driver, table)
	if _, err := s.conn.ExecContext(ctx, "ALTER TABLE "+quoted+" NOCHECK CONSTRAINT ALL"); err != nil {
		s.failed = true
		s.logger.Warn("db copy: SQL Server would not suspend the foreign keys of a table; relying on table order",
			"table", table, "error", err)
		return
	}
	s.seen[strings.ToLower(table)] = true
	s.tables = append(s.tables, table)
	if _, err := s.conn.ExecContext(ctx, "DISABLE TRIGGER ALL ON "+quoted); err != nil {
		s.failed = true
		s.logger.Warn("db copy: SQL Server would not suspend the triggers of a table; relying on table order",
			"table", table, "error", err)
	}
}

// restore undoes the suspension. The session drivers only log a failure, since the
// setting dies with the session; SQL Server's lives on the tables, so each is
// re-validated, and a table whose copied rows fail that is returned as untrusted.
func (s *constraintSuspension) restore(ctx context.Context) ([]string, error) {
	if _, on, ok := sessionForeignKeyStatements(s.driver); ok {
		if !s.session {
			return nil, nil
		}
		if _, err := s.conn.ExecContext(ctx, on); err != nil {
			s.logger.Warn("db copy: restoring foreign key enforcement", "driver", s.driver, "error", err)
		}
		return nil, nil
	}
	if len(s.tables) == 0 {
		return nil, nil
	}
	restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), constraintRestoreTimeout)
	defer cancel()
	var untrusted []string
	for _, table := range s.tables {
		quoted := QuoteIdent(s.driver, table)
		if _, err := s.conn.ExecContext(restoreCtx, "ALTER TABLE "+quoted+" WITH CHECK CHECK CONSTRAINT ALL"); err != nil {
			if _, err := s.conn.ExecContext(restoreCtx, "ALTER TABLE "+quoted+" CHECK CONSTRAINT ALL"); err != nil {
				return untrusted, fmt.Errorf("restoring the foreign keys of %s: %w", table, err)
			}
			untrusted = append(untrusted, table)
		}
		if _, err := s.conn.ExecContext(restoreCtx, "ENABLE TRIGGER ALL ON "+quoted); err != nil {
			return untrusted, fmt.Errorf("restoring the triggers of %s: %w", table, err)
		}
	}
	s.tables = nil
	s.seen = map[string]bool{}
	return untrusted, nil
}

// identityInsertOn lets explicit values into a SQL Server IDENTITY column for the
// rows the copier writes, and returns the call that turns it off again. It is a
// no-op elsewhere, and when the identity column is not among the copied columns.
func identityInsertOn(ctx context.Context, conn *sql.Conn, driver Driver, table string, cols []string) (func(), error) {
	noop := func() {}
	if driver != DriverMSSQL {
		return noop, nil
	}
	var identity sql.NullString
	err := conn.QueryRowContext(ctx,
		"SELECT name FROM sys.identity_columns WHERE object_id = OBJECT_ID(?)", table).Scan(&identity)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !identity.Valid) {
		return noop, nil
	}
	if err != nil {
		return noop, fmt.Errorf("reading the identity column of %s: %w", table, err)
	}
	copied := false
	for _, c := range cols {
		if strings.EqualFold(c, identity.String) {
			copied = true
			break
		}
	}
	if !copied {
		return noop, nil
	}
	quoted := QuoteIdent(driver, table)
	if _, err := conn.ExecContext(ctx, "SET IDENTITY_INSERT "+quoted+" ON"); err != nil { // SQL identifier: quoted is the QuoteIdent form of table
		return noop, fmt.Errorf("allowing explicit identity values into %s: %w", table, err)
	}
	return func() {
		offCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(offCtx, "SET IDENTITY_INSERT "+quoted+" OFF"); err != nil { // SQL identifier: quoted is the QuoteIdent form of table
			slog.Warn("db copy: turning IDENTITY_INSERT off", "table", table, "error", err)
		}
	}, nil
}
