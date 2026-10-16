// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package dbconfig persists the database connection McHarbor should boot
// against, as an encrypted file rather than an environment variable.
//
// Why a file and not a settings row: the connection string is the one piece
// of configuration that cannot live in the database it describes. Why a file
// and not an env var: an env var cannot be written by the running process, so
// the "choose your database in the UI" flow would have no way to remember the
// choice, and the credentials would sit in `docker inspect` output for anyone
// on the host.
//
// The file is `<DATA_DIR>/database.json`, mode 0600, and the DSN inside it is
// AES-256-GCM encrypted with the same core/encryption service that protects
// agent tokens and storage credentials.
package dbconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/therealmcsparrow/mcharbor/core/demo"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// FileName is the basename of the stored connection secret inside DATA_DIR.
const FileName = "database.json"

// currentVersion is the schema version of the stored file. Bumping it means
// Load has to migrate older shapes; nothing does yet.
const currentVersion = 1

// Source records where the connection McHarbor booted against came from.
// The UI uses it to decide whether the form is editable: an env-pinned
// install is managed by whoever wrote the Compose file, not by an admin
// clicking in the browser.
type Source string

const (
	// SourceEnv means MCHARBOR_DB_DRIVER was explicitly present in the
	// process environment. It wins over everything else so existing HA
	// deployments keep booting exactly as they did.
	SourceEnv Source = "env"
	// SourceSecret means the connection came from the encrypted
	// database.json written by the setup wizard or the settings page.
	SourceSecret Source = "secret"
	// SourceDefault means neither was present and the built-in SQLite
	// file at DATABASE_PATH was used.
	SourceDefault Source = "default"
	// SourceDemo means a demo.json marker sent the boot onto demo.db. It beats
	// every other source because demo mode must never open the real database.
	SourceDemo Source = "demo"
)

// Stored is the decrypted content of database.json. DSN is plaintext in
// memory and encrypted on disk; it must never be serialised into an API
// response without going through Redact.
type Stored struct {
	Version   int       `json:"version"`
	Driver    string    `json:"driver"`
	Path      string    `json:"path,omitempty"`
	DSN       string    `json:"dsn,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Previous records the backend this install migrated away from. Only the
	// SQLite half of it is kept — a file path McHarbor can still see on disk,
	// and therefore still offer to delete once the operator is satisfied the
	// migration worked. A previous Postgres or MySQL database belongs to
	// whoever administers that server, so nothing about it is stored and
	// nothing about it is offered.
	Previous *PreviousSQLite `json:"previous,omitempty"`
}

// PreviousSQLite is the SQLite file a migration left behind.
type PreviousSQLite struct {
	Path       string    `json:"path"`
	MigratedAt time.Time `json:"migratedAt"`
}

// onDisk is the wire shape. It differs from Stored only in that DSN holds
// the "enc:v1:..." ciphertext.
type onDisk struct {
	Version   int             `json:"version"`
	Driver    string          `json:"driver"`
	Path      string          `json:"path,omitempty"`
	DSN       string          `json:"dsn,omitempty"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Previous  *PreviousSQLite `json:"previous,omitempty"`
}

// FilePath returns the absolute location of the connection secret.
func FilePath(dataDir string) string {
	return filepath.Join(dataDir, FileName)
}

// Exists reports whether a connection secret has been written. The setup
// wizard uses this as the "the operator has made a choice" marker, which is
// why choosing SQLite still writes a file: the marker cannot live in the
// database, because a later migration would carry it to the new backend and
// a fresh SQLite file would then look already-configured.
func Exists(dataDir string) bool {
	_, err := os.Stat(FilePath(dataDir))
	return err == nil
}

