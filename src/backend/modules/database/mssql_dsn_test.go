// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"errors"
	"slices"
	"testing"
)

// TestBuildDSNForSQLServerFields pins the URL the form produces, because SameAsCurrent
// compares DSNs byte for byte and the password must survive URL escaping.
func TestBuildDSNForSQLServerFields(t *testing.T) {
	got, err := BuildDSN(ConnectionInput{
		Driver: "mssql", Host: "db.local", Name: "mcharbor", User: "app", Password: "p@ss:w/rd",
		TrustServerCertificate: true,
	})
	if err != nil {
		t.Fatalf("BuildDSN: %v", err)
	}
	want := "sqlserver://app:p%40ss%3Aw%2Frd@db.local:1433?TrustServerCertificate=true&database=mcharbor&encrypt=true"
	if got != want {
		t.Fatalf("BuildDSN:\n got %s\nwant %s", got, want)
	}

	got, err = BuildDSN(ConnectionInput{Driver: "sqlserver", Host: "db", Port: 14330, Name: "mch", User: "u", SSLMode: "strict"})
	if err != nil {
		t.Fatalf("BuildDSN with alias: %v", err)
	}
	if want := "sqlserver://u:@db:14330?database=mch&encrypt=strict"; got != want {
		t.Fatalf("BuildDSN:\n got %s\nwant %s", got, want)
	}
}

// TestBuildDSNRejectsIncompleteSQLServerInput covers every shape boot would refuse or
// that would land McHarbor's tables in a system database.
func TestBuildDSNRejectsIncompleteSQLServerInput(t *testing.T) {
	cases := map[string]ConnectionInput{
		"no host":         {Driver: "mssql", Name: "mch", User: "u"},
		"no database":     {Driver: "mssql", Host: "db", User: "u"},
		"no user":         {Driver: "mssql", Host: "db", Name: "mch"},
		"bad port":        {Driver: "mssql", Host: "db", Name: "mch", User: "u", Port: 70000},
		"bad encrypt":     {Driver: "mssql", Host: "db", Name: "mch", User: "u", SSLMode: "require"},
		"system database": {Driver: "mssql", Host: "db", Name: "master", User: "u"},
		"raw odbc form":   {Driver: "mssql", DSN: "server=db;user id=u;password=p;database=mch"},
		"raw no database": {Driver: "mssql", DSN: "sqlserver://u:p@db:1433?encrypt=true"},
		"raw master":      {Driver: "mssql", DSN: "sqlserver://u:p@db:1433?database=MASTER"},
		"raw no host":     {Driver: "mssql", DSN: "sqlserver:///?database=mch"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildDSN(in); !errors.Is(err, errInvalidConfig) {
				t.Fatalf("BuildDSN(%+v) = %v, want errInvalidConfig", in, err)
			}
		})
	}
}

// TestBuildDSNKeepsAValidSQLServerURL keeps the escape hatch verbatim once it validates.
func TestBuildDSNKeepsAValidSQLServerURL(t *testing.T) {
	raw := "sqlserver://u:p@db:1433?Database=mch&encrypt=true&app+name=x"
	got, err := BuildDSN(ConnectionInput{Driver: "mssql", DSN: "  " + raw + " "})
	if err != nil || got != raw {
		t.Fatalf("BuildDSN = %q, %v; want the URL verbatim", got, err)
	}
}

// TestSQLServerIsOfferedAndDialsItsOwnPort guards the picker list and the reachability
// step, which used to fall back to the MySQL port for anything that was not Postgres.
func TestSQLServerIsOfferedAndDialsItsOwnPort(t *testing.T) {
	if !slices.Contains(AvailableDrivers(), driverMSSQL) {
		t.Fatalf("AvailableDrivers() = %v, want mssql listed", AvailableDrivers())
	}
	if addr, ok := hostPort(ConnectionInput{Driver: "mssql", Host: "db"}); !ok || addr != "db:1433" {
		t.Fatalf("hostPort(fields) = %q %v", addr, ok)
	}
	if addr, ok := hostPort(ConnectionInput{Driver: "mssql", DSN: "sqlserver://u:p@db?database=mch"}); !ok || addr != "db:1433" {
		t.Fatalf("hostPort(dsn) = %q %v", addr, ok)
	}
}

// TestCheckVersionForSQLServer reads the engine build, not the marketing year.
func TestCheckVersionForSQLServer(t *testing.T) {
	if err := checkVersion(driverMSSQL, "Microsoft SQL Server 2022 (RTM-CU27) (KB5104824) - 16.0.4295.3 (X64)"); err != nil {
		t.Fatalf("2022 refused: %v", err)
	}
	if err := checkVersion(driverMSSQL, "Microsoft SQL Server 2019 (RTM-CU22) (KB5027702) - 15.0.4322.2 (X64)"); err == nil {
		t.Fatal("2019 accepted; the migration set is verified on 2022 only")
	}
}
