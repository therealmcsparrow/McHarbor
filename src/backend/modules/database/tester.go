// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/dbconfig"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

// The step-by-step connection test.
//
// The point of running seven small checks instead of one Ping is that
// "connection failed" is four different problems with four different fixes:
// a typo in the hostname, a firewall, a wrong password, and a user without
// CREATE rights all present as the same red toast otherwise. Each step here
// isolates one of them, and the UI shows which one stopped.

// Minimum server versions. Below these, the portable migration set starts
// relying on syntax the server does not have: Postgres 12 for generated
// columns and the modern ON CONFLICT forms, MySQL 8.0 / MariaDB 10.6 for
// the JSON functions the RBAC preset migrations use.
const (
	minPostgresMajor = 12
	minMySQLMajor    = 8
	minMariaDBMajor  = 10
	minMariaDBMinor  = 6
)

// versionNumber pulls the first dotted version out of a server banner such
// as "PostgreSQL 16.2 (Debian 16.2-1) on x86_64..." or
// "10.11.6-MariaDB-1:10.11.6+maria~deb12".
var versionNumber = regexp.MustCompile(`(\d+)\.(\d+)`)

// stepRunner accumulates the step list as each check runs.
type stepRunner struct {
	steps []TestStep
}

// run times a check, records its outcome, and reports whether the caller
// should continue. A failure stops the run; a warning does not.
func (sr *stepRunner) run(id string, fn func() (string, i18n.MsgCode, string, error)) bool {
	started := time.Now()
	status, code, detail, err := fn()
	step := TestStep{
		ID:         id,
		Status:     status,
		Detail:     detail,
		DurationMS: time.Since(started).Milliseconds(),
	}
	if err != nil {
		step.Status = stepFail
		step.Code = string(code)
		if step.Detail == "" {
			step.Detail = err.Error()
		}
	} else if status == stepWarn {
		step.Code = string(code)
	}
	sr.steps = append(sr.steps, step)
	return step.Status != stepFail
}

// skipRest marks every step after a failure as skipped, so the UI can show
// the full checklist rather than a list that stops mid-way with no
// explanation of what was never attempted.
func (sr *stepRunner) skipRest(ids []string) {
	seen := make(map[string]bool, len(sr.steps))
	for _, s := range sr.steps {
		seen[s.ID] = true
	}
	for _, id := range ids {
		if !seen[id] {
			sr.steps = append(sr.steps, TestStep{ID: id, Status: stepSkip})
		}
	}
}

// allSteps is the checklist in display order.
var allSteps = []string{"validate", "resolve", "connect", "version", "privileges", "schema", "encoding"}

