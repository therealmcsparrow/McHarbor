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

	_ "github.com/microsoft/go-mssqldb"
)

// openMSSQL opens Microsoft SQL Server 2019+ or Azure SQL. The DSN is taken from
// MCHARBOR_DB_DSN in the go-mssqldb URL form, because the ODBC keyword form has
// no place to carry the query parameters the translation layer depends on:
//
//	sqlserver://user:pass@host:1433?database=mcharbor&encrypt=true
//
// SQL Server is the one backend whose isolation default would break McHarbor:
// READ COMMITTED takes shared read locks, so a long report blocks the writer
// that produced its rows. READ_COMMITTED_SNAPSHOT is set on the database rather
// than per session — a session-level SET cannot enable it — so this only checks
// it and says loudly when it is off, since silently running under locking reads
// is how a multi-node install deadlocks under load.
func openMSSQL(dsn string) (*sql.DB, error) {
	return openMSSQLContext(context.Background(), dsn)
}

// openMSSQLContext is openMSSQL with the caller's context bounding the connectivity check.
func openMSSQLContext(ctx context.Context, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("mssql driver requires MCHARBOR_DB_DSN")
	}
	if !strings.HasPrefix(strings.ToLower(dsn), "sqlserver://") {
		return nil, fmt.Errorf("MCHARBOR_DB_DSN must be a go-mssqldb URL (e.g. `sqlserver://user:pass@host:1433?database=mcharbor&encrypt=true`); the ODBC keyword form is not accepted")
	}

	database, err := openDriver("sqlserver", dsn, DriverMSSQL)
	if err != nil {
		return nil, fmt.Errorf("opening mssql database: %w", err)
	}

	// Pool sizing mirrors Postgres and MySQL. SQL Server's default worker
	// budget is far larger than 32 connections, so the cap is about bounding
	// this node rather than protecting the server.
	database.SetMaxOpenConns(32)
	database.SetMaxIdleConns(8)
	database.SetConnMaxIdleTime(connMaxIdleTime)
	database.SetConnMaxLifetime(connMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := database.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("pinging mssql database: %w", err)
	}

	warnIfLockingReads(pingCtx, database)
	if err := refuseXactAbort(pingCtx, database); err != nil {
		if closeErr := database.Close(); closeErr != nil {
			slog.Warn("mssql: closing refused connection pool", "error", closeErr)
		}
		return nil, err
	}

	slog.Info("database opened", "driver", DriverMSSQL, "dsn", redactMSSQLDSN(dsn))
	return database, nil
}

// warnIfLockingReads reports a database still on locking READ COMMITTED. It is a
// warning and not a refusal because the remedy needs ALTER DATABASE, which the
// application's login may legitimately not hold.
func warnIfLockingReads(ctx context.Context, database *sql.DB) {
	var snapshotOn bool
	err := database.QueryRowContext(ctx,
		"SELECT is_read_committed_snapshot_on FROM sys.databases WHERE name = DB_NAME()",
	).Scan(&snapshotOn)
	switch {
	case err != nil:
		slog.Warn("mssql: could not read is_read_committed_snapshot_on; if reads block writes, this is why",
			"error", err)
	case !snapshotOn:
		slog.Warn("mssql: READ_COMMITTED_SNAPSHOT is off, so reads take shared locks and a long query will block writers",
			"remedy", "ALTER DATABASE [<db>] SET READ_COMMITTED_SNAPSHOT ON WITH ROLLBACK IMMEDIATE")
	}
}

// xactAbortOption is the XACT_ABORT bit of @@OPTIONS; SESSIONPROPERTY does not
// report XACT_ABORT, so the bitmask is the only way to read it.
const xactAbortOption = 16384

// refuseXactAbort stops boot when sessions run with XACT_ABORT on, because the
// INSERT OR IGNORE translation swallows duplicate keys in a CATCH that would then
// sit inside a doomed transaction and roll back the whole db.Tx block.
func refuseXactAbort(ctx context.Context, database *sql.DB) error {
	var options int64
	if err := database.QueryRowContext(ctx, "SELECT CAST(@@OPTIONS AS BIGINT)").Scan(&options); err != nil {
		return fmt.Errorf("reading mssql session options: %w", err)
	}
	if options&xactAbortOption != 0 {
		return errors.New("mssql sessions run with XACT_ABORT ON, which turns every ignored duplicate key into a transaction rollback; remove it from the login's or connector's session settings")
	}
	return nil
}

// redactMSSQLDSN strips the password from a go-mssqldb URL so it can be logged.
func redactMSSQLDSN(dsn string) string {
	return redactDSN(dsn)
}
