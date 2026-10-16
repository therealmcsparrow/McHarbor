// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

const passphrase = "correct horse battery staple"

func testSource(t *testing.T) Source {
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
	for _, stmt := range []string{
		`PRAGMA foreign_keys = OFF`,
		`INSERT INTO settings (key, value) VALUES ('branding_name', 'Harbor Ops')`,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES ('s1', 'u1', '2099-01-01')`,
		`INSERT INTO host_metrics (id, environment_id) VALUES ('m1', 'e1')`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	for _, f := range []struct{ rel, body string }{
		{"tls/ca.pem", "CA CERT"}, {"tls/ca-key.pem", "CA KEY"}, {"tls/cert.pem", "LEAF"},
		{"custom-nodes/hello/node.js", "module.exports = 1"}, {"stacks/web/compose.yml", "services: {}"},
		{"backups/containers/big.tar", "not exported"},
	} {
		p := filepath.Join(dir, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f.body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Source{
		DB: database, Driver: coredb.DriverSQLite, DataDir: dir,
		EncryptionKey: bytes.Repeat([]byte{7}, 32), Secret: "audit-hmac-secret",
		Version: "2026.09.01", Build: "202609261700", InstanceID: "inst-1", CreatedBy: "admin",
	}
}

func export(t *testing.T, src Source, opts ExportOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := Export(context.Background(), src, &buf, opts); err != nil {
		t.Fatalf("export: %v", err)
	}
	return buf.Bytes()
}

func TestExportRoundTripsTablesKeysAndFiles(t *testing.T) {
	src := testSource(t)
	raw := export(t, src, ExportOptions{Secret: Secret{Passphrase: passphrase}, Note: "before upgrade"})

	plain, err := ReadManifest(bytes.NewReader(raw))
	if err != nil || plain.InstanceID != "inst-1" || plain.Note != "before upgrade" || plain.Key.Kind != KeyKindPassphrase {
		t.Fatalf("manifest without the key: %+v, %v", plain, err)
	}
	if bytes.Contains(raw, []byte("Harbor Ops")) || bytes.Contains(raw, []byte("audit-hmac-secret")) || bytes.Contains(raw, []byte("CA KEY")) {
		t.Fatal("secret material is readable in the artifact without the key")
	}

	a, err := Open(bytes.NewReader(raw), Secret{Passphrase: passphrase})
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string]int64{}
	blobs := map[string]string{}
	var branding string
	for {
		e, err := a.Next()
		if err != nil {
			t.Fatal(err)
		}
		if e.Kind == EntryEnd {
			break
		}
		switch e.Kind {
		case EntryTable:
			tables[e.Name] = e.Rows
			if e.Name == "settings" {
				for {
					row, err := a.Row()
					if err != nil {
						break
					}
					for i, c := range e.Columns {
						if c == "value" && row[i] == "Harbor Ops" {
							branding = "Harbor Ops"
						}
					}
				}
			}
		case EntryBlob:
			var b bytes.Buffer
			if _, err := b.ReadFrom(a.Blob()); err != nil {
				t.Fatal(err)
			}
			blobs[e.Name] = b.String()
		}
	}
	if branding != "Harbor Ops" {
		t.Fatal("a settings row did not survive the round trip")
	}
	if _, ok := tables["sessions"]; ok {
		t.Fatal("sessions were exported; a restored session is a restored login")
	}
	if _, ok := tables["host_metrics"]; ok {
		t.Fatal("metrics were exported without being asked for")
	}
	if _, ok := tables["schema_migrations"]; ok {
		t.Fatal("schema_migrations was exported; it belongs to the target")
	}
	if blobs[KeyEncryption] != string(src.EncryptionKey) || blobs[KeySecret] != "audit-hmac-secret" || blobs[TLSCAKey] != "CA KEY" {
		t.Fatalf("key material missing: %v", keys(blobs))
	}
	if _, ok := blobs[TLSCert]; ok {
		t.Fatal("a generated serving certificate was exported; it is re-issued from the CA")
	}
	if blobs["custom-nodes/hello/node.js"] == "" || blobs["stacks/web/compose.yml"] == "" {
		t.Fatalf("data files missing: %v", keys(blobs))
	}
	if _, ok := blobs["backups/containers/big.tar"]; ok {
		t.Fatal("container backups were exported")
	}

	withMetrics := export(t, src, ExportOptions{Secret: Secret{Passphrase: passphrase}, IncludeMetrics: true})
	m, err := Verify(context.Background(), bytes.NewReader(withMetrics), Secret{Passphrase: passphrase})
	if err != nil || !hasTable(m, "host_metrics") {
		t.Fatalf("metrics on request: %v", err)
	}
}

func TestKeyFileExportsAndWrongKeysAreRefused(t *testing.T) {
	src := testSource(t)
	kf, err := GenerateKeyFile()
	if err != nil {
		t.Fatal(err)
	}
	raw := export(t, src, ExportOptions{Secret: Secret{KeyFile: kf}})
	if _, err := Verify(context.Background(), bytes.NewReader(raw), Secret{KeyFile: kf}); err != nil {
		t.Fatalf("key file: %v", err)
	}
	other, err := GenerateKeyFile()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(bytes.NewReader(raw), Secret{KeyFile: other}); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("another key file: %v", err)
	}
	if _, err := Open(bytes.NewReader(raw), Secret{Passphrase: passphrase}); err == nil {
		t.Fatal("a passphrase opened a key-file export")
	}

	pp := export(t, src, ExportOptions{Secret: Secret{Passphrase: passphrase}})
	if _, err := Open(bytes.NewReader(pp), Secret{Passphrase: passphrase + "!"}); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("a wrong passphrase: %v", err)
	}
	var buf bytes.Buffer
	if _, err := Export(context.Background(), src, &buf, ExportOptions{Secret: Secret{Passphrase: "short"}}); err == nil {
		t.Fatal("a short passphrase was accepted")
	}
}

func TestAlteredOrTruncatedExportsAreRefused(t *testing.T) {
	src := testSource(t)
	raw := export(t, src, ExportOptions{Secret: Secret{Passphrase: passphrase}})
	s := Secret{Passphrase: passphrase}

	edited := bytes.Replace(raw, []byte(`"instanceId":"inst-1"`), []byte(`"instanceId":"inst-2"`), 1)
	if _, err := Open(bytes.NewReader(edited), s); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("an edited manifest: %v", err)
	}
	for _, cut := range []int{len(raw) - 1, len(raw) - 40, len(raw) / 2} {
		if _, err := Verify(context.Background(), bytes.NewReader(raw[:cut]), s); err == nil {
			t.Fatalf("an export cut at %d of %d verified", cut, len(raw))
		}
	}
	flipped := append([]byte(nil), raw...)
	flipped[len(flipped)-10] ^= 0xff
	if _, err := Verify(context.Background(), bytes.NewReader(flipped), s); err == nil {
		t.Fatal("a flipped byte in the contents verified")
	}
}

func TestBlobPathsStayInsideWhatAnExportMayCarry(t *testing.T) {
	for _, p := range []string{"../etc/passwd", "/etc/passwd", "stacks/../../x", "plugins/evil.so", "mcharbor.db", `stacks\x`} {
		if checkBlobPath(p) == nil {
			t.Errorf("%q accepted", p)
		}
	}
	for _, p := range []string{KeyEncryption, TLSCAKey, "stacks/web/compose.yml", LedgerFile} {
		if err := checkBlobPath(p); err != nil {
			t.Errorf("%q refused: %v", p, err)
		}
	}
}

func hasTable(m Manifest, name string) bool {
	for _, t := range m.Tables {
		if t.Name == name {
			return true
		}
	}
	return false
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// craft seals hand-written records into an artifact, so a test can produce
// contents a real export never would.
func craft(t *testing.T, write func(sw *streamWriter)) []byte {
	t.Helper()
	key, info, err := newSealKey(Secret{Passphrase: passphrase})
	if err != nil {
		t.Fatal(err)
	}
	m := Manifest{Format: 1, Key: info}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := writeHeader(&buf, raw, manifestMAC(key, raw)); err != nil {
		t.Fatal(err)
	}
	c, err := cipherFor(key)
	if err != nil {
		t.Fatal(err)
	}
	enc, _, err := c.EncryptWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(enc)
	sw := newStreamWriter(gz)
	sum := sha256.Sum256(raw)
	if err := sw.header(record{T: recBegin, Manifest: hex.EncodeToString(sum[:])}); err != nil {
		t.Fatal(err)
	}
	write(sw)
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readAll(raw []byte) error {
	a, err := Open(bytes.NewReader(raw), Secret{Passphrase: passphrase})
	if err != nil {
		return err
	}
	for {
		e, err := a.Next()
		if err != nil {
			return err
		}
		if e.Kind == EntryEnd {
			return nil
		}
	}
}

func TestUnsafeBlobPathsInAnArtifactAreRefused(t *testing.T) {
	raw := craft(t, func(sw *streamWriter) {
		if err := sw.header(record{T: recBlob, Name: "../../etc/cron.d/x", Kind: BlobFile, Size: 2}); err != nil {
			t.Fatal(err)
		}
		if err := sw.write([]byte("hi")); err != nil {
			t.Fatal(err)
		}
		if err := sw.end(); err != nil {
			t.Fatal(err)
		}
	})
	if err := readAll(raw); err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("an artifact writing outside the data directory: %v", err)
	}
}

func TestAStreamWhoseDigestDoesNotMatchIsRefused(t *testing.T) {
	raw := craft(t, func(sw *streamWriter) {
		if err := sw.header(record{T: recTable, Name: "settings", Columns: []string{"key"}, Rows: 1}); err != nil {
			t.Fatal(err)
		}
		if err := sw.row([]any{"a"}); err != nil {
			t.Fatal(err)
		}
		sw.h.Write([]byte("a row that was dropped after hashing"))
		if err := sw.end(); err != nil {
			t.Fatal(err)
		}
	})
	if err := readAll(raw); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("a stream missing hashed content: %v", err)
	}
}
