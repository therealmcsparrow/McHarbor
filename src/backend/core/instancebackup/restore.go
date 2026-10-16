// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// AnchorAction is the audit action of the row a restore writes into the
// chain it replaced. It is a link in the chain, so the seam verifies and is never silent.
const AnchorAction = "instance.restore.anchor"

// MarkerKey is the settings row a restore commits with its own id, which is
// how an interrupted restore is told apart from one that committed.
const MarkerKey = "instance_restore_marker"

// Intent is written outside the database before a restore changes anything.
type Intent struct {
	ID                     string   `json:"id"`
	At                     string   `json:"at"`
	Actor                  string   `json:"actor"`
	Via                    string   `json:"via"`
	Mode                   string   `json:"mode"`
	Categories             []string `json:"categories,omitempty"`
	IncludeAudit           bool     `json:"includeAudit"`
	ManifestSHA256         string   `json:"manifestSha256"`
	ExportCreatedAt        string   `json:"exportCreatedAt"`
	ExportInstanceID       string   `json:"exportInstanceId"`
	PreRestoreTerminalHash string   `json:"preRestoreTerminalHash"`
}

// Result is what a completed restore reports.
type Result struct {
	ID              string         `json:"id"`
	Plan            Plan           `json:"plan"`
	AnchorID        string         `json:"anchorId,omitempty"`
	Tables          int            `json:"tables"`
	Rows            int64          `json:"rows"`
	RestartRequired bool           `json:"restartRequired"`
	Plugins         []PluginRecord `json:"plugins"`
}

// transient rows describe work that was in flight when the export was taken.
// Nothing resumes it, so it comes back failed rather than forever running.
var transient = []struct{ table, from, to string }{
	{"container_backup_runs", "running", "failure"},
	{"schedule_executions", "running", "failure"},
	{"instance_backup_exports", "running", "failed"},
}

type tableTarget struct {
	cols     map[string]bool
	datetime map[string]bool
	pk       string
}

// Restore applies an artifact to t. open is called twice: once to verify and
// plan, once to write, so a truncated or altered file is refused before
// anything changes. The database is written in one transaction; key material
// and files are swapped only after it commits, under a journal that lets
// Recover finish or undo an interrupted restore on the next start.
func Restore(ctx context.Context, t Target, open func() (io.ReadCloser, error), opts RestoreOptions) (Result, error) {
	if opts.Mode == ModeSelective && opts.Conflict == "" {
		opts.Conflict = ConflictReplace
	}
	progress := opts.Progress
	if progress == nil {
		progress = func(Progress) {}
	}
	progress(Progress{Phase: "verifying"})
	rc, err := open()
	if err != nil {
		return Result{}, err
	}
	plan, blobs, err := PlanRestore(ctx, t, rc, opts)
	if closeErr := rc.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return Result{}, err
	}
	if len(plan.Refusals) > 0 {
		return Result{Plan: plan}, &RefusedError{Reasons: plan.Refusals}
	}
	if opts.IncludeAudit && opts.Emit == nil {
		return Result{}, fmt.Errorf("restoring audit history needs somewhere outside the database to record the intent")
	}

	id := xid.New().String()
	pre, err := terminalHash(ctx, t.DB)
	if err != nil {
		return Result{}, err
	}
	rawManifest, err := json.Marshal(plan.Manifest)
	if err != nil {
		return Result{}, err
	}
	sum := sha256.Sum256(rawManifest)
	intent := Intent{
		ID: id, At: time.Now().UTC().Format(time.RFC3339), Actor: opts.Actor, Via: opts.Via, Mode: opts.Mode,
		Categories: opts.Categories, IncludeAudit: opts.IncludeAudit, ManifestSHA256: hex.EncodeToString(sum[:]),
		ExportCreatedAt: plan.Manifest.CreatedAt.Format(time.RFC3339), ExportInstanceID: plan.Manifest.InstanceID,
		PreRestoreTerminalHash: pre,
	}
	if err := AppendLedger(t.DataDir, LedgerEntry{Phase: "intent", Intent: &intent}); err != nil {
		return Result{}, err
	}
	if opts.Emit != nil {
		if err := opts.Emit(ctx, intent); err != nil {
			return Result{}, fmt.Errorf("recording the restore intent: %w", err)
		}
	}

	staging := filepath.Join(t.DataDir, restoreDir, id)
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return Result{}, fmt.Errorf("preparing the restore staging area: %w", err)
	}
	res, err := writeDatabase(ctx, t, open, opts, plan, blobs, intent, staging)
	if err != nil {
		cleanup(staging)
		if ledgerErr := AppendLedger(t.DataDir, LedgerEntry{Phase: "failed", ID: id, Error: err.Error()}); ledgerErr != nil {
			return Result{}, errors.Join(err, ledgerErr)
		}
		return Result{}, err
	}
	res.ID, res.Plan, res.Plugins = id, plan, plan.Manifest.Plugins
	if err := finish(t.DataDir, id); err != nil {
		return res, err
	}
	res.RestartRequired = true
	return res, nil
}

