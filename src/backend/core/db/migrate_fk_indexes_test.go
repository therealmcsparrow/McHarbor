// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"database/sql"
	"os"
	"regexp"
	"testing"
)

var fkIndexStmt = regexp.MustCompile(`(?m)^CREATE INDEX IF NOT EXISTS (\w+) ON (\w+)\(`)

// TestMigration124IndexesExist migrates an empty server database and asserts
// every index migration 124 declares is present in the dialect's own catalog.
func TestMigration124IndexesExist(t *testing.T) {
	cases := []struct {
		env    string
		driver Driver
		query  string
	}{
		{"MCHARBOR_TEST_PG_DSN", DriverPostgres, `SELECT COUNT(*) FROM pg_indexes WHERE tablename = $1 AND indexname = $2`},
		{"MCHARBOR_TEST_MYSQL_DSN", DriverMySQL, `SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`},
		{"MCHARBOR_TEST_MARIADB_DSN", DriverMySQL, `SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`},
		{"MCHARBOR_TEST_MSSQL_DSN", DriverMSSQL, `SELECT COUNT(*) FROM sys.indexes i JOIN sys.tables t ON t.object_id = i.object_id WHERE t.name = @p1 AND i.name = @p2`},
	}

	src, err := os.ReadFile("migrations/124_foreign_key_indexes.sql")
	if err != nil {
		t.Fatalf("reading migration 124: %v", err)
	}
	stmts := fkIndexStmt.FindAllStringSubmatch(string(src), -1)
	if len(stmts) == 0 {
		t.Fatal("no CREATE INDEX statements parsed from migration 124")
	}

	for _, c := range cases {
		t.Run(c.env, func(t *testing.T) {
			dsn := os.Getenv(c.env)
			if dsn == "" {
				t.Skipf("%s not set", c.env)
			}
			var (
				database *sql.DB
				err      error
			)
			switch c.driver {
			case DriverPostgres:
				database, err = openPostgres(dsn)
			case DriverMySQL:
				database, err = openMySQL(dsn)
			default:
				database, err = openMSSQL(dsn)
			}
			if err != nil {
				t.Fatalf("opening %s: %v", c.driver, err)
			}
			defer func() {
				if err := database.Close(); err != nil {
					t.Logf("closing database: %v", err)
				}
			}()

			if err := Migrate(database, c.driver); err != nil {
				t.Fatalf("Migrate(%s): %v", c.driver, err)
			}
			for _, s := range stmts {
				var n int
				if err := database.QueryRow(c.query, s[2], s[1]).Scan(&n); err != nil {
					t.Fatalf("catalog query for %s: %v", s[1], err)
				}
				if n == 0 {
					t.Errorf("index %s on %s missing after migrations", s[1], s[2])
				}
			}
			t.Logf("%d indexes from migration 124 verified on %s", len(stmts), c.env)
		})
	}
}
