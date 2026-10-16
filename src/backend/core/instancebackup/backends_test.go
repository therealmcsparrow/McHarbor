// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"bytes"
	"context"
	"os"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// TestRestoreAcrossBackends is the migration restore: an export taken on
// SQLite restores onto each external backend with its audit chain intact, and
// an export taken there comes back to SQLite. It runs only where the
// four-backend job provides the databases.
func TestRestoreAcrossBackends(t *testing.T) {
	for _, b := range []struct {
		name, env string
		driver    coredb.Driver
	}{
		{"postgres", "MCHARBOR_TEST_PG_DSN", coredb.DriverPostgres},
		{"mysql", "MCHARBOR_TEST_MYSQL_DSN", coredb.DriverMySQL},
		{"mariadb", "MCHARBOR_TEST_MARIADB_DSN", coredb.DriverMySQL},
	} {
		t.Run(b.name, func(t *testing.T) {
			dsn := os.Getenv(b.env)
			if dsn == "" {
				t.Skipf("%s is not set", b.env)
			}
			database, err := coredb.Open(coredb.Config{Driver: b.driver, DSN: dsn})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { closeTest(t, database) })
			if err := coredb.Migrate(database, b.driver); err != nil {
				t.Fatal(err)
			}
			_, raw := sourceWithData(t)
			dir := t.TempDir()
			target := Target{DB: database, Driver: b.driver, DataDir: dir, EncryptionKey: bytes.Repeat([]byte{9}, 32), Secret: "target"}
			res, err := Restore(context.Background(), target, opener(raw), RestoreOptions{
				Secret: Secret{Passphrase: passphrase}, Mode: ModeMigration, IncludeAudit: true, Actor: "admin", Via: "web", Emit: emit,
			})
			if err != nil {
				t.Fatalf("restore onto %s: %v", b.name, err)
			}
			if res.AnchorID == "" {
				t.Fatal("no anchor was written")
			}
			var branding string
			if err := database.QueryRow(rebind(b.driver, "SELECT value FROM settings WHERE "+coredb.QuoteIdent(b.driver, "key")+" = ?"), "branding_name").Scan(&branding); err != nil || branding != "Harbor Ops" {
				t.Fatalf("restored data on %s: %q %v", b.name, branding, err)
			}
			if ok, _, anchors, problem := verifyChain(context.Background(), database, "source-secret"); !ok || anchors != 1 {
				t.Fatalf("the chain on %s: %s (anchors %d)", b.name, problem, anchors)
			}

			var again bytes.Buffer
			if _, err := Export(context.Background(), Source{
				DB: database, Driver: b.driver, DataDir: dir, EncryptionKey: bytes.Repeat([]byte{1}, 32), Secret: "source-secret",
			}, &again, ExportOptions{Secret: Secret{Passphrase: passphrase}}); err != nil {
				t.Fatalf("export from %s: %v", b.name, err)
			}
			back := newInstance(t, 3, "other", "OTHER CA")
			if _, err := Restore(context.Background(), back.target(), opener(again.Bytes()), RestoreOptions{
				Secret: Secret{Passphrase: passphrase}, Mode: ModeMigration, IncludeAudit: true, Actor: "admin", Via: "web", Emit: emit,
			}); err != nil {
				t.Fatalf("restore from %s back to sqlite: %v", b.name, err)
			}
			if back.value(t, "branding_name") != "Harbor Ops" {
				t.Fatal("the round trip lost data")
			}
			if anchors := chainAnchors(t, back.db, "source-secret"); len(anchors) != 2 {
				t.Fatalf("after two restores the chain carries %d anchors, want 2", len(anchors))
			}
		})
	}
}