// RefusedError carries the plan's refusals.
type RefusedError struct{ Reasons []string }

func (e *RefusedError) Error() string {
	return "the restore was refused: " + strings.Join(e.Reasons, ", ")
}

func terminalHash(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) (string, error) {
	var h sql.NullString
	err := q.QueryRowContext(ctx, `SELECT row_hash FROM audit_logs WHERE row_hash IS NOT NULL AND row_hash <> ''
		ORDER BY timestamp DESC, id DESC LIMIT 1`).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading the audit chain's last hash: %w", err)
	}
	return h.String, nil
}

func writeDatabase(ctx context.Context, t Target, open func() (io.ReadCloser, error), opts RestoreOptions, plan Plan, blobs Blobs, intent Intent, staging string) (Result, error) {
	var res Result
	targets := map[string]tableTarget{}
	var order []string
	for _, tbl := range plan.Manifest.Tables {
		if !selected(tbl.Name, opts) {
			continue
		}
		cols, err := coredb.TableColumns(ctx, t.DB, t.Driver, tbl.Name)
		if err != nil {
			continue
		}
		tt := tableTarget{cols: map[string]bool{}}
		for _, c := range cols {
			tt.cols[strings.ToLower(c)] = true
		}
		if t.Driver == coredb.DriverMySQL {
			dt, err := mysqlDateColumns(ctx, t.DB, tbl.Name)
			if err != nil {
				return res, err
			}
			tt.datetime = dt
		}
		if opts.Conflict == ConflictKeep {
			pk, err := coredb.PrimaryKeyColumn(ctx, t.DB, t.Driver, tbl.Name)
			if err == nil {
				tt.pk = pk
			}
		}
		targets[strings.ToLower(tbl.Name)] = tt
		order = append(order, tbl.Name)
	}
	var clearOnly []string
	if opts.Mode != ModeSelective {
		all, err := coredb.ListTables(ctx, t.DB, t.Driver)
		if err != nil {
			return res, fmt.Errorf("listing target tables: %w", err)
		}
		for _, name := range all {
			lower := strings.ToLower(name)
			if NeverExported[lower] && lower != "schema_migrations" && lower != "_migrations" {
				clearOnly = append(clearOnly, name)
			}
		}
	}
	var reenc *reencrypter
	if opts.Mode == ModeSelective && !plan.SameKey {
		r, err := newReencrypter(blobs[KeyEncryption], t.EncryptionKey)
		if err != nil {
			return res, err
		}
		reenc = r
	}

	conn, err := t.DB.Conn(ctx)
	if err != nil {
		return res, fmt.Errorf("opening a restore connection: %w", err)
	}
	defer func() { logCleanup("conn", conn.Close()) }()
	relaxed := relaxFK(ctx, conn, t.Driver)
	defer restoreFK(ctx, conn, t.Driver, relaxed)
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return res, fmt.Errorf("opening the restore transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				logCleanup("restore transaction", rbErr)
			}
		}
	}()
	if t.Driver == coredb.DriverPostgres {
		relaxed = relaxPostgres(ctx, tx)
		if !relaxed && opts.Mode == ModeSelective && opts.Conflict != ConflictKeep {
			return res, &RefusedError{Reasons: []string{RefusePostgresCascade}}
		}
	}
	if opts.IncludeAudit && t.Driver == coredb.DriverPostgres {
		if _, err := tx.ExecContext(ctx, "LOCK TABLE audit_logs IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return res, fmt.Errorf("locking the audit log: %w", err)
		}
	}

	if opts.Conflict != ConflictKeep {
		for i := len(order) - 1; i >= 0; i-- {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+coredb.QuoteIdent(t.Driver, order[i])); err != nil {
				return res, fmt.Errorf("clearing %s: %w", order[i], err)
			}
		}
	}
	for _, name := range clearOnly {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+coredb.QuoteIdent(t.Driver, name)); err != nil {
			return res, fmt.Errorf("clearing %s: %w", name, err)
		}
	}

	rc, err := open()
	if err != nil {
		return res, err
	}
	defer func() { logCleanup("rc", rc.Close()) }()
	a, err := Open(rc, opts.Secret)
	if err != nil {
		return res, err
	}
	defer func() { logCleanup("a", a.Close()) }()
	total := plan.Manifest.TotalRows()
	for {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		e, err := a.Next()
		if err != nil {
			return res, err
		}
		if e.Kind == EntryEnd {
			break
		}
		switch e.Kind {
		case EntryTable:
			tt, ok := targets[strings.ToLower(e.Name)]
			if !ok {
				continue
			}
			progress := opts.Progress
			if progress != nil {
				progress(Progress{Phase: "tables", Item: e.Name, Done: res.Rows, Total: total})
			}
			n, err := insertTable(ctx, tx, t.Driver, a, e, tt, opts.Conflict, reenc)
			if err != nil {
				return res, err
			}
			res.Rows += n
			res.Tables++
		case EntryBlob:
			if err := stageBlob(staging, e, a.Blob(), plan); err != nil {
				return res, err
			}
		}
	}

	for _, fix := range transient {
		if _, ok := targets[fix.table]; !ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, rebind(t.Driver, "UPDATE "+coredb.QuoteIdent(t.Driver, fix.table)+" SET status = ? WHERE status = ?"), fix.to, fix.from); err != nil {
			return res, fmt.Errorf("closing in-flight %s: %w", fix.table, err)
		}
	}
	if opts.IncludeAudit {
		anchor, err := writeAnchor(ctx, tx, t, intent, string(blobs[KeySecret]))
		if err != nil {
			return res, err
		}
		res.AnchorID = anchor
	}
	if err := writeMarker(ctx, tx, t.Driver, intent.ID); err != nil {
		return res, err
	}

	j := journal{ID: intent.ID, Phase: phaseCommitting, RestoresKeys: plan.RestoresKeys, RestoresSecret: plan.RestoresSecret,
		RestoresFiles: plan.RestoresFiles, OldKey: base64.StdEncoding.EncodeToString(t.EncryptionKey)}
	if err := writeJournal(t.DataDir, j); err != nil {
		return res, err
	}
	if err := tx.Commit(); err != nil {
		if rmErr := removeJournal(t.DataDir); rmErr != nil {
			return res, errors.Join(fmt.Errorf("committing the restore: %w", err), rmErr)
		}
		return res, fmt.Errorf("committing the restore: %w", err)
	}
	committed = true
	j.Phase = phaseCommitted
	if err := writeJournal(t.DataDir, j); err != nil {
		return res, err
	}
	return res, nil
}

