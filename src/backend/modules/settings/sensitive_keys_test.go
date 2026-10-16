// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"bytes"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/config"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func newSensitiveKeyFixture(t *testing.T) (*sql.DB, *Handler, *chi.Mux) {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(&router.AppDeps{
		DB:       database,
		Config:   &config.Config{DataDir: t.TempDir()},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		AuditLog: audit.NewLogger(database, "test-secret"),
	})
	r := chi.NewRouter()
	r.Put("/settings/{key}", h.HandleSetByKey)
	r.Put("/settings", h.HandleBulkUpdate)
	return database, h, r
}

func send(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestGenericWritesRefuseSensitiveKeys keeps the TLS key pair and the push signing key
// from being written raw through the key/value routes, which would bypass the
// encrypting writers that own them.
func TestGenericWritesRefuseSensitiveKeys(t *testing.T) {
	database, _, r := newSensitiveKeyFixture(t)
	for _, key := range []string{"tls_key", "tls_cert", "webpush_vapid_private"} {
		single := send(r, http.MethodPut, "/settings/"+key, `{"value":"-----BEGIN PRIVATE KEY-----raw","category":"tls"}`)
		if single.Code != http.StatusBadRequest || !strings.Contains(single.Body.String(), "err.protected_target") {
			t.Errorf("PUT /settings/%s = %d %s, want 400 err.protected_target", key, single.Code, single.Body.String())
		}
		bulk := send(r, http.MethodPut, "/settings", `{"settings":[{"key":"theme","value":"dark"},{"key":"`+key+`","value":"raw"}]}`)
		if bulk.Code != http.StatusBadRequest || !strings.Contains(bulk.Body.String(), "err.protected_target") {
			t.Errorf("bulk update with %s = %d %s, want 400 err.protected_target", key, bulk.Code, bulk.Body.String())
		}
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM settings WHERE value LIKE '%raw%' OR (key = 'theme' AND value = 'dark')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("a refused request still wrote %d setting rows", n)
	}

	ok := send(r, http.MethodPut, "/settings/theme", `{"value":"dark","category":"ui"}`)
	if ok.Code != http.StatusOK {
		t.Fatalf("an ordinary key was refused: %d %s", ok.Code, ok.Body.String())
	}
}

// TestSettingsAuditDiffRedactsSensitiveValues keeps key material out of the audit trail
// even when a sensitive setting reaches the diff, on both the single and bulk shapes.
func TestSettingsAuditDiffRedactsSensitiveValues(t *testing.T) {
	database, h, _ := newSensitiveKeyFixture(t)
	req := httptest.NewRequest(http.MethodPut, "/settings/tls_key", nil)

	h.logAuditWithDiff(req, audit.Entry{Action: "update", EntityType: "settings", EntityName: "tls_key"},
		Setting{Key: "tls_key", Value: "OLD-PRIVATE-KEY"}, Setting{Key: "tls_key", Value: "NEW-PRIVATE-KEY"})
	h.logAuditWithDiff(req, audit.Entry{Action: "update", EntityType: "settings"},
		map[string]any{"webpush_vapid_private": Setting{Key: "webpush_vapid_private", Value: "OLD-VAPID"}, "theme": Setting{Key: "theme", Value: "light"}},
		map[string]any{"webpush_vapid_private": Setting{Key: "webpush_vapid_private", Value: "NEW-VAPID"}, "theme": Setting{Key: "theme", Value: "dark"}})

	rows, err := database.Query(`SELECT COALESCE(state_before, ''), COALESCE(state_after, ''), COALESCE(changed_keys, '') FROM audit_logs`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var all strings.Builder
	count := 0
	for rows.Next() {
		var before, after, changed string
		if err := rows.Scan(&before, &after, &changed); err != nil {
			t.Fatal(err)
		}
		all.WriteString(before + after + changed)
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("audit rows = %d, want 2", count)
	}
	for _, secret := range []string{"OLD-PRIVATE-KEY", "NEW-PRIVATE-KEY", "OLD-VAPID", "NEW-VAPID"} {
		if strings.Contains(all.String(), secret) {
			t.Errorf("audit diff carries %q", secret)
		}
	}
	if !strings.Contains(all.String(), "dark") {
		t.Errorf("an ordinary setting lost its value in the diff: %s", all.String())
	}
}
