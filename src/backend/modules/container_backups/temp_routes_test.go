// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

type tempEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Code    string          `json:"code"`
	Detail  string          `json:"detail"`
}

type tempHarness struct {
	t        *testing.T
	db       *sql.DB
	mux      http.Handler
	dataDir  string
	envValue *string
}

// newTempHarness serves the real temp routes behind the real RBAC middleware.
// It assumes mountTempRoutes(r, app, h) registers /temp, /temp/cleanup on r,
// and that Service carries an injectable getenv field.
func newTempHarness(t *testing.T) *tempHarness {
	t.Helper()
	database := tempTestDB(t)
	for _, stmt := range []string{
		`INSERT INTO environments (id, name, connection_type) VALUES ('env-a', 'env-a', 'socket')`,
		`INSERT INTO users (id, username, password_hash) VALUES ('globalmgr', 'globalmgr', 'x')`,
		`INSERT INTO users (id, username, password_hash) VALUES ('scopedmgr', 'scopedmgr', 'x')`,
		`INSERT INTO users (id, username, password_hash) VALUES ('nobody', 'nobody', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('bk-role', 'bk-role', '["backups.manage","backups.view"]')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur-global', 'globalmgr', 'bk-role')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur-scoped', 'scopedmgr', 'bk-role', 'env-a')`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	h := &tempHarness{t: t, db: database, dataDir: filepath.Join(t.TempDir(), "data"), envValue: new(string)}
	app := &router.AppDeps{DB: database, RBACService: rbac.NewService(database), AuditLog: audit.NewLogger(database, "test-secret"), Logger: slog.Default()}
	svc := NewService(database, nil, h.dataDir, nil, nil, slog.Default())
	svc.getenv = func(key string) string {
		if key == "MCHARBOR_BACKUP_TEMP_DIR" {
			return *h.envValue
		}
		return ""
	}
	handler := &Handler{app: app, service: svc}
	mux := chi.NewRouter()
	mux.Route("/api", func(sub chi.Router) {
		root := apispec.NewRouter(sub, apispec.NewRegistry(), "/api", apispec.GroupProtected)
		root.Route("/container-backups", func(r *apispec.Router) { mountTempRoutes(r, app, handler) })
	})
	h.mux = mux
	return h
}

func (h *tempHarness) do(user, method, path, body string) (*httptest.ResponseRecorder, tempEnvelope) {
	h.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(response.WithDetail(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: user, Username: user})))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	var env tempEnvelope
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			h.t.Fatalf("%s %s: body is not an envelope: %v: %s", method, path, err, rec.Body.String())
		}
	}
	return rec, env
}

func (h *tempHarness) savedSetting() (string, bool) {
	var v sql.NullString
	err := h.db.QueryRow(`SELECT value FROM settings WHERE key = 'backup_temp_dir'`).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		h.t.Fatal(err)
	}
	return v.String, v.Valid && v.String != ""
}

func putBody(path string) string {
	b, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		panic(err) // panic-ok: a map of strings always marshals; test helper has no *testing.T
	}
	return string(b)
}

