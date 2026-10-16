// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/config"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/dbconfig"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/sqldialect"
)

// errInvalidConfig is returned by BuildDSN when the form is incomplete.
// The handler maps it onto i18n.ErrDatabaseConfigInvalid.
var errInvalidConfig = errors.New("database connection details are incomplete")

// errCleanupUnavailable is returned when the old-database deletion is asked
// for and the guard says no.
var errCleanupUnavailable = errors.New("there is no old database this install may delete")

// Service owns everything about the database configuration that is not the
// migration job itself: reading the active connection, turning a form into
// a DSN, and writing the encrypted secret.
type Service struct {
	cfg    *config.Config
	enc    *encryption.Service
	db     *sql.DB
	logger *slog.Logger
}

// NewService builds the service from the two dependencies it actually
// needs plus the live connection it reports on.
func NewService(cfg *config.Config, enc *encryption.Service, database *sql.DB, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{cfg: cfg, enc: enc, db: database, logger: logger}
}

// EnvPinned reports whether MCHARBOR_DB_DRIVER decided this install's
// database, in which case nothing written through the UI would survive a
// restart. Checked with LookupEnv rather than through Config, because the
// struct tag carries envDefault:"sqlite" and would make an unset variable
// indistinguishable from a deliberate one.
func (s *Service) EnvPinned() bool {
	_, ok := os.LookupEnv("MCHARBOR_DB_DRIVER")
	return ok
}

// Configured reports whether a connection secret has been written.
func (s *Service) Configured() bool { return dbconfig.Exists(s.cfg.DataDir) }

// AvailableDrivers is the list the picker renders, in the order it renders
// them: the embedded default first, then the three network backends.
func AvailableDrivers() []string {
	return []string{driverSQLite, driverPostgres, driverMySQL, driverMSSQL}
}

// Current describes the database this process is running on.
func (s *Service) Current(ctx context.Context) (ConfigResponse, error) {
	driver := coredb.DriverOf()
	if driver == "" {
		driver = coredb.DriverSQLite
	}

	out := ConfigResponse{
		Driver:           string(driver),
		EnvPinned:        s.EnvPinned(),
		Configured:       s.Configured(),
		AvailableDrivers: AvailableDrivers(),
	}

	switch {
	case out.EnvPinned:
		out.Source = string(dbconfig.SourceEnv)
		_, pathOrDSN := s.cfg.DBConfig()
		out.Target = dbconfig.Redact(string(driver), pathOrDSN)
	case out.Configured:
		out.Source = string(dbconfig.SourceSecret)
		stored, err := dbconfig.Load(s.cfg.DataDir, s.enc)
		if err != nil {
			return out, err
		}
		if stored != nil {
			switch {
			case stored.DSN != "":
				out.Target = dbconfig.Redact(stored.Driver, stored.DSN)
			case stored.Path != "":
				out.Target = stored.Path
			default:
				out.Target = s.cfg.DatabasePath
			}
			updated := stored.UpdatedAt
			out.UpdatedAt = &updated
		}
	default:
		out.Source = string(dbconfig.SourceDefault)
		out.Target = s.cfg.DatabasePath
	}

	if driver == coredb.DriverSQLite {
		if info, err := os.Stat(s.sqliteFilePath()); err == nil {
			out.SizeBytes = info.Size()
		}
	}

	version, err := coredb.ServerVersion(ctx, s.db, driver)
	if err == nil {
		out.ServerVersion = version
	}

	tables, err := coredb.ListTables(ctx, s.db, driver)
	if err != nil {
		return out, err
	}
	out.Tables = len(tables)
	for _, t := range tables {
		n, err := coredb.RowCount(ctx, s.db, driver, t)
		if err != nil {
			return out, err
		}
		out.Rows += n
	}
	return out, nil
}

// sqliteFilePath resolves where the live SQLite file is, honouring a path
// the operator stored through the UI.
func (s *Service) sqliteFilePath() string {
	stored, err := dbconfig.Load(s.cfg.DataDir, s.enc)
	if err == nil && stored != nil && strings.TrimSpace(stored.Path) != "" {
		return stored.Path
	}
	return s.cfg.DatabasePath
}

