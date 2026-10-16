// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"database/sql"
	"regexp"
	"strings"
	"testing"
)

var narrowIntegerRE = regexp.MustCompile(`(?i)\bINTEGER\b`)

// TestMigrationsEmitNoNarrowInteger guards the int4 defect at its source: no
// migration statement reaches Postgres or MySQL with an INTEGER type, since both
// read it as 32 bits and the schema's contract is SQLite's 64-bit INTEGER.
func TestMigrationsEmitNoNarrowInteger(t *testing.T) {
	names, err := MigrationNames()
	if err != nil {
		t.Fatal(err)
	}
	for _, driver := range []Driver{DriverPostgres, DriverMySQL, DriverMSSQL} {
		for _, name := range names {
			text, err := MigrationSQL(name)
			if err != nil {
				t.Fatal(err)
			}
			for _, stmt := range SplitStatements(text) {
				switch ClassifyStatement(driver, stmt) {
				case SkipStatement, SkipStatementLoudly:
					continue
				}
				out := stripComments(TranslateStatement(driver, stmt))
				if loc := narrowIntegerRE.FindStringIndex(out); loc != nil {
					t.Errorf("%s / %s still declares INTEGER: …%s…", driver, name, out[max(0, loc[0]-40):min(len(out), loc[1]+20)])
				}
			}
		}
	}
}

// TestIntegerTranslation pins the individual rewrites, including the serial
// key and a reserved column name MySQL still has to quote after the widening.
func TestIntegerTranslation(t *testing.T) {
	cases := []struct {
		driver   Driver
		in, want string
	}{
		{DriverPostgres, "CREATE TABLE t (\n    id TEXT PRIMARY KEY,\n    used_bytes INTEGER NOT NULL DEFAULT 0\n);", "used_bytes BIGINT NOT NULL DEFAULT 0"},
		{DriverPostgres, "CREATE TABLE t (\n    seq INTEGER PRIMARY KEY AUTOINCREMENT,\n    n INTEGER\n);", "seq BIGSERIAL PRIMARY KEY"},
		{DriverPostgres, "ALTER TABLE host_metrics ADD COLUMN net_rx INTEGER;", "ADD COLUMN net_rx BIGINT"},
		{DriverPostgres, "UPDATE t SET n = CAST(x AS INTEGER);", "CAST(x AS BIGINT)"},
		{DriverMySQL, "CREATE TABLE t (\n    id TEXT PRIMARY KEY,\n    size_bytes INTEGER NOT NULL DEFAULT 0\n);", "size_bytes BIGINT NOT NULL DEFAULT 0"},
		{DriverMySQL, "CREATE TABLE t (\n    id TEXT PRIMARY KEY,\n    long INTEGER NOT NULL DEFAULT 0\n);", "`long` BIGINT NOT NULL"},
		{DriverMySQL, "UPDATE t SET n = CAST(x AS INTEGER);", "CAST(x AS SIGNED)"},
		{DriverMySQL, "CREATE TABLE t (\n    seq INTEGER PRIMARY KEY AUTOINCREMENT\n);", "seq BIGINT PRIMARY KEY AUTO_INCREMENT"},
	}
	for _, c := range cases {
		if got := TranslateStatement(c.driver, c.in); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q\n got %q\nwant it to contain %q", c.driver, c.in, got, c.want)
		}
	}
	if got := TranslateStatement(DriverSQLite, "CREATE TABLE t (n INTEGER);"); got != "CREATE TABLE t (n INTEGER);" {
		t.Errorf("SQLite must be left byte-for-byte alone, got %q", got)
	}
}

// TestWidenTableStatement pins the ALTER each engine gets: Postgres changes the
// type alone, MySQL restates nullability, default and AUTO_INCREMENT because its
// MODIFY would otherwise drop them.
func TestWidenTableStatement(t *testing.T) {
	cols := []integerColumn{
		{table: "capacity_samples", column: "used_bytes", dflt: sql.NullString{String: "0", Valid: true}},
		{table: "capacity_samples", column: "note_len", nullable: true},
		{table: "capacity_samples", column: "seq", extra: "auto_increment", colType: "int unsigned"},
		{table: "capacity_samples", column: "stamp", extra: "DEFAULT_GENERATED", dflt: sql.NullString{String: "unix_timestamp()", Valid: true}},
	}
	pg := widenTableStatement(DriverPostgres, "capacity_samples", cols[:2])
	if pg != `ALTER TABLE "capacity_samples" ALTER COLUMN "used_bytes" TYPE BIGINT, ALTER COLUMN "note_len" TYPE BIGINT` {
		t.Errorf("postgres: %s", pg)
	}
	my := widenTableStatement(DriverMySQL, "capacity_samples", cols)
	want := "ALTER TABLE `capacity_samples` MODIFY COLUMN `used_bytes` BIGINT NOT NULL DEFAULT 0, " +
		"MODIFY COLUMN `note_len` BIGINT, " +
		"MODIFY COLUMN `seq` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, " +
		"MODIFY COLUMN `stamp` BIGINT NOT NULL DEFAULT (unix_timestamp())"
	if my != want {
		t.Errorf("mysql:\n got %s\nwant %s", my, want)
	}
	if got := mysqlDefault("it's", ""); got != "'it''s'" {
		t.Errorf("a text default is quoted and escaped, got %s", got)
	}
}