func insertTable(ctx context.Context, tx *sql.Tx, driver coredb.Driver, a *Artifact, e Entry, tt tableTarget, conflict string, reenc *reencrypter) (int64, error) {
	var keep []int
	var cols []string
	pkIndex := -1
	for i, c := range e.Columns {
		if tt.cols[strings.ToLower(c)] {
			if tt.pk != "" && strings.EqualFold(c, tt.pk) {
				pkIndex = len(cols)
			}
			keep = append(keep, i)
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		return 0, nil
	}
	if conflict == ConflictKeep && pkIndex < 0 {
		return 0, nil
	}
	batch := coredb.MaxBindParameters(driver) / len(cols)
	if batch > 200 {
		batch = 200
	}
	if batch < 1 {
		batch = 1
	}
	var pending []any
	rows := 0
	var n int64
	flush := func() error {
		if rows == 0 {
			return nil
		}
		if _, err := tx.ExecContext(ctx, insertSQL(driver, e.Name, cols, rows), pending...); err != nil {
			return fmt.Errorf("writing %s: %w", e.Name, err)
		}
		n += int64(rows)
		pending, rows = pending[:0], 0
		return nil
	}
	existsSQL := ""
	if conflict == ConflictKeep {
		existsSQL = "SELECT 1 FROM " + coredb.QuoteIdent(driver, e.Name) + " WHERE " + coredb.QuoteIdent(driver, tt.pk) + " = " + coredb.Placeholder(driver, 1)
	}
	for {
		row, err := a.Row()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return n, err
		}
		values := make([]any, len(keep))
		for i, idx := range keep {
			v := row[idx]
			if reenc != nil {
				if s, ok := v.(string); ok && encryption.IsEncrypted(s) {
					if v, err = reenc.convert(s); err != nil {
						return n, fmt.Errorf("%s.%s: %w", e.Name, cols[i], err)
					}
				}
			}
			values[i] = forTarget(v, driver, tt.datetime[strings.ToLower(cols[i])])
		}
		if existsSQL != "" {
			var one int
			err := tx.QueryRowContext(ctx, existsSQL, values[pkIndex]).Scan(&one)
			if err == nil {
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return n, fmt.Errorf("checking %s: %w", e.Name, err)
			}
		}
		pending = append(pending, values...)
		rows++
		if rows >= batch {
			if err := flush(); err != nil {
				return n, err
			}
		}
	}
	return n, flush()
}