// Test runs the seven checks against a candidate database and returns the
// whole checklist, pass or fail.
//
// The returned connection, when non-nil, is the caller's to close: the
// migrator reuses it so a successful test is not immediately thrown away
// and re-dialled.
func (s *Service) Test(ctx context.Context, in ConnectionInput) (TestResponse, *sql.DB) {
	driver := dbconfig.NormalizeDriver(in.Driver)
	sr := &stepRunner{}
	out := TestResponse{Driver: driver}

	var (
		dsn    string
		path   string
		target *sql.DB
	)

	ok := sr.run("validate", func() (string, i18n.MsgCode, string, error) {
		switch driver {
		case driverSQLite:
			path = strings.TrimSpace(in.Path)
			if path == "" {
				path = s.cfg.DatabasePath
			}
			return stepPass, "", "file: " + path, nil
		case driverPostgres, driverMySQL, driverMSSQL:
			built, err := BuildDSN(in)
			if err != nil {
				return stepFail, i18n.ErrDatabaseConfigInvalid, sanitize(err, ""), err
			}
			dsn = built
			return stepPass, "", dbconfig.Redact(driver, dsn), nil
		default:
			err := fmt.Errorf("%w: unknown driver %q", errInvalidConfig, in.Driver)
			return stepFail, i18n.ErrDatabaseConfigInvalid, err.Error(), err
		}
	})
	out.Target = dbconfig.Redact(driver, dsn)
	if driver == driverSQLite {
		out.Target = path
	}
	if !ok {
		sr.skipRest(allSteps)
		out.Steps = sr.steps
		return out, nil
	}

	ok = sr.run("resolve", func() (string, i18n.MsgCode, string, error) {
		addr, dialable := hostPort(in)
		if !dialable {
			return stepSkip, "", "", nil
		}
		dialer := net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return stepFail, i18n.ErrDatabaseUnreachable, sanitize(err, dsn), err
		}
		if closeErr := conn.Close(); closeErr != nil {
			return stepPass, "", addr + " reachable (socket close: " + closeErr.Error() + ")", nil //nolint:nilerr // a failed socket close after a successful dial is a warning in the step detail, not a failed step
		}
		return stepPass, "", addr + " reachable", nil
	})
	if !ok {
		sr.skipRest(allSteps)
		out.Steps = sr.steps
		return out, nil
	}

	ok = sr.run("connect", func() (string, i18n.MsgCode, string, error) {
		opened, err := openTarget(ctx, driver, dsn, path)
		if err != nil {
			return stepFail, classify(err), sanitize(err, dsn), err
		}
		target = opened
		return stepPass, "", "", nil
	})
	if !ok || target == nil {
		sr.skipRest(allSteps)
		out.Steps = sr.steps
		return out, nil
	}

	// From here on the connection is open, so every remaining failure has
	// to close it before returning.
	closeTarget := func() {
		if target != nil {
			if err := target.Close(); err != nil && s.logger != nil {
				s.logger.Warn("database: closing tested connection", "error", err)
			}
			target = nil
		}
	}

	ok = sr.run("version", func() (string, i18n.MsgCode, string, error) {
		banner, err := coredb.ServerVersion(ctx, target, coredb.Driver(driver))
		if err != nil {
			return stepFail, i18n.ErrDatabaseConnectFailed, sanitize(err, dsn), err
		}
		out.ServerVersion = banner
		if err := checkVersion(driver, banner); err != nil {
			return stepFail, i18n.ErrDatabaseVersionUnsupported, banner, err
		}
		if driver == driverMSSQL {
			// The settings a booted instance refuses or warns about on SQL Server are
			// checked here, so a migration never restarts into a database that will not open.
			warn, err := mssqlSessionFindings(ctx, target)
			if err != nil {
				return stepFail, i18n.ErrDatabaseConnectFailed, sanitize(err, dsn), err
			}
			if warn != "" {
				return stepWarn, "", banner + " — " + warn, nil
			}
		}
		return stepPass, "", banner, nil
	})
	if !ok {
		closeTarget()
		sr.skipRest(allSteps)
		out.Steps = sr.steps
		return out, nil
	}

	ok = sr.run("privileges", func() (string, i18n.MsgCode, string, error) {
		if err := probeWriteAccess(ctx, target, coredb.Driver(driver)); err != nil {
			return stepFail, i18n.ErrDatabasePrivileges, sanitize(err, dsn), err
		}
		return stepPass, "", "create, insert, select and drop confirmed", nil
	})
	if !ok {
		closeTarget()
		sr.skipRest(allSteps)
		out.Steps = sr.steps
		return out, nil
	}

	sr.run("schema", func() (string, i18n.MsgCode, string, error) {
		found, err := coredb.ListTables(ctx, target, coredb.Driver(driver))
		if err != nil {
			return stepFail, i18n.ErrDatabaseConnectFailed, sanitize(err, dsn), err
		}
		out.ExistingTables = len(found)
		if len(found) == 0 {
			return stepPass, "", "empty database", nil
		}
		// Not a failure. A half-finished migration leaves tables behind,
		// and refusing to look at them would make the retry impossible.
		return stepWarn, i18n.ErrDatabaseSameTarget,
			strconv.Itoa(len(found)) + " existing tables will be emptied before the copy", nil
	})

	sr.run("encoding", func() (string, i18n.MsgCode, string, error) {
		charset, err := serverEncoding(ctx, target, driver)
		if err != nil {
			// Some managed services hide the catalog view this reads.
			// Not knowing the encoding is not a reason to block.
			return stepSkip, "", "", nil //nolint:nilerr // an unreadable encoding catalog must not block the test, so the step is skipped
		}
		if !isUTF8(charset) {
			return stepFail, i18n.ErrDatabaseEncoding, charset,
				fmt.Errorf("database character set is %s, not UTF-8", charset)
		}
		return stepPass, "", charset, nil
	})

	out.Steps = sr.steps
	out.OK = true
	for _, step := range out.Steps {
		if step.Status == stepFail {
			out.OK = false
			break
		}
	}
	if !out.OK {
		closeTarget()
		return out, nil
	}
	return out, target
}

