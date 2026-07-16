// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// openPostgres opens the external database used for active-active
// multi-node deployments. The DSN is a libpq-style URL passed via
// MCHARBOR_DB_DSN, e.g.:
//
//	postgres://mcharbor:secret@postgres:5432/mcharbor?sslmode=disable
//
// We use pgx as the driver because it correctly handles context
// cancellation, prepared statement caching, and COPY for any
// bulk-insert paths we add later. database/sql gives us the
// standard McHarbor service layer without a rewrite.
//
// Postgres doesn't need the SQLite PRAGMA warmup pass — pgx
// applies the DSN parameters per-connection on its own. We do set
// a conservative pool ceiling so a misbehaving collector can't
// exhaust the connection limit and stall every other goroutine.
func openPostgres(dsn string) (*sql.DB, error) {
	// Minimal sanity check on the DSN so we fail fast with a
	// readable error instead of a pgx parse error buried in
	// sql.Open. We require a scheme (postgres:// or postgresql://).
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return nil, fmt.Errorf("MCHARBOR_DB_DSN must start with postgres:// or postgresql:// (got prefix %q)", truncate(dsn, 16))
	}

	database, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening postgres database: %w", err)
	}

	// Pool sizing. With WAL-equivalent semantics on Postgres (every
	// connection is a snapshot) the writer bottleneck is gone, so
	// MaxOpenConns can be generous. 32 matches the per-node
	// concurrency budget (event collector, metrics, two
	// schedulers, four recovery goroutines, and the web request
	// pool) with headroom.
	database.SetMaxOpenConns(32)
	database.SetMaxIdleConns(8)
	database.SetConnMaxIdleTime(5 * time.Minute)
	// Bound the worst-case statement time so a runaway query
	// (typically from a long-lived container log read) doesn't
	// tie up a connection forever.
	database.SetConnMaxLifetime(30 * time.Minute)

	// Verify connectivity up front so the operator gets a clear
	// error at startup rather than at the first request.
	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("pinging postgres database: %w", err)
	}

	// Apply a per-session default for any future DSNs that don't
	// set timezone explicitly. We default to UTC so the McHarbor
	// time-based queries (cron firing, retention windows) line up
	// with the rest of the app regardless of the DB host's local
	// time. The coordinator and scheduler both work in UTC.
	if _, err := database.ExecContext(pingCtx, "SET TIME ZONE 'UTC'"); err != nil {
		// Non-fatal: some managed Postgres services reject SET
		// (e.g. read-only replicas). Log and move on; the
		// application already sends tz-aware RFC3339 timestamps.
		slog.Warn("postgres: could not set session timezone to UTC", "error", err)
	}

	slog.Info("database opened", "driver", DriverPostgres, "dsn", redactDSN(dsn))
	return database, nil
}

// redactDSN strips the password from a libpq URL so it can be
// logged safely. The user:password@host pattern is replaced with
// user:***@host. If the DSN doesn't have a password (keypair auth,
// trust-based local dev) it's returned unchanged.
func redactDSN(dsn string) string {
	at := strings.LastIndex(dsn, "@")
	scheme := strings.Index(dsn, "://")
	if at < 0 || scheme < 0 || at <= scheme+3 {
		return dsn
	}
	creds := dsn[scheme+3 : at]
	colon := strings.Index(creds, ":")
	if colon < 0 {
		return dsn
	}
	return dsn[:scheme+3] + creds[:colon+1] + "***" + dsn[at:]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
