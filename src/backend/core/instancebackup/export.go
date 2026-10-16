// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// NeverExported tables are meaningful only on the instance that wrote them. A
// restored session would be a restored login, and a restored one-time token a
// replayable one.
var NeverExported = map[string]bool{
	"schema_migrations": true, "_migrations": true,
	"sessions": true, "sudo_windows": true, "login_attempts": true, "login_ip_locks": true,
	"mfa_login_challenges": true, "oidc_states": true, "saml_requests": true,
	"password_reset_tokens": true, "device_codes": true, "storage_location_oauth_states": true,
	"install_tokens": true, "cluster_instances": true, "plugin_packages": true, "plugin_package_chunks": true,
}

// MetricsTables regenerate on their own and dominate an instance's size, so
// they are left out unless the operator asks, and the switch says why.
var MetricsTables = map[string]bool{
	"agent_health_samples": true, "agent_logs": true, "container_events": true,
	"container_metrics": true, "container_metrics_rollup": true, "container_state_samples": true,
	"container_uptime_daily": true, "host_metrics": true, "host_metrics_rollup": true,
	"license_usage_samples": true, "workflow_metrics": true,
}

// FileRoots are the data-directory entries an export carries. Plugin binaries,
// container backups, git work trees and the database file are deliberately absent.
var FileRoots = []string{
	"custom-nodes", "widgets", "widgets-state.json", "workflow-nodes", "files", "stacks",
	LedgerFile,
}

// Blob kinds.
const (
	BlobKey  = "key"
	BlobTLS  = "tls"
	BlobFile = "file"
)

// Logical paths of the key material inside an export.
const (
	KeyEncryption = "keys/encryption.key"
	KeySecret     = "keys/secret"
	TLSCACert     = "tls/ca.pem"
	TLSCAKey      = "tls/ca-key.pem"
	TLSCert       = "tls/cert.pem"
	TLSKey        = "tls/key.pem"
)

// Source is the instance being exported.
type Source struct {
	DB            *sql.DB
	Driver        coredb.Driver
	DataDir       string
	EncryptionKey []byte
	// Secret keys sessions and the audit chain's HMAC; without it a restored
	// chain cannot verify, so it travels with the encryption key.
	Secret string
	// UploadedCert is true when the serving certificate was supplied by the
	// operator; a generated one is re-issued from the restored CA instead.
	UploadedCert bool
	Version      string
	Build        string
	InstanceID   string
	CreatedBy    string
	Hostname     string
}

// ExportOptions are the operator's choices for one export.
type ExportOptions struct {
	Secret         Secret
	IncludeMetrics bool
	Note           string
	Progress       func(Progress)
}

