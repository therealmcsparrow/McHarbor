// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package database exposes the SQL backend McHarbor runs on: which one it
// is, whether another one would work, and how to move every row to it.
//
// The three drivers and the portable migration set already existed; what
// this module adds is the part an operator can reach without editing a
// Compose file — a tested connection form, an encrypted credential store,
// and a copy with a progress readout.
package database

import "time"

// Driver names as the API speaks them. They map onto core/db's Driver
// through dbconfig.NormalizeDriver, which also accepts the aliases
// ("postgresql", "mariadb", "sqlite3", "sqlserver") an operator is likely to type.
const (
	driverSQLite   = "sqlite"
	driverPostgres = "postgres"
	driverMySQL    = "mysql"
	driverMSSQL    = "mssql"
)

// ConnectionInput is a target database as the form describes it.
//
// Two shapes are accepted. The structured one (Host / Port / Name / User /
// Password / SSLMode) is what the form fills in, and is what the UI can
// validate field by field. The raw one (DSN) is the escape hatch for a
// managed database whose connection string carries options the form does
// not model — a Neon endpoint parameter, a client certificate path, a
// non-default search_path. When DSN is set, every structured field is
// ignored.
type ConnectionInput struct {
	Driver   string `json:"driver"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Name     string `json:"name,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	// SSLMode is the libpq sslmode on Postgres (disable / require /
	// verify-ca / verify-full), the go-sql-driver tls parameter on
	// MySQL (false / true / skip-verify / preferred), and the go-mssqldb
	// encrypt parameter on SQL Server (true / strict / false / disable).
	SSLMode string `json:"sslMode,omitempty"`
	// TrustServerCertificate is SQL Server only: it keeps the connection
	// encrypted but skips verifying the certificate, which is what a default
	// SQL Server install with its self-signed certificate needs.
	TrustServerCertificate bool `json:"trustServerCertificate,omitempty"`
	// Params are extra DSN query parameters, merged after the defaults.
	Params map[string]string `json:"params,omitempty"`
	// DSN, when set, is used verbatim instead of the structured fields.
	DSN string `json:"dsn,omitempty"`
	// Path is the SQLite file path. Empty means the configured default.
	Path string `json:"path,omitempty"`
}

// ConfigResponse describes the database McHarbor is currently running on.
// It never carries a password: Target is the redacted DSN.
type ConfigResponse struct {
	// Driver is the active driver: sqlite, postgres, mysql or mssql. The
	// picker opens on it, marked as in use.
	Driver string `json:"driver"`
	// Source is where the connection came from: "env", "secret" or
	// "default". See core/dbconfig.
	Source string `json:"source"`
	// Target is the redacted connection string, or the SQLite file path.
	Target string `json:"target"`
	// EnvPinned is true when MCHARBOR_DB_DRIVER is set in the process
	// environment. The UI turns read-only, because a value written to
	// database.json would be ignored on the next boot and the operator
	// would be looking at a setting that does nothing.
	EnvPinned bool `json:"envPinned"`
	// ServerVersion is the backend's own version banner.
	ServerVersion string `json:"serverVersion,omitempty"`
	// Tables and Rows are a size readout, so the operator has a sense of
	// how long a migration will take before starting one.
	Tables int   `json:"tables"`
	Rows   int64 `json:"rows"`
	// SizeBytes is the SQLite file size. Zero on the network drivers,
	// where the number would be a server-side statistic McHarbor has no
	// business asking for.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// Configured is true once a connection secret has been written. The
	// first-run wizard uses it as its "the operator has chosen" marker.
	Configured bool `json:"configured"`
	// UpdatedAt is when the secret was last written.
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	// AvailableDrivers is the list the picker renders.
	AvailableDrivers []string `json:"availableDrivers"`
	// RestartSupported is true when McHarbor can restart itself through a
	// local Docker socket. When false the UI tells the operator to restart
	// the container by hand after the migration.
	RestartSupported bool `json:"restartSupported"`
}