func insertSQL(driver coredb.Driver, table string, cols []string, rows int) string {
	var b strings.Builder
	b.WriteString("INSERT INTO " + coredb.QuoteIdent(driver, table) + " (")
	for i, c := range cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(coredb.QuoteIdent(driver, c))
	}
	b.WriteString(") VALUES ")
	n := 0
	for r := 0; r < rows; r++ {
		if r > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(")
		for c := range cols {
			if c > 0 {
				b.WriteString(", ")
			}
			n++
			b.WriteString(coredb.Placeholder(driver, n))
		}
		b.WriteString(")")
	}
	return b.String()
}

func writeAnchor(ctx context.Context, tx *sql.Tx, t Target, in Intent, secret string) (string, error) {
	restored, err := terminalHash(ctx, tx)
	if err != nil {
		return "", err
	}
	details, err := json.Marshal(map[string]any{
		"restoreId": in.ID, "preRestoreTerminalHash": in.PreRestoreTerminalHash,
		"manifestSha256": in.ManifestSHA256, "restoredTerminalHash": restored,
		"mode": in.Mode, "via": in.Via, "exportCreatedAt": in.ExportCreatedAt,
	})
	if err != nil {
		return "", err
	}
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	actor, name, entityType, entityID, d := in.Actor, "instance restore", "instance_backup", in.ID, string(details)
	row := audit.HashRow{
		ID: id, PrevHash: restored, Username: &actor, Action: AnchorAction, Severity: "warning",
		EntityType: &entityType, EntityID: &entityID, EntityName: &name, Details: &d,
		Timestamp: now, CreatedAt: now, UpdatedAt: now,
	}
	hash, err := audit.ComputeRowHash(secret, row)
	if err != nil {
		return "", err
	}
	var prev any
	if restored != "" {
		prev = restored
	}
	if _, err := tx.ExecContext(ctx, rebind(t.Driver, `INSERT INTO audit_logs (id, username, action, severity, entity_type, entity_id,
		entity_name, details, timestamp, created_at, updated_at, prev_hash, row_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		id, actor, AnchorAction, "warning", entityType, entityID, name, d,
		forTarget(now, t.Driver, true), forTarget(now, t.Driver, true), forTarget(now, t.Driver, true), prev, hash); err != nil {
		return "", fmt.Errorf("writing the restore anchor: %w", err)
	}
	return id, nil
}

func writeMarker(ctx context.Context, tx *sql.Tx, driver coredb.Driver, id string) error {
	k := coredb.QuoteIdent(driver, "key")
	if _, err := tx.ExecContext(ctx, rebind(driver, "DELETE FROM settings WHERE "+k+" = ?"), MarkerKey); err != nil { // SQL identifier: k is QuoteIdent(driver, "key")
		return fmt.Errorf("writing the restore marker: %w", err)
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, rebind(driver, "INSERT INTO settings (id, "+k+", value, category, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)"), // SQL identifier: k is QuoteIdent(driver, "key")
		xid.New().String(), MarkerKey, id, "system", forTarget(ts, driver, true), forTarget(ts, driver, true)); err != nil {
		return fmt.Errorf("writing the restore marker: %w", err)
	}
	return nil
}

// rebind turns ? placeholders into the driver's own.
func rebind(driver coredb.Driver, q string) string {
	if driver != coredb.DriverPostgres {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			b.WriteString(coredb.Placeholder(driver, n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// relaxPostgres suspends foreign-key triggers for the transaction. Only a
// superuser may; without it a selective replace would cascade deletes into
// categories the operator did not choose, so the caller refuses that case.
func relaxPostgres(ctx context.Context, tx *sql.Tx) bool {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT mcharbor_fk"); err != nil {
		return false
	}
	if _, err := tx.ExecContext(ctx, "SET LOCAL session_replication_role = 'replica'"); err != nil {
		if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT mcharbor_fk"); rbErr != nil {
			return false
		}
		return false
	}
	return true
}

func relaxFK(ctx context.Context, conn *sql.Conn, driver coredb.Driver) bool {
	switch driver {
	case coredb.DriverPostgres:
		return false
	case coredb.DriverMySQL:
		_, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0")
		return err == nil
	default:
		_, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
		return err == nil
	}
}

func restoreFK(parent context.Context, conn *sql.Conn, driver coredb.Driver, relaxed bool) {
	if !relaxed {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	stmt := "PRAGMA foreign_keys = ON"
	switch driver {
	case coredb.DriverPostgres:
		return
	case coredb.DriverMySQL:
		stmt = "SET FOREIGN_KEY_CHECKS = 1"
	}
	// The connection is returned to the pool next; a failure here only means
	// it keeps the relaxed setting, which the pool's next Close discards.
	if _, err := conn.ExecContext(ctx, stmt); err != nil {
		return
	}
}

// forTarget fits a value to the column it lands in. MySQL's DATETIME rejects
// RFC 3339 text, so it gets a time; SQLite and Postgres get the RFC 3339 text
// McHarbor itself writes, so a timestamp keeps the spelling its audit hash covers.
func forTarget(v any, driver coredb.Driver, datetime bool) any {
	switch x := v.(type) {
	case time.Time:
		if driver == coredb.DriverMySQL {
			return x.UTC()
		}
		return x.UTC().Format(time.RFC3339Nano)
	case string:
		if driver == coredb.DriverMySQL && datetime {
			for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05", "2006-01-02"} {
				if parsed, err := time.Parse(layout, x); err == nil {
					return parsed.UTC()
				}
			}
		}
	}
	return v
}

// mysqlDateColumns names the columns of table MySQL stores as dates or times.
func mysqlDateColumns(ctx context.Context, database *sql.DB, table string) (map[string]bool, error) {
	rows, err := database.QueryContext(ctx, `SELECT COLUMN_NAME, DATA_TYPE FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table)
	if err != nil {
		return nil, fmt.Errorf("reading the column types of %s: %w", table, err)
	}
	defer func() { logCleanup("rows", rows.Close()) }()
	out := map[string]bool{}
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			return nil, err
		}
		switch strings.ToLower(typ) {
		case "datetime", "timestamp", "date":
			out[strings.ToLower(name)] = true
		}
	}
	return out, rows.Err()
}

type reencrypter struct{ from, to *encryption.Service }

func newReencrypter(from, to []byte) (*reencrypter, error) {
	f, err := encryption.New("", base64.StdEncoding.EncodeToString(from))
	if err != nil {
		return nil, fmt.Errorf("loading the export's key: %w", err)
	}
	tgt, err := encryption.New("", base64.StdEncoding.EncodeToString(to))
	if err != nil {
		return nil, fmt.Errorf("loading this instance's key: %w", err)
	}
	return &reencrypter{from: f, to: tgt}, nil
}

func (r *reencrypter) convert(s string) (string, error) {
	plain, err := r.from.Decrypt(s)
	if err != nil {
		return "", fmt.Errorf("decrypting with the export's key: %w", err)
	}
	return r.to.Encrypt(plain)
}

// logCleanup records a failed Close or removal, because the deferred and
// error-path callers that reach it have nowhere to return the error.
func logCleanup(what string, err error) {
	if err != nil {
		slog.Warn("instance backup cleanup failed", "target", what, "error", err)
	}
}
