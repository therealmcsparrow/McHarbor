// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// licschema_check verifies that every licence statement prepares against a freshly migrated schema.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	_ "modernc.org/sqlite"
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func setting(ctx context.Context, db *sql.DB, query string, dest any) error {
	if err := db.QueryRowContext(ctx, query).Scan(dest); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read setting: %w", err)
	}
	return nil
}

func run() error {
	// The migrate helper takes no context, so a caller-supplied one would be dropped; this tool owns its lifetime.
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "licschema")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer closeutil.RemoveAll(dir)
	db, err := sql.Open("sqlite", filepath.Join(dir, "t.db"))
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { closeutil.Report(db.Close(), "db") }()
	if err := coredb.Migrate(db, coredb.DriverSQLite); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	stmts := []string{
		`SELECT id, license_key, key_masked, server_license_id, tier, status,
			valid, reason, seats_total, seats_used, seats_unlimited,
			perpetual, starts_at, expires_at, days_remaining, auto_renew,
			instance_registered, holds_seat, activated_at, last_checked_at, last_error
			FROM licenses WHERE active = 1 ORDER BY created_at DESC LIMIT 1`,
		`INSERT INTO licenses (id, license_key, key_masked, server_license_id, tier, status,
			valid, reason, seats_total, seats_used, seats_unlimited,
			perpetual, starts_at, expires_at, days_remaining, auto_renew,
			instance_registered, holds_seat, activated_at, last_checked_at,
			activated_by, active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		`UPDATE licenses SET status = ?, tier = CASE WHEN ? = '' THEN tier ELSE ? END,
			valid = ?, reason = ?, seats_total = ?, seats_used = ?, seats_unlimited = ?,
			perpetual = ?, starts_at = COALESCE(?, starts_at), expires_at = ?,
			days_remaining = ?, auto_renew = ?, instance_registered = ?, holds_seat = ?,
			last_checked_at = ?, last_error = NULL, updated_at = ? WHERE id = ?`,
		`UPDATE licenses SET active = 0, status = 'released', holds_seat = 0,
			instance_registered = 0, updated_at = ? WHERE id = ?`,
		`SELECT id, servers_used, servers_limit, agent_count, env_count, user_count,
			state, over_since, sampled_at FROM license_usage_samples ORDER BY sampled_at DESC LIMIT ? OFFSET ?`,
		`INSERT INTO license_usage_samples (id, license_id, servers_used, servers_limit,
			agent_count, env_count, user_count, state, over_since, reported, sampled_at,
			created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
	}
	for i, q := range stmts {
		if err := prepares(ctx, db, q); err != nil {
			return fmt.Errorf("STATEMENT %d FAILED: %w", i, err)
		}
	}

	var url, contact, hours string
	var apiKeys int
	for _, s := range []struct {
		query string
		dest  any
	}{
		{`SELECT value FROM settings WHERE key='license.server_url'`, &url},
		{`SELECT value FROM settings WHERE key='license.contact_url'`, &contact},
		{`SELECT value FROM settings WHERE key='license.heartbeat_interval_hours'`, &hours},
		{`SELECT COUNT(*) FROM settings WHERE key='license.api_key'`, &apiKeys},
	} {
		if err := setting(ctx, db, s.query, s.dest); err != nil {
			return err
		}
	}
	fmt.Printf("OK all statements prepare\nserver_url=%s contact_url=%s hours=%s api_key_rows=%d\n", url, contact, hours, apiKeys)
	return nil
}

func prepares(ctx context.Context, db *sql.DB, query string) error {
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer func() { closeutil.Report(stmt.Close(), "stmt") }()
	return nil
}