// Load reads and decrypts the connection secret. It returns (nil, nil) when
// no file has been written yet, which is the normal state of a fresh install.
func Load(dataDir string, enc *encryption.Service) (*Stored, error) {
	raw, err := os.ReadFile(FilePath(dataDir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading database config: %w", err)
	}

	var disk onDisk
	if err := json.Unmarshal(raw, &disk); err != nil {
		return nil, fmt.Errorf("decoding database config: %w", err)
	}
	if disk.Version > currentVersion {
		return nil, fmt.Errorf("database config version %d is newer than this McHarbor understands (%d)", disk.Version, currentVersion)
	}

	out := Stored{
		Version:   disk.Version,
		Driver:    strings.TrimSpace(disk.Driver),
		Path:      disk.Path,
		UpdatedAt: disk.UpdatedAt,
		Previous:  disk.Previous,
	}
	if disk.DSN != "" {
		if enc == nil {
			return nil, errors.New("database config holds an encrypted DSN but no encryption service is configured")
		}
		plain, err := enc.Decrypt(disk.DSN)
		if err != nil {
			return nil, fmt.Errorf("decrypting database DSN: %w", err)
		}
		out.DSN = plain
	}
	return &out, nil
}

// Save encrypts the DSN and writes the secret atomically (temp file in the
// same directory, then rename) so a crash mid-write can never leave a
// truncated file that would make the next boot unable to find its database.
func Save(dataDir string, enc *encryption.Service, s Stored) error {
	if strings.TrimSpace(s.Driver) == "" {
		return errors.New("database config requires a driver")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	disk := onDisk{
		Version:   currentVersion,
		Driver:    strings.TrimSpace(s.Driver),
		Path:      s.Path,
		UpdatedAt: time.Now().UTC(),
		Previous:  s.Previous,
	}
	if s.DSN != "" {
		if enc == nil {
			return errors.New("cannot store a database DSN without an encryption service")
		}
		ciphertext, err := enc.Encrypt(s.DSN)
		if err != nil {
			return fmt.Errorf("encrypting database DSN: %w", err)
		}
		disk.DSN = ciphertext
	}

	payload, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding database config: %w", err)
	}

	target := FilePath(dataDir)
	tmp, err := os.CreateTemp(dataDir, ".database-*.json")
	if err != nil {
		return fmt.Errorf("creating temporary database config: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort cleanup of the temp file if anything below failed
		// before the rename. After a successful rename the path is gone
		// and Remove returns ErrNotExist, which is fine to ignore here.
		if removeErr := os.Remove(tmpName); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			// Nothing actionable — the file is inert JSON in the data dir.
			_ = removeErr
		}
	}()

	if _, err := tmp.Write(payload); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			return fmt.Errorf("closing temporary database config after write error: %w (original: %w)", closeErr, err)
		}
		return fmt.Errorf("writing temporary database config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			return fmt.Errorf("closing temporary database config after sync error: %w (original: %w)", closeErr, err)
		}
		return fmt.Errorf("syncing temporary database config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary database config: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("restricting database config permissions: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("installing database config: %w", err)
	}
	return nil
}