// Persist writes the encrypted connection secret. This is the only call in
// the module that changes what McHarbor will boot against.
//
// When the install is moving off SQLite, the outgoing file path is recorded
// alongside the new connection. That is what later lets the operator delete
// it: after the restart McHarbor has no other way to know where it used to
// live, and guessing at DATABASE_PATH would be a guess about which file to
// delete — not a guess worth making.
func (s *Service) Persist(in ConnectionInput) error {
	driver := dbconfig.NormalizeDriver(in.Driver)
	stored := dbconfig.Stored{Driver: driver, Previous: s.outgoingSQLite(driver)}

	if driver == driverSQLite {
		stored.Path = strings.TrimSpace(in.Path)
		if stored.Path == "" {
			stored.Path = s.cfg.DatabasePath
		}
	} else {
		dsn, err := BuildDSN(in)
		if err != nil {
			return err
		}
		stored.DSN = dsn
	}
	return dbconfig.Save(s.cfg.DataDir, s.enc, stored)
}

// SameAsCurrent reports whether the requested target is the database
// McHarbor is already running on. Migrating a database onto itself would
// clear every destination table before reading a single row from the
// source, which on a same-target run is the same file.
func (s *Service) SameAsCurrent(in ConnectionInput) bool {
	driver := dbconfig.NormalizeDriver(in.Driver)
	current := string(coredb.DriverOf())
	if current == "" {
		current = driverSQLite
	}
	if driver != current {
		return false
	}
	if driver == driverSQLite {
		want := strings.TrimSpace(in.Path)
		if want == "" {
			want = s.cfg.DatabasePath
		}
		return sameFile(want, s.sqliteFilePath())
	}

	want, err := BuildDSN(in)
	if err != nil {
		return false
	}
	stored, err := dbconfig.Load(s.cfg.DataDir, s.enc)
	if err == nil && stored != nil && stored.DSN != "" {
		return want == stored.DSN
	}
	if s.EnvPinned() {
		_, pathOrDSN := s.cfg.DBConfig()
		return want == pathOrDSN
	}
	return false
}

// sameFile compares two SQLite paths, falling back to a string compare when
// either side cannot be stat'ed (the target file may not exist yet).
func sameFile(a, b string) bool {
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	if aerr == nil && berr == nil {
		return os.SameFile(ai, bi)
	}
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// BuildDSN turns a connection form into the connection string its driver
// expects. A DSN supplied verbatim is trimmed and returned unchanged — the
// escape hatch exists precisely for the options this builder does not model.
func BuildDSN(in ConnectionInput) (string, error) {
	driver := dbconfig.NormalizeDriver(in.Driver)
	if raw := strings.TrimSpace(in.DSN); raw != "" {
		if driver == driverMSSQL {
			if err := validateMSSQLDSN(raw); err != nil {
				return "", err
			}
		}
		return raw, nil
	}

	switch driver {
	case driverPostgres:
		return buildPostgresDSN(in)
	case driverMySQL:
		return buildMySQLDSN(in)
	case driverMSSQL:
		return buildMSSQLDSN(in)
	case driverSQLite:
		return "", nil
	default:
		return "", fmt.Errorf("%w: unknown driver %q", errInvalidConfig, in.Driver)
	}
}

// buildPostgresDSN assembles a libpq URL. sslmode defaults to `require`:
// the connection carries every credential and every row in the product, and
// an operator who genuinely wants it in the clear on a private network can
// say so explicitly.
func buildPostgresDSN(in ConnectionInput) (string, error) {
	host := strings.TrimSpace(in.Host)
	name := strings.TrimSpace(in.Name)
	user := strings.TrimSpace(in.User)
	if host == "" || name == "" || user == "" {
		return "", fmt.Errorf("%w: host, database and user are required", errInvalidConfig)
	}
	port := in.Port
	if port == 0 {
		port = 5432
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("%w: port %d is out of range", errInvalidConfig, port)
	}

	sslMode := strings.TrimSpace(in.SSLMode)
	if sslMode == "" {
		sslMode = "require"
	}

	query := url.Values{}
	query.Set("sslmode", sslMode)
	for k, v := range in.Params {
		query.Set(k, v)
	}

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, in.Password),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     "/" + name,
		RawQuery: encodeSorted(query),
	}
	return u.String(), nil
}