// probeWriteAccess confirms the user may create, write, read and drop a
// table. A read-only role and a role that may only write to existing tables
// both fail here, which is exactly when the operator needs to know — not
// half way through the copy.
func probeWriteAccess(ctx context.Context, target *sql.DB, driver coredb.Driver) error {
	name := "mcharbor_probe_" + xid.New().String()
	quoted := coredb.QuoteIdent(driver, name)

	createSQL := "CREATE TABLE " + quoted + " (id TEXT)" // SQL identifier: quoted is coredb.QuoteIdent of a generated name
	selectSQL := "SELECT id FROM " + quoted + " LIMIT 1"
	switch driver {
	case coredb.DriverMySQL:
		// MySQL cannot index or key a bare TEXT column, and InnoDB is the
		// engine the rest of the schema assumes.
		createSQL = "CREATE TABLE " + quoted + " (id VARCHAR(64)) ENGINE=InnoDB"
	case coredb.DriverMSSQL:
		// TEXT is deprecated on SQL Server and LIMIT does not exist there.
		createSQL = "CREATE TABLE " + quoted + " (id NVARCHAR(64))"
		selectSQL = "SELECT TOP 1 id FROM " + quoted
	}
	if _, err := target.ExecContext(ctx, createSQL); err != nil {
		return fmt.Errorf("creating a probe table: %w", err)
	}

	drop := func() error {
		if _, err := target.ExecContext(ctx, "DROP TABLE "+quoted); err != nil {
			return fmt.Errorf("dropping the probe table: %w", err)
		}
		return nil
	}

	insertSQL := "INSERT INTO " + quoted + " (id) VALUES (" + coredb.Placeholder(driver, 1) + ")" // SQL identifier: quoted is coredb.QuoteIdent of a generated name
	if _, err := target.ExecContext(ctx, insertSQL, "probe"); err != nil {
		if dropErr := drop(); dropErr != nil {
			return fmt.Errorf("%w (and %w)", err, dropErr)
		}
		return fmt.Errorf("writing to a probe table: %w", err)
	}

	var got string
	if err := target.QueryRowContext(ctx, selectSQL).Scan(&got); err != nil {
		if dropErr := drop(); dropErr != nil {
			return fmt.Errorf("%w (and %w)", err, dropErr)
		}
		return fmt.Errorf("reading from a probe table: %w", err)
	}
	return drop()
}

// serverEncoding reads the database's character set.
func serverEncoding(ctx context.Context, target *sql.DB, driver string) (string, error) {
	var (
		query string
		value string
	)
	switch driver {
	case driverPostgres:
		query = "SELECT pg_encoding_to_char(encoding) FROM pg_database WHERE datname = current_database()"
	case driverMySQL:
		query = "SELECT @@character_set_database"
	case driverMSSQL:
		// Every text column is NVARCHAR, which is UTF-16 whatever the collation says.
		return "unicode", nil
	default:
		return "UTF-8", nil
	}
	if err := target.QueryRowContext(ctx, query).Scan(&value); err != nil {
		return "", err
	}
	return value, nil
}

// isUTF8 accepts every spelling the three servers use for a Unicode
// character set. utf8mb3 is deliberately rejected: it cannot store an
// emoji, and container names, stack descriptions and notification bodies
// all pass through user text.
func isUTF8(charset string) bool {
	c := strings.ToLower(strings.TrimSpace(charset))
	switch c {
	case "utf8", "utf-8", "utf8mb4", "unicode":
		return true
	default:
		return false
	}
}