// Remove deletes the connection secret. The next boot falls back to the
// env var, or to the built-in SQLite file. Absent is not an error.
func Remove(dataDir string) error {
	if err := os.Remove(FilePath(dataDir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing database config: %w", err)
	}
	return nil
}

// Resolve decides which database this process should open, and says where
// the answer came from.
//
// Precedence, highest first:
//
//  0. A demo.json marker in the data directory, which pins demo.db.
//  1. MCHARBOR_DB_DRIVER explicitly present in the environment. Checked with
//     os.LookupEnv rather than the parsed Config, because the struct tag
//     carries envDefault:"sqlite" and would make "unset" indistinguishable
//     from a deliberate "=sqlite".
//  2. The encrypted database.json written through the UI.
//  3. The built-in SQLite file at DATABASE_PATH.
func Resolve(cfg *config.Config, enc *encryption.Service) (db.Config, Source, error) {
	marker, err := demo.Load(cfg.DataDir)
	if err != nil {
		return db.Config{}, SourceDemo, err
	}
	if marker != nil {
		if err := demo.PrepareDatabase(cfg.DataDir); err != nil {
			return db.Config{}, SourceDemo, err
		}
		return db.Config{Driver: db.DriverSQLite, Path: demo.DatabasePath(cfg.DataDir)}, SourceDemo, nil
	}

	if raw, pinned := os.LookupEnv("MCHARBOR_DB_DRIVER"); pinned {
		// A value nobody recognises used to boot the SQLite file, so a typo, or a driver
		// the switch below did not know, looked like a working start on the wrong database.
		driver := db.Driver(NormalizeDriver(raw))
		switch driver {
		case db.DriverSQLite:
			return db.Config{Driver: driver, Path: cfg.DatabasePath, DSN: cfg.DatabasePath}, SourceEnv, nil
		case db.DriverPostgres, db.DriverMySQL, db.DriverMSSQL:
			dsn := strings.TrimSpace(cfg.DBDSN)
			return db.Config{Driver: driver, Path: dsn, DSN: dsn}, SourceEnv, nil
		default:
			return db.Config{}, SourceEnv, fmt.Errorf("MCHARBOR_DB_DRIVER=%q is not a supported database driver; use sqlite, postgres, mysql or mssql", raw)
		}
	}

	stored, err := Load(cfg.DataDir, enc)
	if err != nil {
		return db.Config{}, SourceDefault, err
	}
	if stored != nil {
		driver := NormalizeDriver(stored.Driver)
		switch driver {
		case string(db.DriverSQLite):
			path := stored.Path
			if strings.TrimSpace(path) == "" {
				path = cfg.DatabasePath
			}
			return db.Config{Driver: db.DriverSQLite, Path: path}, SourceSecret, nil
		case string(db.DriverPostgres), string(db.DriverMySQL), string(db.DriverMSSQL):
			if strings.TrimSpace(stored.DSN) == "" {
				return db.Config{}, SourceSecret, fmt.Errorf("stored %s configuration has no connection string", driver)
			}
			return db.Config{Driver: db.Driver(driver), DSN: stored.DSN}, SourceSecret, nil
		default:
			return db.Config{}, SourceSecret, fmt.Errorf("stored database driver %q is not one of sqlite, postgres, mysql, mssql", stored.Driver)
		}
	}

	return db.Config{Driver: db.DriverSQLite, Path: cfg.DatabasePath}, SourceDefault, nil
}

// NormalizeDriver folds the aliases the UI and the env var both accept onto
// the four canonical db.Driver values. An unknown value is returned lowered
// and trimmed so the caller can report it verbatim.
func NormalizeDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", "sqlite", "sqlite3":
		return string(db.DriverSQLite)
	case "postgres", "postgresql", "pgsql":
		return string(db.DriverPostgres)
	case "mysql", "mariadb":
		return string(db.DriverMySQL)
	case "mssql", "sqlserver", "azuresql":
		return string(db.DriverMSSQL)
	default:
		return strings.ToLower(strings.TrimSpace(driver))
	}
}

// Redact strips the password from a connection string so it can be logged,
// audited, or shown in the UI. SQLite has no credentials, so its path is
// returned unchanged.
//
// Two DSN shapes are handled: the URL form used by Postgres
// (postgres://user:pw@host/db) and the go-sql-driver/mysql form
// (user:pw@tcp(host:3306)/db).
func Redact(driver, dsn string) string {
	if dsn == "" {
		return ""
	}
	switch NormalizeDriver(driver) {
	case string(db.DriverSQLite):
		return dsn
	case string(db.DriverPostgres):
		return redactURL(dsn)
	case string(db.DriverMySQL):
		return redactMySQL(dsn)
	case string(db.DriverMSSQL):
		return redactURL(dsn)
	default:
		return redactURL(dsn)
	}
}

// redactURL replaces the password in a scheme://user:pw@host URL.
func redactURL(dsn string) string {
	scheme := strings.Index(dsn, "://")
	if scheme < 0 {
		return redactMySQL(dsn)
	}
	at := strings.LastIndex(dsn, "@")
	if at <= scheme+3 {
		return dsn
	}
	creds := dsn[scheme+3 : at]
	colon := strings.Index(creds, ":")
	if colon < 0 {
		return dsn
	}
	return dsn[:scheme+3] + creds[:colon+1] + "***" + dsn[at:]
}

// redactMySQL replaces the password in a user:pw@tcp(host:port)/db DSN.
func redactMySQL(dsn string) string {
	at := strings.LastIndex(dsn, "@")
	if at < 0 {
		return dsn
	}
	creds := dsn[:at]
	colon := strings.Index(creds, ":")
	if colon < 0 {
		return dsn
	}
	return creds[:colon+1] + "***" + dsn[at:]
}