// Step statuses. A step that fails stops the run; a step that warns does
// not — a target that already holds McHarbor tables is the expected state
// when retrying a migration that failed halfway.
const (
	stepPass = "pass"
	stepWarn = "warn"
	stepFail = "fail"
	stepSkip = "skip"
)

// TestStep is one line of the step-by-step connection test.
type TestStep struct {
	// ID is the step's stable name: validate, resolve, connect, version,
	// privileges, schema, encoding. The UI translates it.
	ID string `json:"id"`
	// Status is pass, warn, fail or skip.
	Status string `json:"status"`
	// Code is a machine-readable i18n code for a failure, so the UI can
	// translate the common causes rather than showing a driver string.
	Code string `json:"code,omitempty"`
	// Detail is a human-readable note. On failure it carries the driver's
	// own message with every credential redacted — a connection tester
	// that can only say "it failed" sends the operator to the server logs,
	// which is exactly what this screen exists to avoid.
	Detail string `json:"detail,omitempty"`
	// DurationMS is how long the step took.
	DurationMS int64 `json:"durationMs"`
}

// TestResponse is the whole step-by-step run.
type TestResponse struct {
	OK            bool       `json:"ok"`
	Driver        string     `json:"driver"`
	Target        string     `json:"target"`
	ServerVersion string     `json:"serverVersion,omitempty"`
	Steps         []TestStep `json:"steps"`
	// ExistingTables is how many McHarbor tables the target already holds.
	ExistingTables int `json:"existingTables"`
}

// Migration phases, in the order they run.
const (
	phaseQueued    = "queued"
	phasePrepare   = "prepare"
	phaseSchema    = "schema"
	phaseInventory = "inventory"
	phaseCopy      = "copy"
	phaseVerify    = "verify"
	phaseCatchUp   = "catchup"
	phasePersist   = "persist"
	phaseRestart   = "restart"
	phaseDone      = "done"
	phaseFailed    = "failed"
	phaseCancelled = "cancelled"
)

// MigrationRequest starts a migration.
type MigrationRequest struct {
	Target ConnectionInput `json:"target"`
	// RestartAfter schedules the container restart that actually puts the
	// new database into service. Defaults to true; an operator running
	// McHarbor outside Docker turns it off and restarts by hand.
	RestartAfter *bool `json:"restartAfter,omitempty"`
}

// MigrationTable is one table's line in the progress readout.
type MigrationTable struct {
	Table      string `json:"table"`
	Rows       int64  `json:"rows"`
	Total      int64  `json:"total"`
	DurationMS int64  `json:"durationMs"`
	Done       bool   `json:"done"`
}

