// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// openMySQL opens MySQL or MariaDB for active-active multi-node
// deployments. The DSN is taken from MCHARBOR_DB_DSN and uses the
// go-sql-driver/mysql driver format:
//
//	user:pass@tcp(host:3306)/dbname?parseTime=true&loc=UTC&ssl=true
//
// McHarbor uses parseTime=true so DATE / DATETIME columns
// returned by the driver come back as time.Time, and loc=UTC so
// the application sees the same wall-clock time it sent in
// regardless of the DB host's local timezone. UTF8MB4 is set on
// the session via `SET NAMES utf8mb4` so JSON / multibyte
// columns round-trip cleanly.
//
// The MySQL coordinator lock is a session-level named lock
// acquired with GET_LOCK(name, timeout) and released with
// RELEASE_LOCK(name). The lock is per-connection, so the
// coordinator holds a long-lived *sql.Conn for the lifetime of
// the process.
func openMySQL(dsn string) (*sql.DB, error) {
	return openMySQLContext(context.Background(), dsn)
}

// openMySQLContext is openMySQL with the caller's context bounding the connectivity check.
func openMySQLContext(ctx context.Context, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("mysql driver requires MCHARBOR_DB_DSN")
	}
	// Minimal sanity check so a missing scheme fails with a
	// readable error rather than the driver's generic parse
	// complaint buried inside sql.Open. We accept both the
	// DSN-style `user:pass@tcp(host)/db` and the URL-style
	// `mysql://user:pass@host:3306/db` formats; go-sql-driver
	// normalizes them.
	if !strings.HasPrefix(dsn, "mysql://") &&
		!strings.Contains(dsn, "@tcp(") {
		return nil, fmt.Errorf("MCHARBOR_DB_DSN must be a go-sql-driver/mysql DSN (e.g. `user:pass@tcp(host:3306)/db?parseTime=true&loc=UTC`)")
	}

	database, err := openDriver("mysql", dsn, DriverMySQL)
	if err != nil {
		return nil, fmt.Errorf("opening mysql database: %w", err)
	}

	// Pool sizing mirrors Postgres. MySQL uses InnoDB by default
	// and supports row-level locks with multi-version concurrency,
	// so 32 connections per node gives comfortable headroom for
	// the event collector, metrics, two schedulers, recovery
	// goroutines, and the web request pool.
	database.SetMaxOpenConns(32)
	database.SetMaxIdleConns(8)
	database.SetConnMaxIdleTime(connMaxIdleTime)
	database.SetConnMaxLifetime(connMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := database.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("pinging mysql database: %w", err)
	}

	// Default session settings that keep timestamps and
	// character data consistent across nodes. We do not abort on
	// SET failures because some hosted MySQL providers reject
	// non-default session variables; the application's tz-aware
	// RFC3339 timestamps keep working either way.
	if _, err := database.ExecContext(pingCtx, "SET NAMES utf8mb4"); err != nil {
		slog.Warn("mysql: could not set session charset to utf8mb4", "error", err)
	}
	if _, err := database.ExecContext(pingCtx, "SET time_zone = '+00:00'"); err != nil {
		slog.Warn("mysql: could not set session timezone to UTC", "error", err)
	}
	// NO_BACKSLASH_ESCAPES makes MySQL treat a backslash inside a string
	// literal as an ordinary character, which is what SQLite, PostgreSQL and
	// the SQL standard all do.
	//
	// Without it `LIKE 'swarm\_%' ESCAPE '\'` - a real line in
	// 071_kubernetes_swarm_to_plugins.sql — is an unterminated string on MySQL and takes
	// the rest of the statement with it. McHarbor binds every user value as a
	// parameter rather than interpolating it, so nothing depends on the
	// non-standard behaviour this turns off.
	if _, err := database.ExecContext(pingCtx,
		"SET SESSION sql_mode = CONCAT(@@sql_mode, ',NO_BACKSLASH_ESCAPES')",
	); err != nil {
		slog.Warn("mysql: could not enable NO_BACKSLASH_ESCAPES", "error", err)
	}

	slog.Info("database opened", "driver", DriverMySQL, "dsn", redactMySQLDSN(dsn))
	return database, nil
}

// redactMySQLDSN strips the password from a go-sql-driver DSN so
// it can be logged safely. Both the URL form
// (mysql://user:pass@host/db) and the legacy DSN form
// (user:pass@tcp(host:3306)/db) are handled. The text between
// the first ':' after the user/host separator and the next '@'
// (or '(') is replaced with '***'.
func redactMySQLDSN(dsn string) string {
	// URL form: mysql://user:pass@host/db
	if strings.HasPrefix(dsn, "mysql://") {
		return redactDSN(dsn)
	}
	// Legacy DSN form: user:pass@tcp(host:3306)/db
	// The user/pass portion ends at '@' (which precedes 'tcp(').
	at := strings.Index(dsn, "@")
	if at < 0 {
		return dsn
	}
	creds := dsn[:at]
	colon := strings.Index(creds, ":")
	if colon < 0 {
		return dsn
	}
	return creds[:colon+1] + "***" + dsn[at:]
}
