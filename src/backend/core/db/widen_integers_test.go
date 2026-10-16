// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// bigValues are real magnitudes from the install that exposed the defect: a
// 424 GiB filesystem, 28 GiB of host memory, a 47 GiB backup, a 3 GiB upload.
var bigValues = struct {
	used, total, memUsed, memTotal, backup, debug int64
}{455438110720, 999999999999, 30064771072, 34359738368, 50465865728, 3221225472}

// TestBigIntegersRoundTripPostgres and TestBigIntegersRoundTripMySQL run the
// three halves of the int4 fix against a real server: a migrated schema has no
// 32-bit integer column left, migration 119 widens one an older install still
// has, and CopyAll carries >2^31 values from SQLite into it.
func TestBigIntegersRoundTripPostgres(t *testing.T) {
	bigIntegerRoundTrip(t, "MCHARBOR_TEST_PG_DSN", DriverPostgres)
}

func TestBigIntegersRoundTripMySQL(t *testing.T) {
	bigIntegerRoundTrip(t, "MCHARBOR_TEST_MYSQL_DSN", DriverMySQL)
}

func bigIntegerRoundTrip(t *testing.T, envVar string, driver Driver) {
	dsn := os.Getenv(envVar)
	if dsn == "" {
		t.Skipf("%s not set; skipping the %s round trip", envVar, driver)
	}
	ctx := context.Background()
	var dst *sql.DB
	var err error
	if driver == DriverPostgres {
		dst, err = openPostgres(dsn)
	} else {
		dst, err = openMySQL(dsn)
	}
	if err != nil {
		t.Fatalf("opening %s: %v", driver, err)
	}
	defer func() {
		if err := dst.Close(); err != nil {
			t.Logf("closing dst: %v", err)
		}
	}()
	if err := Migrate(dst, driver); err != nil {
		t.Fatalf("Migrate(%s): %v", driver, err)
	}

	cols, skipped, err := narrowIntegerColumns(ctx, dst, driver)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range append(cols, skipped...) {
		t.Errorf("%s.%s is still a 32-bit integer on %s", c.table, c.column, driver)
	}

	simulateOldInstall(ctx, t, dst, driver)
	if err := Migrate(dst, driver); err != nil {
		t.Fatalf("re-running Migrate after narrowing: %v", err)
	}
	cols, _, err = narrowIntegerColumns(ctx, dst, driver)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 0 {
		t.Fatalf("migration 119 left %d narrow columns on an older install: %v", len(cols), cols)
	}
	assertStillNotNullWithDefault(ctx, t, dst, driver)

	src := bigValueSource(t)
	defer func() {
		if err := src.Close(); err != nil {
			t.Logf("closing src: %v", err)
		}
	}()
	if _, err := CopyAll(ctx, src, DriverSQLite, dst, driver, CopyOptions{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}); err != nil {
		t.Fatalf("CopyAll sqlite → %s: %v", driver, err)
	}
	checks := []struct {
		query string
		want  int64
	}{
		{"SELECT used_bytes FROM capacity_samples WHERE id = 'cs1'", bigValues.used},
		{"SELECT total_bytes FROM capacity_samples WHERE id = 'cs1'", bigValues.total},
		{"SELECT memory_used FROM host_metrics WHERE id = 'hm1'", bigValues.memUsed},
		{"SELECT memory_total FROM host_metrics WHERE id = 'hm1'", bigValues.memTotal},
		{"SELECT size_bytes FROM instance_backup_exports WHERE id = 'ib1'", bigValues.backup},
		{"SELECT size_bytes FROM container_debug_sessions WHERE id = 'ds1'", bigValues.debug},
	}
	for _, c := range checks {
		var got int64
		if err := dst.QueryRowContext(ctx, c.query).Scan(&got); err != nil {
			t.Errorf("%s: %v", c.query, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s = %d, want %d", c.query, got, c.want)
		}
	}
	if _, err := dst.ExecContext(ctx, "UPDATE capacity_samples SET bucket_seconds = 1099511627776 WHERE id = 'cs1'"); err != nil {
		t.Errorf("writing 2^40 into a column 119 widened: %v", err)
	}
}

// simulateOldInstall narrows capacity_samples.used_bytes back to the 32-bit
// type an install migrated before this change carries, and forgets 119, so the
// next Migrate has to widen a real column.
func simulateOldInstall(ctx context.Context, t *testing.T, database *sql.DB, driver Driver) {
	t.Helper()
	if _, err := database.ExecContext(ctx, "DELETE FROM capacity_samples"); err != nil {
		t.Fatal(err)
	}
	narrow := `ALTER TABLE capacity_samples ALTER COLUMN used_bytes TYPE integer, ALTER COLUMN bucket_seconds TYPE integer`
	if driver == DriverMySQL {
		narrow = "ALTER TABLE capacity_samples MODIFY COLUMN used_bytes INT NOT NULL DEFAULT 0, MODIFY COLUMN bucket_seconds INT NOT NULL DEFAULT 3600"
	}
	if _, err := database.ExecContext(ctx, narrow); err != nil {
		t.Fatalf("narrowing for the older-install case: %v", err)
	}
	if _, err := database.ExecContext(ctx, "DELETE FROM schema_migrations WHERE name = '119_widen_integer_columns.sql'"); err != nil {
		t.Fatal(err)
	}
	cols, _, err := narrowIntegerColumns(ctx, database, driver)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 2 {
		t.Fatalf("the simulated older install has %d narrow columns, want 2", len(cols))
	}
}

// assertStillNotNullWithDefault checks the widening kept what MySQL's MODIFY
// would silently drop: NOT NULL and the column default.
func assertStillNotNullWithDefault(ctx context.Context, t *testing.T, database *sql.DB, driver Driver) {
	t.Helper()
	q := `SELECT is_nullable, column_default FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'capacity_samples' AND column_name = 'bucket_seconds'`
	if driver == DriverMySQL {
		q = `SELECT IS_NULLABLE, COLUMN_DEFAULT FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'capacity_samples' AND COLUMN_NAME = 'bucket_seconds'`
	}
	var nullable string
	var dflt sql.NullString
	if err := database.QueryRowContext(ctx, q).Scan(&nullable, &dflt); err != nil {
		t.Fatal(err)
	}
	if nullable != "NO" || !dflt.Valid || dflt.String != "3600" {
		t.Errorf("bucket_seconds after widening: nullable=%s default=%v, want NO and 3600", nullable, dflt)
	}
}

// bigValueSource is a migrated SQLite database holding one row per byte-count
// column that the defect report named.
func bigValueSource(t *testing.T) *sql.DB {
	t.Helper()
	src, err := openSQLite(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(src, DriverSQLite); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	stmts := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO environments (id, name) VALUES ('e1', 'homelab')`, nil},
		{`INSERT INTO capacity_samples (id, environment_id, subject_kind, subject_id, sampled_at, used_bytes, total_bytes)
			VALUES ('cs1', 'e1', 'rootfs', '/', ?, ?, ?)`, []any{now, bigValues.used, bigValues.total}},
		{`INSERT INTO host_metrics (id, environment_id, memory_used, memory_total) VALUES ('hm1', 'e1', ?, ?)`,
			[]any{bigValues.memUsed, bigValues.memTotal}},
		{`INSERT INTO instance_backup_exports (id, size_bytes) VALUES ('ib1', ?)`, []any{bigValues.backup}},
		{`INSERT INTO container_debug_sessions (id, container_id, path, arch, size_bytes, expires_at, created_at, updated_at)
			VALUES ('ds1', 'c1', '/tmp/busybox', 'amd64', ?, ?, ?, ?)`, []any{bigValues.debug, now, now, now}},
	}
	for _, s := range stmts {
		if _, err := src.Exec(s.q, s.args...); err != nil {
			t.Fatalf("seeding the source: %v\n%s", err, s.q)
		}
	}
	return src
}
