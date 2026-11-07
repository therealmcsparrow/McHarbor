// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func channelsHandler(t *testing.T) (*Handler, *sql.DB) {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "ch.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	app := &router.AppDeps{DB: database, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AuditLog: audit.NewLogger(database, "k")}
	return &Handler{app: app}, database
}

func channelsReq(method, body string, signedIn bool) *http.Request {
	r := httptest.NewRequest(method, "/api/updates/channels", strings.NewReader(body))
	if signedIn {
		r = r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "op"}))
	}
	return r
}

func decodeChannels(t *testing.T, w *httptest.ResponseRecorder) map[string]bool {
	t.Helper()
	var env struct {
		Data map[string]bool `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return env.Data
}

func TestChannelsDefaultOffAndPartialPut(t *testing.T) {
	h, database := channelsHandler(t)
	w := httptest.NewRecorder()
	h.HandleGetChannels(w, channelsReq(http.MethodGet, "", true))
	got := decodeChannels(t, w)
	if w.Code != 200 || len(got) != 5 {
		t.Fatalf("GET: %d %v", w.Code, got)
	}
	for k, v := range got {
		if v {
			t.Errorf("%s defaults on", k)
		}
	}

	w = httptest.NewRecorder()
	h.HandlePutChannels(w, channelsReq(http.MethodPut, `{"agentPrereleases":true,"mcharborAutoPrereleases":true}`, true))
	if w.Code != 200 {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	// A stale client sends one switch; the others keep their newer values.
	w = httptest.NewRecorder()
	h.HandlePutChannels(w, channelsReq(http.MethodPut, `{"mcharborPrereleases":true}`, true))
	got = decodeChannels(t, w)
	if !got["agentPrereleases"] || !got["mcharborAutoPrereleases"] || !got["mcharborPrereleases"] || got["pluginPrereleases"] {
		t.Fatalf("stale save lost a switch: %v", got)
	}

	var before, after, keys string
	if err := database.QueryRow(`SELECT state_before, state_after, changed_keys FROM audit_logs WHERE action = 'update_channels_change' ORDER BY id DESC LIMIT 1`).Scan(&before, &after, &keys); err != nil {
		t.Fatal(err)
	}
	if keys != "mcharborPrereleases" || strings.Contains(after, "agentPrereleases") || !strings.Contains(after, "true") || !strings.Contains(before, "false") {
		t.Fatalf("audit must list only the change: keys=%q before=%q after=%q", keys, before, after)
	}
}

func TestChannelsNoChangeWritesNoAudit(t *testing.T) {
	h, database := channelsHandler(t)
	w := httptest.NewRecorder()
	h.HandlePutChannels(w, channelsReq(http.MethodPut, `{"agentPrereleases":false}`, true))
	if w.Code != 200 {
		t.Fatalf("PUT: %d", w.Code)
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'update_channels_change'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("audit rows %d err %v", n, err)
	}
}

func TestChannelsHostileBodies(t *testing.T) {
	h, _ := channelsHandler(t)
	for name, body := range map[string]string{
		"unknown field": `{"autoInstall":true}`,
		"wrong type":    `{"agentPrereleases":"yes"}`,
		"not json":      `nope`,
		"oversized":     `{"agentPrereleases":true,"x":"` + strings.Repeat("a", 8192) + `"}`,
	} {
		w := httptest.NewRecorder()
		h.HandlePutChannels(w, channelsReq(http.MethodPut, body, true))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.HandlePutChannels(w, channelsReq(http.MethodPut, `{}`, false))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: status %d", w.Code)
	}
}

func TestChannelsFailuresNameTheirOperation(t *testing.T) {
	h, database := channelsHandler(t)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	codeOf := func(w *httptest.ResponseRecorder) string {
		t.Helper()
		var env struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		return env.Code
	}
	get := httptest.NewRecorder()
	h.HandleGetChannels(get, channelsReq(http.MethodGet, "", true))
	if got := codeOf(get); got != string(i18n.ErrSettingsFailed) {
		t.Fatalf("a failed read answered %q", got)
	}
	put := httptest.NewRecorder()
	h.HandlePutChannels(put, channelsReq(http.MethodPut, `{"agentPrereleases":true}`, true))
	if got := codeOf(put); got != string(i18n.ErrSettingsUpdateFailed) {
		t.Fatalf("a failed save answered %q", got)
	}
}
