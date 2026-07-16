// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

// Driver identifies which SQL backend Open() selected. The
// rest of McHarbor code is database-agnostic (uses database/sql),
// so this type is mostly informational: the coordinator uses it
// to decide which named-lock primitive to call (pg_try_advisory_lock
// on Postgres, GET_LOCK on MySQL, no-op on SQLite).
type Driver string

const (
	// DriverSQLite is the embedded single-file database used by
	// single-node installs. The default when MCHARBOR_DB_DRIVER
	// is unset. WAL mode, foreign keys, and busy timeouts are
	// applied at Open().
	DriverSQLite Driver = "sqlite"
	// DriverPostgres is the external database used for active-
	// active multi-node deployments. The DSN is taken from
	// MCHARBOR_DB_DSN (libpq-style URL, e.g. postgres://user:pw@host:5432/db).
	// Connection pool sizing, statement timeouts, and search_path
	// are applied at Open().
	DriverPostgres Driver = "postgres"
	// DriverMySQL covers MySQL 8+ and MariaDB 10.6+ for active-
	// active multi-node deployments. The DSN is the
	// go-sql-driver/mysql DSN format, e.g.
	// `user:pass@tcp(host:3306)/db?parseTime=true&loc=UTC&tls=true`.
	// Connection pool sizing mirrors Postgres; named locks use
	// MySQL's GET_LOCK / RELEASE_LOCK per-connection primitives.
	DriverMySQL Driver = "mysql"
)

// String renders the driver name in logs and the /api/cluster/status
// payload.
func (d Driver) String() string { return string(d) }