// buildMySQLDSN assembles a go-sql-driver/mysql DSN.
//
// parseTime and loc are not optional extras: without them the driver hands
// DATETIME columns back as []byte, and every timestamp McHarbor reads would
// come through as a string in the server's local zone rather than a UTC
// time.Time. The rest of the application works in UTC throughout.
func buildMySQLDSN(in ConnectionInput) (string, error) {
	host := strings.TrimSpace(in.Host)
	name := strings.TrimSpace(in.Name)
	user := strings.TrimSpace(in.User)
	if host == "" || name == "" || user == "" {
		return "", fmt.Errorf("%w: host, database and user are required", errInvalidConfig)
	}
	port := in.Port
	if port == 0 {
		port = 3306
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("%w: port %d is out of range", errInvalidConfig, port)
	}

	tls := strings.TrimSpace(in.SSLMode)
	if tls == "" {
		tls = "preferred"
	}

	query := url.Values{}
	query.Set("parseTime", "true")
	query.Set("loc", "UTC")
	query.Set("charset", "utf8mb4")
	query.Set("tls", tls)
	for k, v := range in.Params {
		query.Set(k, v)
	}

	credentials := user
	if in.Password != "" {
		credentials += ":" + in.Password
	}
	return fmt.Sprintf("%s@tcp(%s)/%s?%s",
		credentials,
		net.JoinHostPort(host, strconv.Itoa(port)),
		name,
		encodeSorted(query),
	), nil
}

// encodeSorted renders query parameters in a stable order so the same form
// always produces byte-identical DSNs — which is what makes SameAsCurrent's
// string comparison meaningful.
func encodeSorted(values url.Values) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(values.Get(k)))
	}
	return strings.Join(parts, "&")
}

// hostPort pulls the address a connection would dial out of a form, so the
// tester can separate "the host is unreachable" from "the password is
// wrong" before it ever opens a driver.
//
// It returns ("", false) when the address cannot be determined — a raw DSN
// with an unusual shape, or SQLite, which dials nothing.
func hostPort(in ConnectionInput) (string, bool) {
	driver := dbconfig.NormalizeDriver(in.Driver)
	if driver == driverSQLite {
		return "", false
	}

	if host := strings.TrimSpace(in.Host); host != "" && strings.TrimSpace(in.DSN) == "" {
		port := in.Port
		if port == 0 {
			port = defaultPort(driver)
		}
		return net.JoinHostPort(host, strconv.Itoa(port)), true
	}

	raw := strings.TrimSpace(in.DSN)
	if raw == "" {
		return "", false
	}
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		host := u.Hostname()
		port := u.Port()
		if port == "" {
			port = strconv.Itoa(defaultPort(driver))
		}
		return net.JoinHostPort(host, port), true
	}
	// go-sql-driver form: user:pw@tcp(host:port)/db
	open := strings.Index(raw, "(")
	closeIdx := strings.Index(raw, ")")
	if open >= 0 && closeIdx > open {
		addr := raw[open+1 : closeIdx]
		if _, _, err := net.SplitHostPort(addr); err == nil {
			return addr, true
		}
	}
	return "", false
}

// openTarget opens a connection to a candidate database without touching
// the package-level instance core/db.Open installs. The tester and the
// migrator both need a second, throwaway connection alongside the live one.
func openTarget(ctx context.Context, driver string, dsn, path string) (*sql.DB, error) {
	var (
		database *sql.DB
		err      error
	)
	switch dbconfig.NormalizeDriver(driver) {
	case driverPostgres:
		database, err = sql.Open("pgx", dsn)
	case driverMySQL:
		// The session settings the booted instance pins, so Migrate runs under the same
		// sql_mode: without NO_BACKSLASH_ESCAPES migration 071's ESCAPE '\' is a syntax error.
		database, err = sql.Open("mysql", sqldialect.MySQLDSN(dsn))
	case driverMSSQL:
		database, err = openMSSQLTarget(dsn)
	case driverSQLite:
		database, err = sql.Open("sqlite", "file:"+path+"?_journal_mode=WAL&_busy_timeout=60000&_foreign_keys=ON")
	default:
		return nil, fmt.Errorf("%w: unknown driver %q", errInvalidConfig, driver)
	}
	if err != nil {
		return nil, err
	}

	// A migration is a long single-writer job, not a request workload;
	// a small pool keeps it from monopolising a shared server.
	database.SetMaxOpenConns(8)
	database.SetMaxIdleConns(4)
	database.SetConnMaxIdleTime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := database.PingContext(pingCtx); err != nil {
		if closeErr := database.Close(); closeErr != nil {
			return nil, fmt.Errorf("closing target after failed ping: %w (original: %w)", closeErr, err)
		}
		return nil, err
	}
	return database, nil
}

