// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	coreinstancebackup "github.com/therealmcsparrow/mcharbor/core/instancebackup"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func TestRestoreLocalRestoresAStoppedInstanceAndAttributesTheOSUser(t *testing.T) {
	src := t.TempDir()
	srcDB, err := db.OpenSQLite(filepath.Join(src, "mcharbor.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(srcDB, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if _, err := srcDB.Exec(`INSERT INTO settings (key, value) VALUES ('branding_name', 'Harbor Ops')`); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(t.TempDir(), "instance.mchx")
	f, err := os.Create(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coreinstancebackup.Export(context.Background(), coreinstancebackup.Source{
		DB: srcDB, Driver: db.DriverSQLite, DataDir: src, EncryptionKey: bytes.Repeat([]byte{3}, 32), Secret: "src-secret",
	}, f, coreinstancebackup.ExportOptions{Secret: coreinstancebackup.Secret{Passphrase: "correct horse battery"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := srcDB.Close(); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	t.Setenv("DATA_DIR", dst)
	t.Setenv("DATABASE_PATH", filepath.Join(dst, "mcharbor.db"))
	t.Setenv("MCHARBOR_SECRET", "")
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("MCHARBOR_DB_DRIVER", "")
	t.Setenv("TLS_ENABLED", "false")
	t.Setenv("PORT", "1")
	passFile := filepath.Join(t.TempDir(), "pass")
	if err := os.WriteFile(passFile, []byte("correct horse battery\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := runRestoreLocal([]string{"--file", artifact, "--passphrase-file", passFile}, strings.NewReader("nope\n"), &out, &errOut); code != 1 {
		t.Fatalf("an unconfirmed restore: exit %d\n%s", code, errOut.String())
	}
	if code := runRestoreLocal([]string{"--file", artifact, "--passphrase-file", passFile}, strings.NewReader("RESTORE\n"), &out, &errOut); code != 0 {
		t.Fatalf("restore-local: exit %d\n%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), `"actor":"local:`) {
		t.Fatalf("the intent is not attributed to the OS user: %s", errOut.String())
	}

	check, err := db.OpenSQLite(filepath.Join(dst, "mcharbor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeutil.Report(check.Close(), "check") }()
	var branding string
	if err := check.QueryRow(`SELECT value FROM settings WHERE key = 'branding_name'`).Scan(&branding); err != nil || branding != "Harbor Ops" {
		t.Fatalf("restored data: %q %v", branding, err)
	}
	key, err := os.ReadFile(filepath.Join(dst, ".encryption_key"))
	if err != nil || !bytes.Equal(key, bytes.Repeat([]byte{3}, 32)) {
		t.Fatal("the export's key is not in place")
	}
	if _, err := os.Stat(filepath.Join(dst, "restore-status.json")); err != nil {
		t.Fatal("no status was left for the next start to reinstall plugins from")
	}
}

func TestRestoreLocalRefusesWhileMcHarborAnswersUnlessTheInstanceIsNamed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(srv.Close)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	dir := t.TempDir()
	database, err := db.OpenSQLite(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO license_installs (id, install_key, key_masked) VALUES ('inst-42', 'k', 'k')`); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Port: port}
	var errOut bytes.Buffer
	if code := refuseWhileRunning(context.Background(), cfg, database, "", &errOut); code != 1 || !strings.Contains(errOut.String(), "--while-running inst-42") {
		t.Fatalf("a running instance: exit %d, %q", code, errOut.String())
	}
	if code := refuseWhileRunning(context.Background(), cfg, database, "inst-7", &errOut); code != 1 {
		t.Fatal("naming another instance overrode the refusal")
	}
	if code := refuseWhileRunning(context.Background(), cfg, database, "inst-42", &errOut); code != 0 {
		t.Fatal("naming this instance did not override the refusal")
	}
	srv.Close()
	if code := refuseWhileRunning(context.Background(), cfg, database, "", &errOut); code != 0 {
		t.Fatal("a stopped instance was refused")
	}
}