// MigrationStatus is the whole job as the progress modal reads it.
type MigrationStatus struct {
	ID string `json:"id"`
	// Running is true from the moment the job starts until it reaches a
	// terminal phase.
	Running bool `json:"running"`
	// Phase is one of the phase constants above.
	Phase string `json:"phase"`
	// FromDriver / ToDriver frame the modal's header.
	FromDriver string `json:"fromDriver"`
	ToDriver   string `json:"toDriver"`
	ToTarget   string `json:"toTarget"`
	// CurrentTable is the table being copied right now.
	CurrentTable string `json:"currentTable,omitempty"`
	// TableIndex is 1-based within TableTotal.
	TableIndex int `json:"tableIndex"`
	TableTotal int `json:"tableTotal"`
	// RowsCopied / RowsTotal are the whole-run counters the progress bar
	// is drawn from.
	RowsCopied int64 `json:"rowsCopied"`
	RowsTotal  int64 `json:"rowsTotal"`
	// RowsCaughtUp counts the rows the catch-up passes moved after the bulk
	// copy: writes that landed on the source between a table being read and
	// the migration finishing. Normally zero, because the background workers
	// are held still for the duration.
	RowsCaughtUp int64 `json:"rowsCaughtUp"`
	// DeltaPass is which catch-up pass is running, 1-based.
	DeltaPass int `json:"deltaPass,omitempty"`
	// Tables fills in as the copy proceeds.
	Tables []MigrationTable `json:"tables"`
	// Steps is the pre-flight test the job re-runs before touching
	// anything, so the modal can show why a migration refused to start.
	Steps []TestStep `json:"steps,omitempty"`
	// Warnings names the non-fatal findings: tables that drifted while the
	// copy ran, columns present on one side only, a destination that would
	// not suspend foreign key checks.
	Warnings []string `json:"warnings,omitempty"`
	// Error is set on a failed job: an i18n code plus a redacted detail.
	Error       string `json:"error,omitempty"`
	ErrorDetail string `json:"errorDetail,omitempty"`
	// RestartScheduled is true once the container restart has been asked
	// for; RestartRequired is true when it could not be and the operator
	// has to do it themselves.
	RestartScheduled bool       `json:"restartScheduled"`
	RestartRequired  bool       `json:"restartRequired"`
	StartedAt        time.Time  `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
}

// SetupStatusResponse drives the first-run database screen. It is served
// without authentication — no account exists yet — and therefore says as
// little as possible: which driver is active, whether a choice has been
// made, and whether the environment already decided.
type SetupStatusResponse struct {
	// NeedsSetup mirrors /auth/status: true while no account exists.
	NeedsSetup bool `json:"needsSetup"`
	// Configured is true once a connection secret has been written.
	Configured bool `json:"configured"`
	// EnvPinned is true when MCHARBOR_DB_DRIVER decided already, in which
	// case the wizard skips the database step entirely.
	EnvPinned        bool     `json:"envPinned"`
	Driver           string   `json:"driver"`
	AvailableDrivers []string `json:"availableDrivers"`
	RestartSupported bool     `json:"restartSupported"`
}

// SetupApplyResponse is the answer to the first-run apply. There is no data
// to copy on a fresh install, so the whole operation is "write the secret,
// then restart into it".
type SetupApplyResponse struct {
	Driver string `json:"driver"`
	Target string `json:"target"`
	// RestartScheduled is true when McHarbor asked Docker to restart it.
	RestartScheduled bool `json:"restartScheduled"`
	// RestartRequired is true when it could not, and the operator has to
	// restart the container themselves before the choice takes effect.
	RestartRequired bool `json:"restartRequired"`
}

// CleanupStatus describes the old SQLite file a completed migration left on
// disk, and whether McHarbor is willing to delete it.
//
// Only SQLite is ever offered. A PostgreSQL or MySQL database McHarbor
// migrated away from belongs to whoever administers that server; dropping it
// is their decision and their tooling, and a button here would be McHarbor
// destroying data on a machine it does not own.
type CleanupStatus struct {
	// Available is true when there is a file to delete and deleting it is
	// safe — the migration completed, the path is not the live database, and
	// the file is still there.
	Available bool `json:"available"`
	// Path is the old SQLite file. Empty when Available is false.
	Path string `json:"path,omitempty"`
	// Files lists the paths that would be removed: the database and its
	// write-ahead log and shared-memory siblings.
	Files []CleanupFile `json:"files,omitempty"`
	// TotalBytes is the disk space the deletion would reclaim.
	TotalBytes int64 `json:"totalBytes"`
	// MigratedAt is when the migration that left this file ran.
	MigratedAt *time.Time `json:"migratedAt,omitempty"`
	// Reason names why the action is unavailable, for the UI to explain
	// rather than silently render nothing.
	Reason string `json:"reason,omitempty"`
}

// CleanupFile is one file the cleanup would remove.
type CleanupFile struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// CleanupResult reports what was actually deleted.
type CleanupResult struct {
	Removed []string `json:"removed"`
	Bytes   int64    `json:"bytes"`
}
