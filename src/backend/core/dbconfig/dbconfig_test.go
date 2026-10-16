// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dbconfig

import (
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
)

func TestResolveEnvDriver(t *testing.T) {
	tests := []struct {
		value string
		want  db.Driver
	}{
		{value: "", want: db.DriverSQLite},
		{value: "sqlite", want: db.DriverSQLite},
		{value: "sqlite3", want: db.DriverSQLite},
		{value: "pgsql", want: db.DriverPostgres},
		{value: "postgres", want: db.DriverPostgres},
		{value: "postgresql", want: db.DriverPostgres},
		{value: "mysql", want: db.DriverMySQL},
		{value: "MariaDB", want: db.DriverMySQL},
		{value: "mssql", want: db.DriverMSSQL},
		{value: " sqlserver ", want: db.DriverMSSQL},
		{value: "azuresql", want: db.DriverMSSQL},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("MCHARBOR_DB_DRIVER", tt.value)
			dir := t.TempDir()
			cfg := &config.Config{DataDir: dir, DBDriver: tt.value, DBDSN: "dsn", DatabasePath: dir + "/mcharbor.db"}

			got, source, err := Resolve(cfg, nil)
			if err != nil {
				t.Fatalf("Resolve() with MCHARBOR_DB_DRIVER=%q: %v", tt.value, err)
			}
			if got.Driver != tt.want || source != SourceEnv {
				t.Fatalf("Resolve() with MCHARBOR_DB_DRIVER=%q = (%q, %q), want (%q, %q)",
					tt.value, got.Driver, source, tt.want, SourceEnv)
			}
		})
	}
}

func TestResolveRefusesUnknownEnvDriver(t *testing.T) {
	for _, value := range []string{"oracle", "postgress", "sql-server"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("MCHARBOR_DB_DRIVER", value)
			dir := t.TempDir()
			cfg := &config.Config{DataDir: dir, DBDriver: value, DatabasePath: dir + "/mcharbor.db"}

			got, _, err := Resolve(cfg, nil)
			if err == nil {
				t.Fatalf("Resolve() with MCHARBOR_DB_DRIVER=%q opened %q, want a refusal", value, got.Driver)
			}
			if !strings.Contains(err.Error(), value) {
				t.Fatalf("refusal %q does not name the value %q", err, value)
			}
		})
	}
}