// checkVersion rejects a server too old for the migration set.
func checkVersion(driver, banner string) error {
	match := versionNumber.FindStringSubmatch(banner)
	if match == nil {
		// A banner this code cannot parse is not a reason to refuse; the
		// privileges probe already proved the server speaks SQL.
		return nil
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return nil //nolint:nilerr // a version number this code cannot parse is not a reason to refuse the server
	}
	minor, err := strconv.Atoi(match[2])
	if err != nil {
		minor = 0
	}

	switch driver {
	case driverPostgres:
		if major < minPostgresMajor {
			return fmt.Errorf("server reports PostgreSQL %d, older than the minimum %d", major, minPostgresMajor)
		}
	case driverMySQL:
		if strings.Contains(strings.ToLower(banner), "mariadb") {
			if major < minMariaDBMajor || (major == minMariaDBMajor && minor < minMariaDBMinor) {
				return fmt.Errorf("server reports MariaDB %d.%d, older than the minimum %d.%d",
					major, minor, minMariaDBMajor, minMariaDBMinor)
			}
			return nil
		}
		if major < minMySQLMajor {
			return fmt.Errorf("server reports MySQL %d, older than the minimum %d", major, minMySQLMajor)
		}
	case driverMSSQL:
		return checkMSSQLVersion(banner)
	}
	return nil
}

// classify maps a driver error onto the i18n code that names its cause, so
// the UI can show a translated explanation instead of a driver string.
func classify(err error) i18n.MsgCode {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "password authentication failed"),
		strings.Contains(msg, "access denied"),
		strings.Contains(msg, "authentication failed"),
		strings.Contains(msg, "login failed for user"),
		strings.Contains(msg, "role") && strings.Contains(msg, "does not exist"):
		return i18n.ErrDatabaseAuthFailed
	case strings.Contains(msg, "no such host"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "network is unreachable"):
		return i18n.ErrDatabaseUnreachable
	case strings.Contains(msg, "permission denied"),
		strings.Contains(msg, "must be owner"):
		return i18n.ErrDatabasePrivileges
	default:
		return i18n.ErrDatabaseConnectFailed
	}
}

// sanitize prepares a driver error for a response body.
//
// The api-routes rule says never to return a raw err.Error() to a client,
// and it is right in general: an error string is written for an operator
// reading a log, not for an attacker probing a surface. A connection tester
// is the one endpoint where that trade runs the other way — its entire
// purpose is telling the operator why the connection failed, and a
// translated "could not connect" sends them straight to the container logs
// they opened this screen to avoid.
//
// So the message is returned, with the credentials taken out of it: any
// occurrence of the DSN is replaced with its redacted form, and the
// password on its own is replaced too, because several drivers quote the
// connection string back with only part of it. Both endpoints that reach
// this code are gated: the settings one behind database.manage, the
// first-run one behind "no account exists yet".
func sanitize(err error, dsn string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if dsn != "" {
		msg = strings.ReplaceAll(msg, dsn, "<connection string>")
		if password, ok := passwordOf(dsn); ok && password != "" {
			msg = strings.ReplaceAll(msg, password, "***")
		}
	}
	if len(msg) > 500 {
		msg = msg[:500] + "..."
	}
	return msg
}

// passwordOf extracts the password from either DSN shape, so sanitize can
// scrub it out of an error the driver echoed back in pieces.
func passwordOf(dsn string) (string, bool) {
	at := strings.LastIndex(dsn, "@")
	if at < 0 {
		return "", false
	}
	creds := dsn[:at]
	if scheme := strings.Index(creds, "://"); scheme >= 0 {
		creds = creds[scheme+3:]
	}
	colon := strings.Index(creds, ":")
	if colon < 0 {
		return "", false
	}
	return creds[colon+1:], true
}

// errTestFailed marks a pre-flight that did not pass, so the migrator can
// stop without inventing an error string of its own.
var errTestFailed = errors.New("the target database did not pass the connection test")