// Progress is one step of a long export or restore.
type Progress struct {
	Phase string `json:"phase"`
	Item  string `json:"item,omitempty"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
}

type blobSource struct {
	inv  BlobInventory
	mode fs.FileMode
	data []byte
	file string
}

// Export writes one encrypted artifact to w. The database is read inside a
// single transaction, so the inventory in the manifest and the rows that
// follow describe the same moment.
func Export(ctx context.Context, src Source, w io.Writer, opts ExportOptions) (Manifest, error) {
	key, info, err := newSealKey(opts.Secret)
	if err != nil {
		return Manifest{}, err
	}
	if len(src.EncryptionKey) == 0 || src.Secret == "" {
		return Manifest{}, fmt.Errorf("the instance's key material is not available to export")
	}
	progress := opts.Progress
	if progress == nil {
		progress = func(Progress) {}
	}

	// Listed before the transaction opens: introspection runs on the pool, and
	// a single-connection pool would otherwise wait on the transaction forever.
	tables, err := exportTables(ctx, src, opts.IncludeMetrics)
	if err != nil {
		return Manifest{}, err
	}
	tx, err := beginSnapshot(ctx, src.DB)
	if err != nil {
		return Manifest{}, err
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			progress(Progress{Phase: "warning", Item: rbErr.Error()})
		}
	}()
	inventory := make([]TableInventory, 0, len(tables))
	// bounded by the tables of the exported database; one COUNT per table cannot be a single statement portably.
	for _, t := range tables {
		var n int64
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+coredb.QuoteIdent(src.Driver, t)).Scan(&n); err != nil {
			return Manifest{}, fmt.Errorf("counting %s: %w", t, err)
		}
		inventory = append(inventory, TableInventory{Name: t, Rows: n})
	}
	plugins, err := readPlugins(ctx, tx)
	if err != nil {
		return Manifest{}, err
	}
	blobs, err := collectBlobs(src)
	if err != nil {
		return Manifest{}, err
	}
	schema, err := coredb.MigrationNames()
	if err != nil {
		return Manifest{}, fmt.Errorf("listing migrations: %w", err)
	}

	m := Manifest{
		Format: 1, Version: src.Version, Build: src.Build, Backend: string(src.Driver),
		CreatedAt: time.Now().UTC(), CreatedBy: src.CreatedBy, InstanceID: src.InstanceID,
		Hostname: src.Hostname, Schema: schema, Tables: inventory, IncludesMetrics: opts.IncludeMetrics,
		Plugins: plugins, Key: info, Note: opts.Note,
	}
	for _, b := range blobs {
		m.Blobs = append(m.Blobs, b.inv)
	}
	rawManifest, err := json.Marshal(m)
	if err != nil {
		return Manifest{}, fmt.Errorf("encoding the manifest: %w", err)
	}
	if err := writeHeader(w, rawManifest, manifestMAC(key, rawManifest)); err != nil {
		return Manifest{}, err
	}

	cipher, err := cipherFor(key)
	if err != nil {
		return Manifest{}, err
	}
	enc, _, err := cipher.EncryptWriter(w)
	if err != nil {
		return Manifest{}, fmt.Errorf("starting encryption: %w", err)
	}
	gz := gzip.NewWriter(enc)
	sw := newStreamWriter(gz)
	sum := sha256.Sum256(rawManifest)
	if err := sw.header(record{T: recBegin, Manifest: hex.EncodeToString(sum[:])}); err != nil {
		return Manifest{}, err
	}

	total := m.TotalRows()
	var done int64
	for _, t := range inventory {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		progress(Progress{Phase: "tables", Item: t.Name, Done: done, Total: total})
		n, err := dumpTable(ctx, tx, src.Driver, sw, t)
		if err != nil {
			return Manifest{}, err
		}
		done += n
	}
	for _, b := range blobs {
		progress(Progress{Phase: "files", Item: b.inv.Path})
		if err := writeBlob(sw, b); err != nil {
			return Manifest{}, err
		}
	}
	if err := sw.end(); err != nil {
		return Manifest{}, fmt.Errorf("finishing the export: %w", err)
	}
	if err := gz.Close(); err != nil {
		return Manifest{}, fmt.Errorf("finishing compression: %w", err)
	}
	if err := enc.Close(); err != nil {
		return Manifest{}, fmt.Errorf("finishing encryption: %w", err)
	}
	progress(Progress{Phase: "done", Done: total, Total: total})
	return m, nil
}

// beginSnapshot opens the read transaction the export runs in. Repeatable
// read gives Postgres and MySQL one snapshot; SQLite rejects the option but a
// deferred transaction there already reads one snapshot in WAL mode.
func beginSnapshot(ctx context.Context, database *sql.DB) (*sql.Tx, error) {
	tx, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err == nil {
		return tx, nil
	}
	tx, err = database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("opening the export transaction: %w", err)
	}
	return tx, nil
}

func exportTables(ctx context.Context, src Source, metrics bool) ([]string, error) {
	all, err := coredb.ListTables(ctx, src.DB, src.Driver)
	if err != nil {
		return nil, fmt.Errorf("listing tables: %w", err)
	}
	var tables []string
	for _, t := range all {
		lower := strings.ToLower(t)
		if NeverExported[lower] || (!metrics && MetricsTables[lower]) {
			continue
		}
		tables = append(tables, t)
	}
	deps := make(map[string][]string, len(tables))
	for _, t := range tables {
		refs, err := coredb.TableReferences(ctx, src.DB, src.Driver, t)
		if err != nil {
			return nil, err
		}
		deps[t] = refs
	}
	return coredb.TopoSort(tables, deps), nil
}

func readPlugins(ctx context.Context, tx *sql.Tx) ([]PluginRecord, error) {
	rows, err := tx.QueryContext(ctx, "SELECT slug, version, enabled, source FROM appstore_plugins ORDER BY slug LIMIT 1000")
	if err != nil {
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}
	defer func() { logCleanup("rows", rows.Close()) }()
	var out []PluginRecord
	for rows.Next() {
		var p PluginRecord
		var enabled int64
		if err := rows.Scan(&p.Slug, &p.Version, &enabled, &p.Source); err != nil {
			return nil, fmt.Errorf("reading installed plugins: %w", err)
		}
		p.Enabled = enabled != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

func dumpTable(ctx context.Context, tx *sql.Tx, driver coredb.Driver, sw *streamWriter, t TableInventory) (int64, error) {
	query, err := selectExact(ctx, tx, driver, t.Name)
	if err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", t.Name, err)
	}
	defer func() { logCleanup("rows", rows.Close()) }()
	cols, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("reading the columns of %s: %w", t.Name, err)
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return 0, fmt.Errorf("reading the column types of %s: %w", t.Name, err)
	}
	binary := make([]bool, len(types))
	for i, ct := range types {
		name := strings.ToUpper(ct.DatabaseTypeName())
		binary[i] = strings.Contains(name, "BLOB") || strings.Contains(name, "BINARY") || strings.Contains(name, "BYTEA")
	}
	if err := sw.header(record{T: recTable, Name: t.Name, Columns: cols, Rows: t.Rows}); err != nil {
		return 0, err
	}
	holders := make([]any, len(cols))
	scan := make([]any, len(cols))
	for i := range scan {
		scan[i] = &holders[i]
	}
	out := make([]any, len(cols))
	var n int64
	for rows.Next() {
		if err := rows.Scan(scan...); err != nil {
			return n, fmt.Errorf("reading a row of %s: %w", t.Name, err)
		}
		for i := range holders {
			out[i] = encodeValue(holders[i], binary[i])
		}
		if err := sw.row(out); err != nil {
			return n, fmt.Errorf("writing %s: %w", t.Name, err)
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return n, fmt.Errorf("reading %s: %w", t.Name, err)
	}
	if n != t.Rows {
		return n, fmt.Errorf("%s changed while it was exported (%d rows counted, %d read)", t.Name, t.Rows, n)
	}
	return n, nil
}

// selectExact builds the SELECT for one table. On SQLite a column declared as a
// date is read as its stored text: the driver would otherwise parse it into a
// time and hand back a different spelling, and audit hashes cover the spelling.
func selectExact(ctx context.Context, tx *sql.Tx, driver coredb.Driver, table string) (string, error) {
	all := "SELECT * FROM " + coredb.QuoteIdent(driver, table)
	if driver != coredb.DriverSQLite {
		return all, nil
	}
	rows, err := tx.QueryContext(ctx, all+" WHERE 1 = 0")
	if err != nil {
		return "", fmt.Errorf("reading the columns of %s: %w", table, err)
	}
	defer func() { logCleanup("rows", rows.Close()) }()
	types, err := rows.ColumnTypes()
	if err != nil {
		return "", fmt.Errorf("reading the column types of %s: %w", table, err)
	}
	parts := make([]string, len(types))
	for i, ct := range types {
		q := coredb.QuoteIdent(driver, ct.Name())
		if isDateType(ct.DatabaseTypeName()) {
			parts[i] = "CAST(" + q + " AS TEXT) AS " + q
		} else {
			parts[i] = q
		}
	}
	if len(parts) == 0 {
		return all, nil
	}
	// No LIMIT: an export reads every row of the table it copies.
	return "SELECT " + strings.Join(parts, ", ") + " FROM " + coredb.QuoteIdent(driver, table), rows.Err()
}

func isDateType(name string) bool {
	n := strings.ToUpper(name)
	return strings.Contains(n, "DATE") || strings.Contains(n, "TIMESTAMP") || n == "TIME"
}

func collectBlobs(src Source) ([]blobSource, error) {
	out := []blobSource{
		{inv: BlobInventory{Path: KeyEncryption, Kind: BlobKey, Size: int64(len(src.EncryptionKey))}, mode: 0o600, data: src.EncryptionKey},
		{inv: BlobInventory{Path: KeySecret, Kind: BlobKey, Size: int64(len(src.Secret))}, mode: 0o600, data: []byte(src.Secret)},
	}
	tlsFiles := []struct {
		logical, name string
		mode          fs.FileMode
	}{{TLSCACert, "ca.pem", 0o644}, {TLSCAKey, "ca-key.pem", 0o600}}
	if src.UploadedCert {
		tlsFiles = append(tlsFiles, struct {
			logical, name string
			mode          fs.FileMode
		}{TLSCert, "cert.pem", 0o644}, struct {
			logical, name string
			mode          fs.FileMode
		}{TLSKey, "key.pem", 0o600})
	}
	for _, f := range tlsFiles {
		p := filepath.Join(src.DataDir, "tls", f.name)
		st, err := os.Stat(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f.logical, err)
		}
		out = append(out, blobSource{inv: BlobInventory{Path: f.logical, Kind: BlobTLS, Size: st.Size()}, mode: f.mode, file: p})
	}
	var files []blobSource
	for _, root := range FileRoots {
		base := filepath.Join(src.DataDir, root)
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src.DataDir, p)
			if err != nil {
				return err
			}
			files = append(files, blobSource{
				inv:  BlobInventory{Path: path.Clean(filepath.ToSlash(rel)), Kind: BlobFile, Size: info.Size()},
				mode: info.Mode().Perm(), file: p,
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", root, err)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].inv.Path < files[j].inv.Path })
	return append(out, files...), nil
}

func writeBlob(sw *streamWriter, b blobSource) error {
	if err := sw.header(record{T: recBlob, Name: b.inv.Path, Kind: b.inv.Kind, Size: b.inv.Size, Mode: uint32(b.mode)}); err != nil {
		return err
	}
	if b.file == "" {
		return sw.write(b.data)
	}
	f, err := os.Open(b.file)
	if err != nil {
		return fmt.Errorf("reading %s: %w", b.inv.Path, err)
	}
	defer func() { logCleanup("f", f.Close()) }()
	n, err := io.Copy(writerFunc(sw.write), io.LimitReader(f, b.inv.Size))
	if err != nil {
		return fmt.Errorf("reading %s: %w", b.inv.Path, err)
	}
	if n != b.inv.Size {
		return fmt.Errorf("%s changed size while it was exported", b.inv.Path)
	}
	return nil
}

type writerFunc func([]byte) error

func (f writerFunc) Write(p []byte) (int, error) {
	if err := f(p); err != nil {
		return 0, err
	}
	return len(p), nil
}