func TestTempRoutesRequireGlobalManage(t *testing.T) {
	h := newTempHarness(t)
	root := filepath.Join(t.TempDir(), "spool")
	leftover := filepath.Join(root, "mcharbor-run-old")
	if err := os.MkdirAll(leftover, 0o700); err != nil {
		t.Fatal(err)
	}
	*h.envValue = root
	old := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(leftover, old, old); err != nil {
		t.Fatal(err)
	}

	calls := []struct{ method, path, body string }{
		{http.MethodGet, "/api/container-backups/temp", ""},
		{http.MethodPut, "/api/container-backups/temp", putBody(filepath.Join(t.TempDir(), "x"))},
		{http.MethodPost, "/api/container-backups/temp/cleanup", ""},
		{http.MethodGet, "/api/container-backups/temp?env=env-a", ""},
		{http.MethodPut, "/api/container-backups/temp?env=env-a", putBody(filepath.Join(t.TempDir(), "x"))},
		{http.MethodPost, "/api/container-backups/temp/cleanup?env=env-a", ""},
	}
	for _, user := range []string{"nobody", "scopedmgr"} {
		for _, c := range calls {
			rec, env := h.do(user, c.method, c.path, c.body)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s %s: status %d, want 403", user, c.method, c.path, rec.Code)
			}
			if env.Detail != "" {
				t.Errorf("%s %s: a 403 must never carry a cause, got %q", c.method, c.path, env.Detail)
			}
		}
	}
	if _, saved := h.savedSetting(); saved {
		t.Fatal("a refused PUT saved a location")
	}
	if _, err := os.Stat(leftover); err != nil {
		t.Fatalf("a refused cleanup removed an entry: %v", err)
	}

	rec, _ := h.do("globalmgr", http.MethodGet, "/api/container-backups/temp", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("a global backups.manage grant must reach GET /temp, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTempGetReportsTheResolvedRoot(t *testing.T) {
	h := newTempHarness(t)
	rec, env := h.do("globalmgr", http.MethodGet, "/api/container-backups/temp", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var st struct {
		Path        string `json:"path"`
		Source      string `json:"source"`
		SettingPath string `json:"settingPath"`
		EnvOverride bool   `json:"envOverride"`
		DefaultPath string `json:"defaultPath"`
	}
	if err := json.Unmarshal(env.Data, &st); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(h.dataDir, "backups", "tmp")
	if st.Path != want || st.DefaultPath != want || st.Source != "default" || st.SettingPath != "" || st.EnvOverride {
		t.Fatalf("status = %+v, want the default root %q", st, want)
	}
}

func TestTempPutSavesTheLocationAndAudits(t *testing.T) {
	h := newTempHarness(t)
	target := filepath.Join(t.TempDir(), "spool", "tmp")
	rec, _ := h.do("globalmgr", http.MethodPut, "/api/container-backups/temp", putBody(target))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got, ok := h.savedSetting(); !ok || got != target {
		t.Fatalf("saved setting = (%q, %v), want %q", got, ok, target)
	}
	var entityType, before, after string
	err := h.db.QueryRow(`SELECT COALESCE(entity_type, ''), COALESCE(state_before, ''), COALESCE(state_after, '') FROM audit_logs WHERE action = 'backup_temp.location_changed'`).Scan(&entityType, &before, &after)
	if err != nil {
		t.Fatalf("no audit row for the change: %v", err)
	}
	if entityType != "backup_temp" || before != filepath.Join(h.dataDir, "backups", "tmp") || after != target {
		t.Fatalf("audit = (%q, before %q, after %q), want old and new resolved roots", entityType, before, after)
	}
}

func TestTempPutEmptyPathResetsToDefault(t *testing.T) {
	h := newTempHarness(t)
	target := filepath.Join(t.TempDir(), "spool")
	if rec, _ := h.do("globalmgr", http.MethodPut, "/api/container-backups/temp", putBody(target)); rec.Code != http.StatusOK {
		t.Fatalf("seed PUT: %d %s", rec.Code, rec.Body.String())
	}
	rec, env := h.do("globalmgr", http.MethodPut, "/api/container-backups/temp", putBody(""))
	if rec.Code != http.StatusOK {
		t.Fatalf("reset PUT: %d %s", rec.Code, rec.Body.String())
	}
	var st struct {
		Source      string `json:"source"`
		SettingPath string `json:"settingPath"`
	}
	if err := json.Unmarshal(env.Data, &st); err != nil {
		t.Fatal(err)
	}
	if st.Source != "default" || st.SettingPath != "" {
		t.Fatalf("after reset: %+v", st)
	}
	if _, saved := h.savedSetting(); saved {
		t.Fatal("reset left a saved location")
	}
}

func TestTempPutRefusals(t *testing.T) {
	h := newTempHarness(t)
	base := t.TempDir()
	sep := string(os.PathSeparator)
	blocker := filepath.Join(base, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		body string
		code i18n.MsgCode
	}{
		{"invalid body", `{"path":`, i18n.ErrInvalidBody},
		{"relative", putBody("relative" + sep + "dir"), i18n.ErrAbsolutePathReq},
		{"traversal", putBody(base + sep + "a" + sep + ".." + sep + "b"), i18n.ErrPathTraversal},
		{"ancestor of the data dir", putBody(filepath.Dir(h.dataDir)), i18n.ErrBackupTempPathSensitive},
		{"data dir", putBody(h.dataDir), i18n.ErrBackupTempPathSensitive},
		{"not writable", putBody(filepath.Join(blocker, "sub")), i18n.ErrBackupTempNotWritable},
	}
	if os.PathSeparator == '/' {
		cases = append(cases, struct {
			name string
			body string
			code i18n.MsgCode
		}{"child of /etc", putBody("/etc/mcharbor-tmp"), i18n.ErrBackupTempPathSensitive})
		open := filepath.Join(base, "open")
		if err := os.Mkdir(open, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(open, 0o777); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, struct {
			name string
			body string
			code i18n.MsgCode
		}{"world-writable", putBody(open), i18n.ErrBackupTempWorldWritable})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, env := h.do("globalmgr", http.MethodPut, "/api/container-backups/temp", c.body)
			if rec.Code != http.StatusBadRequest || env.Code != string(c.code) {
				t.Fatalf("status %d code %q, want 400 %q: %s", rec.Code, env.Code, c.code, rec.Body.String())
			}
			if c.code == i18n.ErrBackupTempNotWritable && strings.TrimSpace(env.Detail) == "" {
				t.Error("a probe failure must carry the redacted OS error as detail")
			}
			if _, saved := h.savedSetting(); saved {
				t.Fatal("a refused PUT saved a location")
			}
		})
	}
}

