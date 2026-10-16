// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

const (
	seedEncryptionKey = "c2VlZGVkLWVuY3J5cHRpb24ta2V5LTMyYnl0ZXMhISE="
	seedServerSecret  = "seeded-mcharbor-secret-9f8e7d6c"
	seedDSNPassword   = "SeededDsnPassw0rd"
	seedDSN           = "postgres://mch:" + seedDSNPassword + "@db.internal:5432/mch?sslmode=require"
	seedAgentToken    = "seeded-agent-token-abcdef123456"
	seedKubeconfig    = "seeded-kubeconfig-contents-xyz987"
	seedBearer        = "seeded-k8s-bearer-token-qwerty55"
	seedSSHKey        = "seeded-ssh-private-key-material-77"
	seedAPIKey        = "mch_seededapikey1234567890"
	seedOtherToken    = "seeded-random-service-token-4242"
	seedURLPassword   = "SeededUrlPass99"
	seedPluginPass    = "SeededPluginPass31"
	seedBackupKey     = "seeded-backup-master-key-material-01"
)

// seededSecrets are every value that must never appear in a bundle, in the forms a leak
// would take.
func seededSecrets() []string {
	out := []string{
		seedEncryptionKey, seedServerSecret, seedDSNPassword, seedDSN, seedAgentToken, seedKubeconfig,
		seedBearer, seedSSHKey, seedAPIKey, seedOtherToken, seedURLPassword, seedPluginPass, seedBackupKey,
	}
	for _, v := range []string{seedServerSecret, seedAgentToken, seedAPIKey} {
		out = append(out, base64.StdEncoding.EncodeToString([]byte(v)))
	}
	return out
}

func seededEnv(t *testing.T) {
	t.Helper()
	env := map[string]string{
		"ENCRYPTION_KEY":         seedEncryptionKey,
		"MCHARBOR_SECRET":        seedServerSecret,
		"MCHARBOR_DB_DSN":        seedDSN,
		"MCHARBOR_DB_DRIVER":     "postgres",
		"SOME_SERVICE_TOKEN":     seedOtherToken,
		"MCHARBOR_ADVERTISE_URL": "https://admin:" + seedURLPassword + "@node1.example:8705",
		"MCHARBOR_CUSTOM_FLAG":   "custom-value-not-allow-listed",
		"LOG_LEVEL":              "debug",
		"PORT":                   "8705",
	}
	environ := make([]string, 0, len(env))
	for k, v := range env {
		environ = append(environ, k+"="+v)
	}
	prev := processEnv
	processEnv = func() ([]string, func(string) (string, bool)) {
		return environ, func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	}
	t.Cleanup(func() { processEnv = prev })
}

func migratedDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "support.db"))
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
	database.SetMaxOpenConns(1)
	return database
}

func seededService(t *testing.T) *Service {
	t.Helper()
	seededEnv(t)
	database := migratedDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, database, `INSERT INTO environments (id, name, connection_type, agent_token, tls_key, ssh_key, socket_path)
		VALUES ('e1', 'prod-env', 'agent', ?, ?, ?, '/var/run/docker.sock')`, seedAgentToken, seedSSHKey, seedSSHKey)
	mustExec(t, database, `INSERT INTO audit_logs (id, action, created_at, updated_at, timestamp) VALUES ('a1', 'permission.denied', ?, ?, ?)`, now, now, now)
	dataDir := t.TempDir()
	logs := NewLogRing(50)
	logger := slog.New(logs.Handler(slog.NewTextHandler(io.Discard, nil)))
	logger.Info("connecting", "dsn", seedDSN)
	logger.Warn("upstream call", "api_key", seedAPIKey, "header", "Authorization: Bearer "+seedBearer)
	logger.Info("loaded key " + seedServerSecret)
	logger.Error("cluster import failed", "kubeconfig", seedKubeconfig)
	logger.Info("backup key in use " + seedBackupKey)
	svc := NewService(context.Background(), Deps{
		DB: database, DataDir: dataDir, DatabasePath: filepath.Join(dataDir, "mcharbor.db"), DatabaseDriver: "sqlite",
		EncryptionKeySet: true, Logs: logs, Logger: logger,
		KnownSecrets: []string{seedEncryptionKey, seedServerSecret, seedDSN, seedBackupKey},
		Plugins: func() ([]PluginState, bool) {
			return []PluginState{{Slug: "addon-demo", Version: "2026.09.01", Error: "open failed: password=" + seedPluginPass}}, true
		},
	})
	fixed := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }
	t.Cleanup(svc.Close)
	return svc
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func readZip(t *testing.T, path string) map[string]string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeutil.Report(zr.Close(), "zr") }()
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(rc)
		closeutil.Report(rc.Close(), "rc")
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(raw)
	}
	return out
}

