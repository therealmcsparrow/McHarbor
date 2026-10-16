// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliance

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/config"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func openTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	database, err := coredb.OpenSQLite(filepath.Join(dir, "mcharbor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Log(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database, dir
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func writeCert(t *testing.T, dir string, notAfter time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mcharbor.test"},
		NotBefore:    notAfter.Add(-365 * 24 * time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tls"), 0o700); err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(filepath.Join(dir, "tls", "cert.pem"), pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
}

func factValue(t *testing.T, r Report, section, key string) string {
	t.Helper()
	for _, s := range r.Sections {
		if s.ID != section {
			continue
		}
		for _, f := range s.Facts {
			if f.Key == key {
				return f.Value
			}
		}
	}
	t.Fatalf("fact %s.%s not in report", section, key)
	return ""
}

func sectionStatus(r Report, id string) Status {
	for _, s := range r.Sections {
		if s.ID == id {
			return s.Status
		}
	}
	return ""
}

func TestServiceMeasuresTheDatabase(t *testing.T) {
	db, dir := openTestDB(t)
	now := time.Now().UTC()
	users := []struct {
		id, provider string
		active, mfa  int
	}{
		{"u1", "local", 1, 1},
		{"u2", "local", 1, 1},
		{"u3", "local", 1, 0},
		{"u4", "local", 0, 1},
		{"u5", "oidc", 1, 0},
	}
	for _, u := range users {
		mustExec(t, db, `INSERT INTO users (id, username, password_hash, auth_provider, is_active) VALUES (?, ?, 'x', ?, ?)`,
			u.id, "user-"+u.id, u.provider, u.active)
		mustExec(t, db, `INSERT INTO user_mfa (user_id, enabled) VALUES (?, ?)`, u.id, u.mfa)
	}
	mustExec(t, db, `INSERT INTO appstore_plugins (id, slug, name, version, signed) VALUES ('p1','a','A','1.0.0',1), ('p2','b','B','1.0.0',0)`)
	mustExec(t, db, `INSERT INTO instance_backup_exports (id, status, completed_at, created_at) VALUES ('e1','completed',?,?)`,
		now.Add(-2*time.Hour).Format(time.RFC3339), now.Add(-2*time.Hour).Format(time.RFC3339))
	mustExec(t, db, `INSERT INTO instance_backup_exports (id, status, created_at) VALUES ('e2','failed',?)`,
		now.Add(-time.Hour).Format(time.RFC3339))
	writeCert(t, dir, now.Add(10*24*time.Hour))

	cfg := &config.Config{DataDir: dir, TLSEnabled: true, TLSMinVersion: config.TLSMinVersion(tls.VersionTLS13), EncryptionKey: "set"}
	svc := NewService(db, Sources{
		Config:       cfg,
		BackupKeySet: func() bool { return true },
		BackupKeyID:  func() string { return "kid-123" },
		HA: func(context.Context) HAState {
			return HAState{Known: true, Mode: "peer", Driver: "sqlite", NodeID: "node-a", PluginStore: "local"}
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	r := svc.Report(context.Background(), AllSections, false)

	checks := map[[2]string]string{
		{SectionMFA, "localUsers"}:               "3",
		{SectionMFA, "localUsersWithMfa"}:        "2",
		{SectionMFA, "mfaAdoption"}:              "66",
		{SectionMFA, "externalUsers"}:            "1",
		{SectionPlugins, "installed"}:            "2",
		{SectionPlugins, "unsigned"}:             "1",
		{SectionInstanceBackup, "lastRunStatus"}: "failed",
		{SectionTLS, "source"}:                   "self-signed",
		{SectionTLS, "minVersion"}:               "1.3",
		{SectionKeys, "encryptionKeySource"}:     "environment",
		{SectionKeys, "backupKeyId"}:             "kid-123",
		{SectionHA, "nodeId"}:                    "node-a",
	}
	for k, want := range checks {
		if got := factValue(t, r, k[0], k[1]); got != want {
			t.Errorf("%s.%s = %q, want %q", k[0], k[1], got, want)
		}
	}
	if got := sectionStatus(r, SectionInstanceBackup); got != StatusFail {
		t.Errorf("instanceBackup status %s, want fail after a failed run", got)
	}
	if got := sectionStatus(r, SectionTLS); got != StatusWarn {
		t.Errorf("tls status %s, want warn for a cert expiring in 10 days", got)
	}
	if r.Summary.Pass+r.Summary.Warn+r.Summary.Fail+r.Summary.Info+r.Summary.Unknown != len(AllSections) {
		t.Errorf("summary %+v does not cover %d sections", r.Summary, len(AllSections))
	}

	red := svc.Report(context.Background(), []string{SectionKeys, SectionHA, SectionTLS}, true)
	if !red.Redacted || len(red.Sections) != 3 {
		t.Fatalf("redacted report: redacted=%v sections=%d", red.Redacted, len(red.Sections))
	}
	for _, k := range [][2]string{{SectionKeys, "backupKeyId"}, {SectionHA, "nodeId"}, {SectionTLS, "subject"}} {
		if got := factValue(t, red, k[0], k[1]); got != RedactedValue {
			t.Errorf("redacted %s.%s = %q", k[0], k[1], got)
		}
	}
}

func TestMissingTableMakesSectionUnknown(t *testing.T) {
	db, dir := openTestDB(t)
	mustExec(t, db, `DROP TABLE appstore_plugins`)
	svc := NewService(db, Sources{Config: &config.Config{DataDir: dir}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := svc.Report(context.Background(), []string{SectionPlugins}, false)
	if r.Sections[0].Status != StatusUnknown {
		t.Fatalf("plugins status %s, want unknown when its table is missing", r.Sections[0].Status)
	}
}

func TestSavedTLSSettingOverridesEnvironment(t *testing.T) {
	db, dir := openTestDB(t)
	mustExec(t, db, `INSERT INTO settings (id, key, value, category) VALUES ('s-tls', 'tls_enabled', 'false', 'tls')
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`)
	svc := NewService(db, Sources{Config: &config.Config{DataDir: dir, TLSEnabled: true}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := svc.Report(context.Background(), []string{SectionTLS}, false)
	if got := factValue(t, r, SectionTLS, "enabled"); got != "false" {
		t.Fatalf("tls.enabled = %q, want false when the saved setting turns TLS off", got)
	}
	if r.Sections[0].Status != StatusWarn {
		t.Fatalf("tls status %s, want warn", r.Sections[0].Status)
	}
}