func TestTempPutIsConflictWhileTheEnvVarIsSet(t *testing.T) {
	h := newTempHarness(t)
	*h.envValue = filepath.Join(t.TempDir(), "fromenv")
	rec, env := h.do("globalmgr", http.MethodPut, "/api/container-backups/temp", putBody(filepath.Join(t.TempDir(), "other")))
	if rec.Code != http.StatusConflict || env.Code != string(i18n.ErrBackupTempEnvOverride) {
		t.Fatalf("status %d code %q, want 409 %q", rec.Code, env.Code, i18n.ErrBackupTempEnvOverride)
	}
	if _, saved := h.savedSetting(); saved {
		t.Fatal("a conflicting PUT saved a location")
	}
}

func TestTempCleanupRemovesOnlyMcHarborNamedStaleEntriesAndAudits(t *testing.T) {
	h := newTempHarness(t)
	root := filepath.Join(t.TempDir(), "spool")
	stale := filepath.Join(root, "mcharbor-run-stale")
	foreign := filepath.Join(root, "someone-elses-data")
	for _, dir := range []string{stale, foreign} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-3 * time.Hour)
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatal(err)
		}
	}
	*h.envValue = root

	rec, env := h.do("globalmgr", http.MethodPost, "/api/container-backups/temp/cleanup", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var res struct {
		RemovedEntries int `json:"removedEntries"`
	}
	if err := json.Unmarshal(env.Data, &res); err != nil {
		t.Fatal(err)
	}
	if res.RemovedEntries != 1 {
		t.Fatalf("removedEntries = %d, want 1", res.RemovedEntries)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("the stale McHarbor-named folder survived: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("cleanup deleted a folder McHarbor did not name: %v", err)
	}
	var severity string
	if err := h.db.QueryRow(`SELECT COALESCE(severity, '') FROM audit_logs WHERE action = 'backup_temp.cleaned'`).Scan(&severity); err != nil {
		t.Fatalf("no audit row for the cleanup: %v", err)
	}
	if severity != "warning" {
		t.Fatalf("cleanup audit severity = %q, want warning", severity)
	}
}