// outgoingSQLite returns the SQLite file this install is migrating away from,
// or nil when it is not moving off SQLite.
//
// Only SQLite is recorded. A PostgreSQL or MySQL database McHarbor leaves
// behind belongs to whoever administers that server; McHarbor knowing its DSN
// is not a reason for McHarbor to offer to drop it.
func (s *Service) outgoingSQLite(newDriver string) *dbconfig.PreviousSQLite {
	current := string(coredb.DriverOf())
	if current == "" {
		current = driverSQLite
	}
	if current != driverSQLite || newDriver == driverSQLite {
		// Either there is no SQLite file to leave behind, or the install is
		// staying on SQLite and the "old" file is the live one.
		return s.carriedPrevious()
	}
	path := s.sqliteFilePath()
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	return &dbconfig.PreviousSQLite{Path: path, MigratedAt: time.Now().UTC()}
}

// carriedPrevious keeps an earlier recorded file across a write that is not
// itself a migration off SQLite, so a postgres -> mysql move does not forget
// the SQLite file the install started on.
func (s *Service) carriedPrevious() *dbconfig.PreviousSQLite {
	stored, err := dbconfig.Load(s.cfg.DataDir, s.enc)
	if err != nil || stored == nil {
		return nil
	}
	return stored.Previous
}

// sqliteSidecars are the files SQLite keeps next to the database itself. The
// write-ahead log can be larger than the database; leaving them behind would
// reclaim a fraction of the space the operator asked to free, and would leave
// a half-deleted database on disk.
var sqliteSidecars = []string{"", "-wal", "-shm", "-journal"}

// Cleanup describes the old SQLite file a completed migration left behind,
// and whether McHarbor is willing to delete it.
func (s *Service) Cleanup() CleanupStatus {
	stored, err := dbconfig.Load(s.cfg.DataDir, s.enc)
	if err != nil {
		return CleanupStatus{Reason: "unreadable"}
	}
	if stored == nil || stored.Previous == nil || strings.TrimSpace(stored.Previous.Path) == "" {
		return CleanupStatus{Reason: "none"}
	}

	path := stored.Previous.Path
	// The decisive check. If the recorded path resolves to the database this
	// process is reading from, deleting it would destroy the live install —
	// so the action is refused rather than merely hidden.
	if sameFile(path, s.sqliteFilePath()) || string(coredb.DriverOf()) == driverSQLite {
		return CleanupStatus{Reason: "in_use"}
	}

	out := CleanupStatus{Path: path}
	migratedAt := stored.Previous.MigratedAt
	out.MigratedAt = &migratedAt
	for _, suffix := range sqliteSidecars {
		candidate := path + suffix
		info, statErr := os.Stat(candidate)
		if statErr != nil || info.IsDir() {
			continue
		}
		out.Files = append(out.Files, CleanupFile{Path: candidate, Bytes: info.Size()})
		out.TotalBytes += info.Size()
	}
	if len(out.Files) == 0 {
		return CleanupStatus{Reason: "missing"}
	}
	out.Available = true
	return out
}

// RemovePreviousSQLite deletes the old database and its sidecar files, then
// clears the record so the action is offered exactly once.
//
// There is no undo. The guard is Cleanup(): it refuses when the path is the
// live database, and the caller re-checks it here rather than trusting a
// status the browser may have been holding for an hour.
func (s *Service) RemovePreviousSQLite() (CleanupResult, error) {
	status := s.Cleanup()
	if !status.Available {
		return CleanupResult{}, fmt.Errorf("%w: %s", errCleanupUnavailable, status.Reason)
	}

	var result CleanupResult
	for _, file := range status.Files {
		if err := os.Remove(file.Path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return result, fmt.Errorf("removing %s: %w", filepath.Base(file.Path), err)
		}
		result.Removed = append(result.Removed, file.Path)
		result.Bytes += file.Bytes
	}

	stored, err := dbconfig.Load(s.cfg.DataDir, s.enc)
	if err != nil {
		return result, fmt.Errorf("reloading the database configuration: %w", err)
	}
	if stored != nil {
		stored.Previous = nil
		if err := dbconfig.Save(s.cfg.DataDir, s.enc, *stored); err != nil {
			return result, fmt.Errorf("clearing the recorded previous database: %w", err)
		}
	}
	return result, nil
}