func generate(t *testing.T, svc *Service, cats []string) (Bundle, map[string]string) {
	t.Helper()
	b, err := svc.StartBundle(cats, "tester")
	if err != nil {
		t.Fatal(err)
	}
	svc.wg.Wait()
	path, final, err := svc.BundlePath(b.ID)
	if err != nil {
		t.Fatalf("bundle %s not ready: %v", b.ID, err)
	}
	return final, readZip(t, path)
}

// TestBundleNeverContainsSeededSecrets is the promise the support bundle makes: whatever an
// admin hands to support carries no key, token, password, kubeconfig or DSN credential.
func TestBundleNeverContainsSeededSecrets(t *testing.T) {
	svc := seededService(t)
	_, files := generate(t, svc, BundleCategories)
	if len(files) < len(BundleCategories) {
		t.Fatalf("bundle has %d files, want at least %d; an empty bundle proves nothing", len(files), len(BundleCategories))
	}
	all := strings.Join(mapValues(files), "\n")
	for _, want := range []string{"prod-env", "permission.denied", "addon-demo", Redacted, "LOG_LEVEL"} {
		if !strings.Contains(all, want) {
			t.Errorf("bundle is missing %q; the categories were not collected", want)
		}
	}
	var leaks []string
	for name, content := range files {
		for _, secret := range seededSecrets() {
			if strings.Contains(content, secret) || strings.Contains(content, url.QueryEscape(secret)) {
				leaks = append(leaks, name+": "+secret)
			}
		}
	}
	if len(leaks) > 0 {
		t.Fatalf("bundle leaked %d seeded secrets:\n  %s", len(leaks), strings.Join(leaks, "\n  "))
	}
	if strings.Contains(all, "/var/run/docker.sock") {
		t.Error("bundle exposes a Docker socket path")
	}
}

// TestPreviewIsExactlyTheBundle guards the redaction preview's claim: an admin who reviewed
// the preview approved exactly the bytes that end up in the zip.
func TestPreviewIsExactlyTheBundle(t *testing.T) {
	svc := seededService(t)
	cats := []string{CatSystem, CatEnvironments, CatAudit}
	preview, err := svc.Preview(context.Background(), cats)
	if err != nil {
		t.Fatal(err)
	}
	_, files := generate(t, svc, cats)
	if len(preview.Files) != len(files) {
		t.Fatalf("preview has %d files, bundle has %d", len(preview.Files), len(files))
	}
	for _, pf := range preview.Files {
		if files[pf.Name] != pf.Content {
			t.Errorf("%s differs between preview and bundle", pf.Name)
		}
	}
}

// TestBundleHistoryKeepsTheNewestAndRejectsTraversal keeps DATA_DIR from filling up and keeps
// a crafted id from reaching a file outside the bundle directory.
func TestBundleHistoryKeepsTheNewestAndRejectsTraversal(t *testing.T) {
	svc := seededService(t)
	svc.keep = 2
	var ids []string
	for i := 0; i < 3; i++ {
		b, _ := generate(t, svc, []string{CatAudit})
		ids = append(ids, b.ID)
		svc.now = func() time.Time { return time.Date(2026, 9, 26, 12, i+1, 0, 0, time.UTC) }
	}
	list, err := svc.ListBundles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("history holds %d bundles, want 2", len(list))
	}
	if _, err := svc.BundleByID(ids[0]); err == nil {
		t.Error("the oldest bundle survived pruning")
	}
	for _, name := range []string{"evil.json", "evil.zip"} {
		body := `{"id":"../evil","status":"ready"}`
		if err := os.WriteFile(filepath.Join(svc.d.DataDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, bad := range []string{"../evil", "..%2f..%2fx", "", strings.Repeat("a", 21)} {
		if _, _, err := svc.BundlePath(bad); err == nil {
			t.Errorf("BundlePath(%q) was accepted", bad)
		}
	}
	if _, err := svc.DeleteBundle(ids[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(svc.bundleDir(), ids[2]+".zip")); !os.IsNotExist(err) {
		t.Error("delete left the zip behind")
	}
}

// TestBundleCategoriesAreValidated stops an unknown category from being silently ignored,
// which would hand support a bundle missing what they asked for.
func TestBundleCategoriesAreValidated(t *testing.T) {
	svc := seededService(t)
	if _, err := svc.StartBundle(nil, "x"); !errors.Is(err, ErrNoCategories) {
		t.Errorf("empty selection: got %v", err)
	}
	if _, err := svc.StartBundle([]string{"kubeconfigs"}, "x"); !errors.Is(err, ErrBadCategory) {
		t.Errorf("unknown category: got %v", err)
	}
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
