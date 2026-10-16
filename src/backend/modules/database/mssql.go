// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/sqldialect"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// SQL Server specifics for the settings page: building and checking a connection.
// What the copier needs (batch sizing, constraint suspension, identity inserts,
// introspection) lives in core/db beside the other drivers' versions of it.

// minMSSQLMajor is SQL Server 2022, the only release the migration set is verified on.
const minMSSQLMajor = 16

// mssqlXactAbort is the XACT_ABORT bit of @@OPTIONS, the only place it can be read.
const mssqlXactAbort = 16384

// mssqlEncryptModes is the go-mssqldb `encrypt` vocabulary the form offers.
var mssqlEncryptModes = map[string]bool{"true": true, "strict": true, "false": true, "disable": true}

// mssqlSystemDatabases would receive McHarbor's tables when a connection string names
// no database, because the login's default database is almost always master.
var mssqlSystemDatabases = map[string]bool{"master": true, "tempdb": true, "model": true, "msdb": true}

// mssqlBuild reads the engine build out of an @@VERSION banner such as
// "Microsoft SQL Server 2022 (RTM-CU14) (KB5038325) - 16.0.4135.4 (X64)".
var mssqlBuild = regexp.MustCompile(`-\s*(\d+)\.(\d+)\.`)

// defaultPort is the port each network driver listens on when the form leaves it blank.
func defaultPort(driver string) int {
	switch driver {
	case driverPostgres:
		return 5432
	case driverMSSQL:
		return 1433
	default:
		return 3306
	}
}

// buildMSSQLDSN assembles a go-mssqldb URL. The database travels as a query parameter
// because the URL path is the instance name in this driver's grammar, and encrypt
// defaults to true for the same reason Postgres defaults to sslmode=require.
func buildMSSQLDSN(in ConnectionInput) (string, error) {
	host := strings.TrimSpace(in.Host)
	name := strings.TrimSpace(in.Name)
	user := strings.TrimSpace(in.User)
	if host == "" || name == "" || user == "" {
		return "", fmt.Errorf("%w: host, database and user are required", errInvalidConfig)
	}
	if mssqlSystemDatabases[strings.ToLower(name)] {
		return "", fmt.Errorf("%w: %s is a SQL Server system database", errInvalidConfig, name)
	}
	port := in.Port
	if port == 0 {
		port = defaultPort(driverMSSQL)
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("%w: port %d is out of range", errInvalidConfig, port)
	}

	encrypt := strings.ToLower(strings.TrimSpace(in.SSLMode))
	if encrypt == "" {
		encrypt = "true"
	}
	if !mssqlEncryptModes[encrypt] {
		return "", fmt.Errorf("%w: encrypt must be true, strict, false or disable", errInvalidConfig)
	}

	query := url.Values{}
	query.Set("database", name)
	query.Set("encrypt", encrypt)
	if in.TrustServerCertificate {
		query.Set("TrustServerCertificate", "true")
	}
	for k, v := range in.Params {
		query.Set(k, v)
	}

	u := url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(user, in.Password),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		RawQuery: encodeSorted(query),
	}
	return u.String(), nil
}

// validateMSSQLDSN checks a pasted connection string before it is stored. Boot refuses
// anything but the sqlserver:// URL form, and a URL without a database would put every
// McHarbor table into the login's default database, which is usually master.
func validateMSSQLDSN(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "sqlserver") {
		return fmt.Errorf("%w: a SQL Server connection string must be a sqlserver:// URL", errInvalidConfig)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("%w: the connection string names no host", errInvalidConfig)
	}
	name := ""
	for key, values := range u.Query() {
		lower := strings.ToLower(key)
		if (lower == "database" || lower == "initial catalog") && len(values) > 0 {
			name = strings.TrimSpace(values[0])
		}
	}
	if name == "" {
		return fmt.Errorf("%w: the connection string must name a database (?database=...)", errInvalidConfig)
	}
	if mssqlSystemDatabases[strings.ToLower(name)] {
		return fmt.Errorf("%w: %s is a SQL Server system database", errInvalidConfig, name)
	}
	return nil
}

// openMSSQLTarget opens a candidate SQL Server through the same statement rewriter the
// booted instance uses, so Migrate and the copier send it exactly the T-SQL a running
// McHarbor would. A bare handle would receive SQLite placeholders and pragmas it rejects.
func openMSSQLTarget(dsn string) (*sql.DB, error) {
	dsn = sqldialect.MSSQLDSN(dsn)
	plain, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, err
	}
	drv := plain.Driver()
	if err := plain.Close(); err != nil {
		return nil, fmt.Errorf("closing unwrapped sqlserver handle: %w", err)
	}
	dc, ok := drv.(driver.DriverContext)
	if !ok {
		return nil, errors.New("the sqlserver driver does not expose a connector")
	}
	connector, err := dc.OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(telemetry.WrapDBConnectorWithDialect(connector, "mssql_migration_target", telemetry.Dialect{
		Rewrite: sqldialect.MSSQL, Args: sqldialect.MSSQLArgs, Value: sqldialect.MSSQLValue,
	})), nil
}

// checkMSSQLVersion rejects anything older than SQL Server 2022 by its engine build.
func checkMSSQLVersion(banner string) error {
	match := mssqlBuild.FindStringSubmatch(banner)
	if match == nil {
		return nil
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return nil //nolint:nilerr // a build number this code cannot parse is not a reason to refuse the server
	}
	if major < minMSSQLMajor {
		return fmt.Errorf("server reports SQL Server build %d, older than SQL Server 2022 (build %d)", major, minMSSQLMajor)
	}
	return nil
}

// mssqlSessionFindings reads the three settings a SQL Server target is refused or warned
// for: a system database, XACT_ABORT (boot refuses it) and READ_COMMITTED_SNAPSHOT off
// (boot only warns, so the test warns too). warn is empty when nothing is worth saying.
func mssqlSessionFindings(ctx context.Context, database *sql.DB) (warn string, err error) {
	var (
		name       string
		options    int64
		snapshotOn bool
	)
	if err := database.QueryRowContext(ctx,
		"SELECT DB_NAME(), CAST(@@OPTIONS AS BIGINT), is_read_committed_snapshot_on FROM sys.databases WHERE name = DB_NAME()",
	).Scan(&name, &options, &snapshotOn); err != nil {
		return "", fmt.Errorf("reading SQL Server session settings: %w", err)
	}
	if mssqlSystemDatabases[strings.ToLower(name)] {
		return "", fmt.Errorf("the connection opened the system database %s; name the McHarbor database with ?database=", name)
	}
	if options&mssqlXactAbort != 0 {
		return "", errors.New("sessions run with XACT_ABORT ON, which McHarbor refuses at start-up; remove it from the login's or connector's session settings")
	}
	if !snapshotOn {
		return "READ_COMMITTED_SNAPSHOT is off, so long reads will block writers. Run: ALTER DATABASE [" +
			strings.ReplaceAll(name, "]", "]]") + "] SET READ_COMMITTED_SNAPSHOT ON WITH ROLLBACK IMMEDIATE", nil
	}
	return "", nil
}
