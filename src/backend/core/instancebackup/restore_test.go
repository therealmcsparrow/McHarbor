// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

type instance struct {
	dir    string
	db     *sql.DB
	key    []byte
	secret string
	enc    *encryption.Service
}

func newInstance(t *testing.T, keyByte byte, secret, ca string) *instance {
	t.Helper()
	dir := t.TempDir()
	database, err := coredb.OpenSQLite(filepath.Join(dir, "mcharbor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTest(t, database) })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	if _, err := database.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{keyByte}, 32)
	enc, err := encryption.New("", base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{".encryption_key": string(key), ".secret": secret, "tls/ca.pem": ca, "tls/ca-key.pem": ca + " KEY", "tls/cert.pem": "LEAF OF " + ca} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return &instance{dir: dir, db: database, key: key, secret: secret, enc: enc}
}

func (in *instance) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := in.db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func (in *instance) value(t *testing.T, key string) string {
	t.Helper()
	var v string
	err := in.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// audit appends a chained audit row the way the logger does.
func (in *instance) audit(t *testing.T, id, action, ts string) {
	t.Helper()
	prev, err := terminalHash(context.Background(), in.db)
	if err != nil {
		t.Fatal(err)
	}
	row := audit.HashRow{ID: id, PrevHash: prev, Action: action, Severity: "info", Timestamp: ts, CreatedAt: ts, UpdatedAt: ts}
	h, err := audit.ComputeRowHash(in.secret, row)
	if err != nil {
		t.Fatal(err)
	}
	var p any
	if prev != "" {
		p = prev
	}
	in.exec(t, `INSERT INTO audit_logs (id, action, severity, timestamp, created_at, updated_at, prev_hash, row_hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, action, "info", ts, ts, ts, p, h)
}

// chainAnchors recomputes every link with secret and returns the anchors.
func chainAnchors(t *testing.T, database *sql.DB, secret string) []string {
	t.Helper()
	rows, err := database.Query(`SELECT id, username, action, severity, entity_type, entity_id, entity_name, details,
		timestamp, created_at, updated_at, prev_hash, row_hash FROM audit_logs ORDER BY timestamp ASC, id ASC`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close rows: %v", err)
		}
	}()
	prev := ""
	var anchors []string
	for rows.Next() {
		var r audit.HashRow
		var username, et, eid, en, d, ph sql.NullString
		var rh string
		if err := rows.Scan(&r.ID, &username, &r.Action, &r.Severity, &et, &eid, &en, &d, &r.Timestamp, &r.CreatedAt, &r.UpdatedAt, &ph, &rh); err != nil {
			t.Fatal(err)
		}
		for _, f := range []struct {
			src *sql.NullString
			dst **string
		}{{&username, &r.Username}, {&et, &r.EntityType}, {&eid, &r.EntityID}, {&en, &r.EntityName}, {&d, &r.Details}} {
			if f.src.Valid {
				v := f.src.String
				*f.dst = &v
			}
		}
		if ph.String != prev {
			t.Fatalf("row %s links to %q, the chain ended at %q", r.ID, ph.String, prev)
		}
		r.PrevHash = ph.String
		want, err := audit.ComputeRowHash(secret, r)
		if err != nil {
			t.Fatal(err)
		}
		if want != rh {
			t.Fatalf("row %s does not verify", r.ID)
		}
		if r.Action == AnchorAction {
			anchors = append(anchors, *r.Details)
		}
		prev = rh
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return anchors
}

func (in *instance) source() Source {
	return Source{DB: in.db, Driver: coredb.DriverSQLite, DataDir: in.dir, EncryptionKey: in.key, Secret: in.secret, Version: "2026.09.01", InstanceID: "src"}
}

func (in *instance) target() Target {
	return Target{DB: in.db, Driver: coredb.DriverSQLite, DataDir: in.dir, EncryptionKey: in.key, Secret: in.secret}
}

func opener(raw []byte) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
}

var emitted []Intent

func emit(_ context.Context, in Intent) error {
	emitted = append(emitted, in)
	return nil
}

func sourceWithData(t *testing.T) (*instance, []byte) {
	t.Helper()
	src := newInstance(t, 1, "source-secret", "SOURCE CA")
	cipher, err := src.enc.Encrypt("vault-password")
	if err != nil {
		t.Fatal(err)
	}
	src.exec(t, `INSERT INTO settings (id, key, value) VALUES ('st1', 'branding_name', 'Harbor Ops'), ('st2', 'smtp_password', ?)`, cipher)
	src.exec(t, `INSERT INTO workflows (id, name, created_at, updated_at) VALUES ('w1', 'nightly', '2026-09-01', '2026-09-01')`)
	src.audit(t, "a1", "login", "2026-09-01T10:00:00Z")
	src.audit(t, "a2", "create", "2026-09-01T11:00:00Z")
	stack := filepath.Join(src.dir, "stacks", "web", "compose.yml")
	if err := os.MkdirAll(filepath.Dir(stack), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stack, []byte("services: {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := Export(context.Background(), src.source(), &buf, ExportOptions{Secret: Secret{Passphrase: passphrase}}); err != nil {
		t.Fatal(err)
	}
	return src, buf.Bytes()
}

func TestFullRestoreOntoAFreshInstanceBringsBackDataKeysAndTheChain(t *testing.T) {
	_, raw := sourceWithData(t)
	dst := newInstance(t, 2, "target-secret", "TARGET CA")
	dst.exec(t, `INSERT INTO settings (key, value) VALUES ('branding_name', 'Fresh')`)
	dst.exec(t, `INSERT INTO sessions (id, user_id, expires_at) VALUES ('s1', 'u1', '2099-01-01')`)
	dst.audit(t, "t1", "setup", "2026-09-20T09:00:00Z")
	targetTail, err := terminalHash(context.Background(), dst.db)
	if err != nil {
		t.Fatal(err)
	}
	emitted = nil

	res, err := Restore(context.Background(), dst.target(), opener(raw), RestoreOptions{
		Secret: Secret{Passphrase: passphrase}, Mode: ModeFull, IncludeAudit: true, Actor: "admin", Via: "web", Emit: emit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.RestartRequired || res.AnchorID == "" {
		t.Fatalf("result: %+v", res)
	}
	if got := dst.value(t, "branding_name"); got != "Harbor Ops" {
		t.Fatalf("branding after restore: %q", got)
	}
	var sessions int
	if err := dst.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("sessions survived a full restore: %d %v", sessions, err)
	}
	key, err := os.ReadFile(filepath.Join(dst.dir, ".encryption_key"))
	if err != nil || !bytes.Equal(key, bytes.Repeat([]byte{1}, 32)) {
		t.Fatal("the export's encryption key was not put in place")
	}
	restoredEnc, err := encryption.New("", base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := restoredEnc.Decrypt(dst.value(t, "smtp_password")); err != nil || plain != "vault-password" {
		t.Fatalf("an encrypted column does not decrypt with the restored key: %q %v", plain, err)
	}
	if s, err := os.ReadFile(filepath.Join(dst.dir, ".secret")); err != nil || string(s) != "source-secret" {
		t.Fatalf("the audit secret: %q %v", s, err)
	}
	if ca, err := os.ReadFile(filepath.Join(dst.dir, "tls", "ca.pem")); err != nil || string(ca) != "SOURCE CA" {
		t.Fatalf("the CA agents pin was not restored: %q", ca)
	}
	if _, err := os.Stat(filepath.Join(dst.dir, "tls", "cert.pem")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a serving certificate from the replaced CA was left in place")
	}
	if _, err := os.Stat(filepath.Join(dst.dir, "stacks", "web", "compose.yml")); err != nil {
		t.Fatal("stack files were not restored")
	}

	anchors := chainAnchors(t, dst.db, "source-secret")
	if len(anchors) != 1 || !strings.Contains(anchors[0], targetTail) {
		t.Fatalf("the anchor must witness the destroyed chain's last hash %q: %v", targetTail, anchors)
	}
	if len(emitted) != 1 || emitted[0].PreRestoreTerminalHash != targetTail {
		t.Fatalf("the intent did not leave the database first: %+v", emitted)
	}
	ledger, err := ReadLedger(dst.dir)
	if err != nil || len(ledger) < 2 || ledger[0].Phase != "intent" || ledger[len(ledger)-1].Phase != "done" {
		t.Fatalf("ledger: %+v %v", ledger, err)
	}
	if Pending(dst.dir) {
		t.Fatal("the journal was left behind")
	}
}

func TestSelectiveRestoreTouchesOnlyItsCategoriesAndReencrypts(t *testing.T) {
	_, raw := sourceWithData(t)
	dst := newInstance(t, 2, "target-secret", "TARGET CA")
	dst.exec(t, `INSERT INTO workflows (id, name, created_at, updated_at) VALUES ('w9', 'mine', '2026-09-01', '2026-09-01')`)
	dst.audit(t, "t1", "setup", "2026-09-20T09:00:00Z")

	res, err := Restore(context.Background(), dst.target(), opener(raw), RestoreOptions{
		Secret: Secret{Passphrase: passphrase}, Mode: ModeSelective, Categories: []string{CategorySettings}, Actor: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.AnchorID != "" {
		t.Fatal("a restore without audit wrote an anchor")
	}
	if dst.value(t, "branding_name") != "Harbor Ops" {
		t.Fatal("the chosen category was not restored")
	}
	var name string
	if err := dst.db.QueryRow(`SELECT name FROM workflows`).Scan(&name); err != nil || name != "mine" {
		t.Fatalf("a category that was not chosen changed: %q %v", name, err)
	}
	if plain, err := dst.enc.Decrypt(dst.value(t, "smtp_password")); err != nil || plain != "vault-password" {
		t.Fatalf("an encrypted value from another instance must be re-encrypted with this one's key: %v", err)
	}
	if key, err := os.ReadFile(filepath.Join(dst.dir, ".encryption_key")); err != nil || !bytes.Equal(key, dst.key) {
		t.Fatal("a selective restore replaced the instance key")
	}
	if anchors := chainAnchors(t, dst.db, "target-secret"); len(anchors) != 0 {
		t.Fatal("the audit chain changed")
	}
}

func TestRestoreRefusalsAndPreconditions(t *testing.T) {
	_, raw := sourceWithData(t)
	dst := newInstance(t, 2, "target-secret", "TARGET CA")
	ctx := context.Background()

	pinned := dst.target()
	pinned.KeyPinned = true
	_, err := Restore(ctx, pinned, opener(raw), RestoreOptions{Secret: Secret{Passphrase: passphrase}, Mode: ModeFull})
	var refused *RefusedError
	if !errors.As(err, &refused) || refused.Reasons[0] != RefuseKeyPinned {
		t.Fatalf("a key pinned by the environment: %v", err)
	}
	if _, err := Restore(ctx, dst.target(), opener(raw), RestoreOptions{Secret: Secret{Passphrase: passphrase}, Mode: ModeFull, IncludeAudit: true}); err == nil {
		t.Fatal("an audit restore ran with nowhere to record its intent")
	}
	if _, err := Restore(ctx, dst.target(), opener(raw[:len(raw)-20]), RestoreOptions{Secret: Secret{Passphrase: passphrase}, Mode: ModeFull}); err == nil {
		t.Fatal("a truncated export was restored")
	}
	if dst.value(t, "branding_name") != "" {
		t.Fatal("a refused restore wrote to the database")
	}

	p, err := buildPlan(ctx, dst.target(), Manifest{Schema: []string{"999_from_the_future.sql"}}, Blobs{}, RestoreOptions{Mode: ModeFull})
	if err != nil || len(p.Refusals) == 0 || p.Refusals[0] != RefuseNewerSchema {
		t.Fatalf("an export from a newer schema: %+v %v", p.Refusals, err)
	}
}

func TestAnInterruptedRestoreIsFinishedOrUndoneOnTheNextStart(t *testing.T) {
	_, raw := sourceWithData(t)
	ctx := context.Background()
	opts := RestoreOptions{Secret: Secret{Passphrase: passphrase}, Mode: ModeFull, Actor: "admin"}

	committed := newInstance(t, 2, "target-secret", "TARGET CA")
	plan, blobs, err := PlanRestore(ctx, committed.target(), bytes.NewReader(raw), opts)
	if err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(committed.dir, restoreDir, "r1")
	if _, err := writeDatabase(ctx, committed.target(), opener(raw), opts, plan, blobs, Intent{ID: "r1"}, staging); err != nil {
		t.Fatal(err)
	}
	// The process died here: the database committed, the keys were not swapped.
	if _, err := Recover(committed.dir, func(string) (bool, error) { t.Fatal("a committed journal needs no probe"); return false, nil }); err != nil {
		t.Fatal(err)
	}
	if key, err := os.ReadFile(filepath.Join(committed.dir, ".encryption_key")); err != nil || !bytes.Equal(key, bytes.Repeat([]byte{1}, 32)) {
		t.Fatal("recovery did not finish swapping the key for a committed restore")
	}

	undone := newInstance(t, 2, "target-secret", "TARGET CA")
	if err := os.MkdirAll(filepath.Join(undone.dir, restoreDir, "r2"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeJournal(undone.dir, journal{ID: "r2", Phase: phaseCommitting, RestoresKeys: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := Recover(undone.dir, func(id string) (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	if key, err := os.ReadFile(filepath.Join(undone.dir, ".encryption_key")); err != nil || !bytes.Equal(key, undone.key) {
		t.Fatal("recovery swapped keys for a restore that never committed")
	}
	if Pending(undone.dir) {
		t.Fatal("the journal of an undone restore was left behind")
	}
}

func TestRehearsalStandsAnExportUpWithoutTouchingTheInstance(t *testing.T) {
	src, raw := sourceWithData(t)
	ctx := context.Background()
	scratch := t.TempDir()
	before, err := os.ReadFile(filepath.Join(src.dir, ".encryption_key"))
	if err != nil {
		t.Fatal(err)
	}

	r, err := Rehearse(ctx, scratch, opener(raw), Secret{Passphrase: passphrase})
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK || !r.ChainVerified || r.ChainRows != 3 || r.Anchors != 1 || r.Rows == 0 || len(r.Mismatches) != 0 {
		t.Fatalf("a good export: %+v", r)
	}
	if after, err := os.ReadFile(filepath.Join(src.dir, ".encryption_key")); err != nil || !bytes.Equal(before, after) {
		t.Fatal("the rehearsal touched the instance it came from")
	}
	if entries, err := os.ReadDir(scratch); err != nil || len(entries) != 0 {
		t.Fatalf("the rehearsal left %d entries behind", len(entries))
	}

	bad, err := Rehearse(ctx, scratch, opener(raw), Secret{Passphrase: passphrase + "x"})
	if err != nil || bad.OK || bad.Problem == "" {
		t.Fatalf("a wrong key must fail the rehearsal with a reason: %+v %v", bad, err)
	}
	cut, err := Rehearse(ctx, scratch, opener(raw[:len(raw)/2]), Secret{Passphrase: passphrase})
	if err != nil || cut.OK {
		t.Fatalf("a truncated export must fail the rehearsal: %+v %v", cut, err)
	}
}

func TestRehearsalCatchesAnExportWhoseAuditChainIsBroken(t *testing.T) {
	src := newInstance(t, 1, "source-secret", "SOURCE CA")
	src.audit(t, "a1", "login", "2026-09-01T10:00:00Z")
	src.audit(t, "a2", "create", "2026-09-01T11:00:00Z")
	src.audit(t, "a3", "delete", "2026-09-01T12:00:00Z")
	// A deleted row leaves every remaining row's own hash valid; only the link
	// from a3 back to a1 shows the gap.
	src.exec(t, `DELETE FROM audit_logs WHERE id = 'a2'`)
	var buf bytes.Buffer
	if _, err := Export(context.Background(), src.source(), &buf, ExportOptions{Secret: Secret{Passphrase: passphrase}}); err != nil {
		t.Fatal(err)
	}
	r, err := Rehearse(context.Background(), t.TempDir(), opener(buf.Bytes()), Secret{Passphrase: passphrase})
	if err != nil {
		t.Fatal(err)
	}
	if r.OK || r.ChainVerified || !strings.Contains(r.Problem, "a3") {
		t.Fatalf("a chain with a removed row verified in rehearsal: %+v", r)
	}
}

// closeTest closes c and fails the test on error, because a swallowed Close
// would hide a leaked connection.
func closeTest(t testing.TB, c io.Closer) {
	t.Helper()
	if err := c.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
