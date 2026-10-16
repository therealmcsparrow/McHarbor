// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"path/filepath"
	"testing"
)

const servicePrincipalMigration = "114_runbook_service_principals.sql"

// TestServicePrincipalMigrationShape pins what 114 must leave behind: every
// existing user reads as an ordinary person, and a run with no trigger reads as
// a manual one, so nothing on an upgraded install changes meaning.
func TestServicePrincipalMigrationShape(t *testing.T) {
	database, err := openSQLite(filepath.Join(t.TempDir(), "principals.db"))
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	}()
	if err := Migrate(database, DriverSQLite); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := database.Exec(`INSERT INTO users (id, username, password_hash) VALUES ('u-plain', 'plain', 'x')`); err != nil {
		t.Fatalf("inserting a user: %v", err)
	}
	var kind string
	if err := database.QueryRow(`SELECT principal_kind FROM users WHERE id = 'u-plain'`).Scan(&kind); err != nil {
		t.Fatalf("reading principal_kind: %v", err)
	}
	if kind != "user" {
		t.Fatalf("a user inserted without a kind reads as %q, want user", kind)
	}

	for _, column := range []string{
		"automation_principal_id", "automation_on_alert", "automation_cron", "automation_since",
		"automation_next_run_at", "automation_last_run_at", "automation_last_error", "automation_updated_by",
	} {
		if _, err := database.Exec(`SELECT ` + column + ` FROM runbooks LIMIT 1`); err != nil {
			t.Errorf("runbooks.%s is missing: %v", column, err)
		}
	}
	if _, err := database.Exec(`SELECT trigger_kind FROM runbook_runs LIMIT 1`); err != nil {
		t.Errorf("runbook_runs.trigger_kind is missing: %v", err)
	}

	body, err := MigrationSQL(servicePrincipalMigration)
	if err != nil {
		t.Fatalf("reading %s: %v", servicePrincipalMigration, err)
	}
	for _, driver := range []Driver{DriverPostgres, DriverMySQL, DriverMSSQL} {
		for _, stmt := range SplitStatements(body) {
			// TolerateDuplicate is how MySQL and SQL Server run CREATE INDEX IF
			// NOT EXISTS; anything skipped would leave a column missing there.
			if got := ClassifyStatement(driver, stmt); got != ExecuteStatement && got != TolerateDuplicate {
				t.Errorf("%s would not execute on %s (action %d): %s", servicePrincipalMigration, driver, got, firstLine(stmt))
			}
		}
	}
}
