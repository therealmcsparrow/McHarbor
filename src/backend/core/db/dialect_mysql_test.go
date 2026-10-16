// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"strings"
	"testing"
)

// TestMySQLMigrationQuotesLong guards the first failure of a real MySQL 8.4 and
// MariaDB 11 run (2026-09-27): 100_plugin_cli.sql names a column `long`, which both
// servers reserve, and the whole migration set stopped there.
func TestMySQLMigrationQuotesLong(t *testing.T) {
	got := TranslateStatement(DriverMySQL, "CREATE TABLE IF NOT EXISTS plugin_cli_commands (\n    id TEXT PRIMARY KEY,\n    long TEXT NOT NULL DEFAULT '',\n    example TEXT NOT NULL DEFAULT ''\n);")
	if !strings.Contains(got, "`long`") {
		t.Fatalf("long is not quoted:\n%s", got)
	}
}
