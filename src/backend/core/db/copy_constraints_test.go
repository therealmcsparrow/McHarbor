// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// mssqlHardParameterLimit is SQL Server's RPC ceiling; error 8003 past it.
const mssqlHardParameterLimit = 2100

// TestRowsPerStatementFitsEveryBindBudget pins the INSERT sizing the bulk copy and
// the catch-up pass share: never past the destination's budget, never past 500 rows,
// and a caller's smaller BatchRows is honoured.
func TestRowsPerStatementFitsEveryBindBudget(t *testing.T) {
	if MaxBindParameters(DriverMSSQL) >= mssqlHardParameterLimit {
		t.Fatalf("MaxBindParameters(mssql) = %d, at or past SQL Server's %d", MaxBindParameters(DriverMSSQL), mssqlHardParameterLimit)
	}
	for _, driver := range []Driver{DriverSQLite, DriverPostgres, DriverMySQL, DriverMSSQL} {
		for cols := 1; cols <= 80; cols++ {
			for _, requested := range []int{0, 7, 5000} {
				rows := rowsPerStatement(driver, cols, requested)
				if rows < 1 || rows > maxRowsPerStatement || rows*cols > MaxBindParameters(driver) && rows > 1 {
					t.Fatalf("%s, %d cols, requested %d: %d rows (%d params)", driver, cols, requested, rows, rows*cols)
				}
				if requested == 7 && rows > 7 {
					t.Fatalf("%s, %d cols: requested 7 rows, got %d", driver, cols, rows)
				}
			}
		}
	}
}

// TestDeltaChunkRowsStaysUnderSQLServersLimit is the catch-up pass that used to size
// itself from MaxBindParameters alone: 13 columns into SQL Server went out as 500
// rows, 6500 parameters, and failed with 8003.
func TestDeltaChunkRowsStaysUnderSQLServersLimit(t *testing.T) {
	for _, pair := range [][2]Driver{{DriverSQLite, DriverMSSQL}, {DriverMSSQL, DriverSQLite}, {DriverMSSQL, DriverPostgres}, {DriverMSSQL, DriverMySQL}} {
		for cols := 1; cols <= 80; cols++ {
			chunk := deltaChunkRows(pair[0], pair[1], cols, 0)
			if chunk > MaxBindParameters(pair[0]) {
				t.Fatalf("%s -> %s: a %d-key IN list overflows the source", pair[0], pair[1], chunk)
			}
			if chunk*cols > MaxBindParameters(pair[1]) && chunk > 1 {
				t.Fatalf("%s -> %s, %d cols: %d rows is %d parameters", pair[0], pair[1], cols, chunk, chunk*cols)
			}
		}
	}
	if got := deltaChunkRows(DriverSQLite, DriverSQLite, 13, 9); got != 9 {
		t.Fatalf("BatchRows 9 was ignored: chunk %d", got)
	}
}

// TestSessionForeignKeyStatementsPerDriver keeps the three session switches as they
// were and gives SQL Server none, which is what stops the copier sending it a PRAGMA.
func TestSessionForeignKeyStatementsPerDriver(t *testing.T) {
	cases := map[Driver][2]string{
		DriverSQLite:   {"PRAGMA foreign_keys = OFF", "PRAGMA foreign_keys = ON"},
		DriverPostgres: {"SET session_replication_role = 'replica'", "SET session_replication_role = 'origin'"},
		DriverMySQL:    {"SET FOREIGN_KEY_CHECKS = 0", "SET FOREIGN_KEY_CHECKS = 1"},
	}
	for driver, want := range cases {
		off, on, ok := sessionForeignKeyStatements(driver)
		if !ok || off != want[0] || on != want[1] {
			t.Fatalf("%s: got (%q, %q, %v)", driver, off, on, ok)
		}
	}
	if off, on, ok := sessionForeignKeyStatements(DriverMSSQL); ok || off != "" || on != "" {
		t.Fatalf("mssql has no session switch, got (%q, %q, %v)", off, on, ok)
	}
}

// TestServerVersionQueryPerDriver guards the health check on SQL Server, which used
// to be asked sqlite_version().
func TestServerVersionQueryPerDriver(t *testing.T) {
	want := map[Driver]string{
		DriverSQLite:   "SELECT sqlite_version()",
		DriverPostgres: "SELECT version()",
		DriverMySQL:    "SELECT VERSION()",
		DriverMSSQL:    "SELECT @@VERSION",
	}
	for driver, query := range want {
		if got := serverVersionQuery(driver); got != query {
			t.Fatalf("%s: %q, want %q", driver, got, query)
		}
	}
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	}()
	version, err := ServerVersion(context.Background(), database, DriverSQLite)
	if err != nil || !strings.HasPrefix(version, "3.") {
		t.Fatalf("sqlite version %q, %v", version, err)
	}
}

// TestCopyBetweenSQLiteFilesSuspendsQuietly is the unchanged path: SQLite to SQLite,
// a child before its parent in the source, foreign keys relaxed, no warning logged.
func TestCopyBetweenSQLiteFilesSuspendsQuietly(t *testing.T) {
	dir := t.TempDir()
	open := func(name string) *sql.DB {
		database, err := sql.Open("sqlite", "file:"+filepath.Join(dir, name)+"?_pragma=foreign_keys(1)")
		if err != nil {
			t.Fatal(err)
		}
		database.SetMaxOpenConns(1)
		t.Cleanup(func() {
			if err := database.Close(); err != nil {
				t.Logf("closing database: %v", err)
			}
		})
		for _, stmt := range []string{
			`CREATE TABLE parents (id TEXT PRIMARY KEY, n INTEGER)`,
			`CREATE TABLE kids (id TEXT PRIMARY KEY, parent_id TEXT REFERENCES parents(id), note TEXT)`,
		} {
			if _, err := database.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
		return database
	}
	src, dst := open("src.db"), open("dst.db")
	if _, err := src.Exec(`INSERT INTO parents VALUES ('p1', 5000000000)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		if _, err := src.Exec(`INSERT INTO kids VALUES (?, 'p1', NULL)`, "k"+strings.Repeat("x", i)); err != nil {
			t.Fatal(err)
		}
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	report, err := CopyAll(context.Background(), src, DriverSQLite, dst, DriverSQLite, CopyOptions{BatchRows: 3, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ForeignKeysRelaxed || report.TotalRows != 41 || len(report.UntrustedTables) != 0 {
		t.Fatalf("report %+v", report)
	}
	if strings.Contains(logs.String(), "would not suspend") {
		t.Fatalf("a suspension warning was logged for SQLite:\n%s", logs.String())
	}

	if _, err := src.Exec(`INSERT INTO kids VALUES ('late', 'p1', 'x')`); err != nil {
		t.Fatal(err)
	}
	delta, err := CopyDelta(context.Background(), src, DriverSQLite, dst, DriverSQLite, DeltaOptions{BatchRows: 1, Logger: logger})
	if err != nil || delta.Total != 1 {
		t.Fatalf("delta %+v, %v", delta, err)
	}
	var big int64
	if err := dst.QueryRow(`SELECT n FROM parents WHERE id = 'p1'`).Scan(&big); err != nil || big != 5000000000 {
		t.Fatalf("parent value %d, %v", big, err)
	}
}
