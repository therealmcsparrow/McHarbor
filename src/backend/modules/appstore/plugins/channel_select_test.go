// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

const (
	stableHost = "2026.11.01"
	preHost    = "2026.11.02-beta.1"
)

func stableRow(v string) PluginVersion {
	return PluginVersion{Version: v, MinMcharbor: ">=2026.09.01", DownloadURL: "file:///" + v}
}

func preRow(v, floor string) PluginVersion {
	return PluginVersion{Version: v, MinMcharbor: floor, DownloadURL: "file:///" + v}
}

// TestPluginChannelSelection is the repair table: stable or prerelease host,
// plugin on or off. The prerelease row is listed first on purpose, because list
// position must never decide what a repair installs.
func TestPluginChannelSelection(t *testing.T) {
	pre := preRow("2026.11.02-beta.1", ">=2026.11.01")
	s1 := stableRow("2026.11.01")
	s0 := stableRow("2026.10.01")
	both := []PluginVersion{pre, s1, s0}

	rows := []struct {
		name      string
		versions  []PluginVersion
		installed string
		host      string
		on        bool
		changed   bool
		want      string // empty means no target
	}{
		{"on, exact match wins", both, "2026.10.01", stableHost, true, false, "2026.10.01"},
		{"on, stable, no release change keeps the exact stable", both, "2026.11.01", stableHost, true, true, "2026.11.01"},
		{"on, installed gone, release changed: newest incl. prerelease", []PluginVersion{pre, s1}, "2026.09.01", stableHost, true, true, "2026.11.02-beta.1"},
		{"on, installed gone, release unchanged: no target", both, "2026.09.01", stableHost, true, false, ""},
		{"on, prerelease installed and carried: exact", both, "2026.11.02-beta.1", stableHost, true, false, "2026.11.02-beta.1"},
		{"off, stable installed: exact as today", both, "2026.10.01", stableHost, false, false, "2026.10.01"},
		{"off, stable gone, release changed: prerelease skipped", []PluginVersion{pre, s1}, "2026.09.01", stableHost, false, true, "2026.11.01"},
		{"off, only a prerelease exists: no target", []PluginVersion{pre}, "2026.09.01", stableHost, false, true, ""},
		{"off, prerelease installed, release changed: newest stable", both, "2026.11.02-beta.1", stableHost, false, true, "2026.11.01"},
		{"off, prerelease installed, release changed, no stable: no target", []PluginVersion{pre}, "2026.11.02-beta.1", stableHost, false, true, ""},
		{"off, prerelease installed, build stale only: no target", both, "2026.11.02-beta.1", stableHost, false, false, ""},
		{"off, prerelease installed, stable below floor: no target", []PluginVersion{pre, preRow("2026.11.00", ">=9999.01.01")}, "2026.11.02-beta.1", stableHost, false, true, ""},
		{"prerelease host takes the prerelease meeting the floor", []PluginVersion{s1, preRow("2026.11.02-beta.1", ">=2026.11.02-beta.1")}, "2026.10.01", preHost, false, true, "2026.11.02-beta.1"},
		{"prerelease host with no prerelease meeting the floor takes the newest stable", []PluginVersion{s0, s1, preRow("2026.11.03-beta.1", ">=2026.11.03-beta.1")}, "2026.09.01", preHost, false, true, "2026.11.01"},
		{"prerelease host, no release change, exact build", []PluginVersion{s1, preRow("2026.11.02-beta.1", ">=2026.11.02-beta.1")}, "2026.11.02-beta.1", preHost, false, false, "2026.11.02-beta.1"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			got, ok := pickRepair(r.versions, r.installed, r.host, r.on, r.changed)
			if r.want == "" {
				if ok {
					t.Fatalf("picked %s, want no target", got.Version)
				}
				return
			}
			if !ok || got.Version != r.want {
				t.Fatalf("picked %q (ok=%v), want %q", got.Version, ok, r.want)
			}
		})
	}
}

// TestCatalogOffersPrereleaseOnlyToOptedPlugins covers AC8 on a stable host.
func TestCatalogOffersPrereleaseOnlyToOptedPlugins(t *testing.T) {
	entries := []PluginEntry{
		{Slug: "a", Versions: []PluginVersion{stableRow("2026.11.01"), preRow("2026.11.02-beta.1", ">=2026.11.01")}},
		{Slug: "b", Versions: []PluginVersion{stableRow("2026.11.01"), preRow("2026.11.02-beta.1", ">=2026.11.01")}},
		{Slug: "only-pre", Versions: []PluginVersion{preRow("2026.11.02-beta.1", ">=2026.11.01")}},
	}
	st := channelState{overrides: map[string]bool{"a": true}}
	got := filterChannel(entries, st, stableHost)
	if len(got) != 2 || len(got[0].Versions) != 2 || len(got[1].Versions) != 1 || got[1].Version != "2026.11.01" {
		t.Fatalf("override on a, global off: %+v", got)
	}
	st = channelState{global: true, overrides: map[string]bool{"b": false}}
	got = filterChannel(entries, st, stableHost)
	if len(got) != 3 || len(got[0].Versions) != 2 || len(got[1].Versions) != 2 {
		t.Fatalf("global on, a stored false on b must not beat it: %+v", got)
	}
	if len(entries[1].Versions) != 2 {
		t.Fatal("filtering mutated the raw catalog the repair reads")
	}
}

func TestPickLatestFollowsTheChannel(t *testing.T) {
	versions := []PluginVersion{stableRow("2026.11.01"), preRow("2026.11.02-beta.1", ">=2026.11.01")}
	if v, ok := pickLatest(versions, stableHost, false); !ok || v.Version != "2026.11.01" {
		t.Fatalf("off: %+v %v", v, ok)
	}
	if v, ok := pickLatest(versions, stableHost, true); !ok || v.Version != "2026.11.02-beta.1" {
		t.Fatalf("on: %+v %v", v, ok)
	}
}

func channelService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	database := migratedPlugins(t)
	return NewService(database, nil, ""), database
}

func mustChannelState(t *testing.T, svc *Service) channelState {
	t.Helper()
	st, err := svc.channelState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestOverrideStoreKeepsConcurrentSaves(t *testing.T) {
	svc, _ := channelService(t)
	ctx := context.Background()
	on, off := true, false
	var wg sync.WaitGroup
	for _, slug := range []string{"alpha", "beta", "gamma", "delta"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.SetChannelOverride(ctx, slug, &on); err != nil {
				t.Errorf("%s: %v", slug, err)
			}
		}()
	}
	wg.Wait()
	st, err := svc.channelState(ctx)
	if err != nil || len(st.overrides) != 4 {
		t.Fatalf("a concurrent save was lost: %v %v", st.overrides, err)
	}
	old, err := svc.SetChannelOverride(ctx, "alpha", &off)
	if err != nil || old == nil || !*old {
		t.Fatalf("old value: %v %v", old, err)
	}
	if _, err := svc.SetChannelOverride(ctx, "alpha", nil); err != nil {
		t.Fatal(err)
	}
	if st = mustChannelState(t, svc); len(st.overrides) != 3 {
		t.Fatalf("clear left %v", st.overrides)
	}
	for _, bad := range []string{"", "../x", "A", "a/b", "a b", strings.Repeat("a", 65)} {
		if _, err := svc.SetChannelOverride(ctx, bad, &on); err == nil {
			t.Errorf("slug %q was accepted", bad)
		}
	}
}

func TestGlobalSwitchAndOverrideCombine(t *testing.T) {
	svc, database := channelService(t)
	ctx := context.Background()
	on := true
	if _, _, err := updatechannel.Apply(ctx, database, updatechannel.Patch{PluginPrereleases: &on}); err != nil {
		t.Fatal(err)
	}
	off := false
	if _, err := svc.SetChannelOverride(ctx, "quiet", &off); err != nil {
		t.Fatal(err)
	}
	st := svc.channelStateOrOff(ctx)
	if !st.prereleases("loud") || !st.prereleases("quiet") {
		t.Fatalf("global on, a stored false on quiet must not beat it: %+v", st)
	}
}

type channelRig struct {
	handler http.Handler
	db      *sql.DB
	svc     *Service
}

func newChannelRig(t *testing.T, perms string) *channelRig {
	t.Helper()
	svc, database := channelService(t)
	if err := rbac.UpsertCatalog(database); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r1', 'r1', '` + perms + `')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur1', 'u1', 'r1')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	app := &router.AppDeps{RBACService: rbac.NewService(database)}
	h := NewHandler(nil, svc, audit.NewLogger(database, "k"))
	mux := chi.NewRouter()
	mountRoutes(apispec.NewRouter(mux, apispec.NewRegistry(), "/api", apispec.GroupProtected), app, h)
	return &channelRig{handler: mux, db: database, svc: svc}
}

func (c *channelRig) put(slug, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/appstore/plugins/"+slug+"/channel", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "op"}))
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, req)
	return w
}

func (c *channelRig) auditRows(t *testing.T) []string {
	t.Helper()
	rows, err := c.db.Query(`SELECT entity_id || '|' || state_before || '|' || state_after FROM audit_logs WHERE action = 'plugin_channel_change' ORDER BY timestamp`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			t.Errorf("close rows: %v", cErr)
		}
	}()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestChannelRouteIsGatedAndAudited(t *testing.T) {
	denied := newChannelRig(t, `["plugins.view"]`)
	if w := denied.put("comm-ntfy", `{"prereleases":true}`); w.Code != http.StatusForbidden {
		t.Fatalf("a viewer reached the override: %d %s", w.Code, w.Body.String())
	}
	if st := mustChannelState(t, denied.svc); len(st.overrides) != 0 {
		t.Fatalf("a refused request still wrote: %v", st.overrides)
	}

	ok := newChannelRig(t, `["plugins.manage"]`)
	if w := ok.put("comm-ntfy", `{"prereleases":true}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"prereleases":true`) {
		t.Fatalf("set: %d %s", w.Code, w.Body.String())
	}
	if w := ok.put("comm-ntfy", `{"prereleases":true}`); w.Code != http.StatusOK {
		t.Fatalf("repeat: %d", w.Code)
	}
	if w := ok.put("comm-ntfy", `{"prereleases":null}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"prereleases":null`) {
		t.Fatalf("clear: %d %s", w.Code, w.Body.String())
	}
	got := ok.auditRows(t)
	want := []string{`comm-ntfy|{"prereleases":null}|{"prereleases":true}`, `comm-ntfy|{"prereleases":true}|{"prereleases":null}`}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("audit rows (a repeat writes none): %q, want %q", got, want)
	}
}

func TestChannelRouteRefusesHostileInput(t *testing.T) {
	rig := newChannelRig(t, `["plugins.manage"]`)
	for name, c := range map[string]struct{ slug, body string }{
		"dotted slug":    {"..%2Fx", `{"prereleases":true}`},
		"upper slug":     {"Comm", `{"prereleases":true}`},
		"missing key":    {"comm-ntfy", `{}`},
		"string value":   {"comm-ntfy", `{"prereleases":"true"}`},
		"unknown field":  {"comm-ntfy", `{"prereleases":true,"x":1}`},
		"oversized body": {"comm-ntfy", `{"prereleases":true,"pad":"` + strings.Repeat("a", 5000) + `"}`},
	} {
		if w := rig.put(c.slug, c.body); w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if st := mustChannelState(t, rig.svc); len(st.overrides) != 0 {
		t.Fatalf("hostile input wrote %v", st.overrides)
	}
}

// writeCatalogEntry publishes a catalog file for the upgrade fixture with the
// given version rows.
func writeCatalogEntry(t *testing.T, f *upgradeFixture, rows string) {
	t.Helper()
	body := `{"slug":"` + upgradeSlug + `","name":"Up","versions":[` + rows + `]}`
	if err := os.WriteFile(filepath.Join(f.catalog, upgradeSlug+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestReconcileKeepsAPluginWithNoChannelTarget: a prerelease installed with the
// setting off and nothing stable to move to stays installed and enabled, whether
// the release changed or only the build did.
func TestReconcileKeepsAPluginWithNoChannelTarget(t *testing.T) {
	if versionIsPre := strings.Contains(appversion.Current(), "-"); versionIsPre {
		t.Skip("the rows describe a stable host")
	}
	for name, builtFor := range map[string]string{"release changed": otherRelease, "build stale only": appversion.Current()} {
		f := newUpgradeFixture(t)
		const pre = "2026.11.02-beta.1"
		f.exec(`UPDATE appstore_plugins SET version = ?, built_for = ?, build = 'old' WHERE slug = ?`, pre, builtFor, upgradeSlug)
		writeCatalogEntry(t, f, `{"version":"`+pre+`","build":"new","min_mcharbor":">=2026.11.01","download_url":"`+f.url+`"}`)
		f.serve.Store(func(int64) []byte { return nil })
		f.reconcile()
		if n := f.downloads.Load(); n != 0 {
			t.Errorf("%s: reconcile downloaded %d artifacts, want none", name, n)
		}
		var version string
		var enabled bool
		if err := f.inst.db.QueryRowContext(f.ctx, `SELECT version, enabled FROM appstore_plugins WHERE slug = ?`, upgradeSlug).Scan(&version, &enabled); err != nil {
			t.Fatal(err)
		}
		if version != pre || !enabled {
			t.Errorf("%s: row is %q enabled=%v, want it kept", name, version, enabled)
		}
	}
}

// TestReconcileLeavesTheChannelOnAReleaseChange: with a stable available, a
// prerelease install whose setting is off is reinstalled; with the override on
// and a prerelease carried nothing changes release-wise.
func TestReconcileLeavesTheChannelOnAReleaseChange(t *testing.T) {
	if strings.Contains(appversion.Current(), "-") {
		t.Skip("the rows describe a stable host")
	}
	f := newUpgradeFixture(t)
	const pre = "2026.11.02-beta.1"
	f.exec(`UPDATE appstore_plugins SET version = ?, built_for = ? WHERE slug = ?`, pre, otherRelease, upgradeSlug)
	f.serve.Store(func(int64) []byte { return tarballWith(t, "2026.10.01", "", nil) })
	writeCatalogEntry(t, f,
		`{"version":"`+pre+`","min_mcharbor":">=2026.11.01","download_url":"`+f.url+`"},`+
			`{"version":"2026.10.01","min_mcharbor":">=2026.09.01","download_url":"`+f.url+`"}`)
	f.reconcile()
	if n := f.downloads.Load(); n == 0 {
		t.Fatal("setting off, release changed, a stable exists: the repair never reached the stable build")
	}
}

func TestRestoreKeepsTheExactVersion(t *testing.T) {
	f := newUpgradeFixture(t)
	const pre = "2026.11.02-beta.1"
	writeCatalogEntry(t, f,
		`{"version":"`+pre+`","min_mcharbor":">=2026.11.01","download_url":"`+f.url+`"},`+
			`{"version":"2026.10.01","min_mcharbor":">=2026.09.01","download_url":"`+f.url+`"}`)
	src, err := f.h.catalogSourceExact(f.ctx, upgradeSlug, pre)
	if err != nil || src.URL != f.url {
		t.Fatalf("a restore of a recorded prerelease with the setting off: %+v %v", src, err)
	}
	if _, err := f.h.catalogSourceExact(f.ctx, upgradeSlug, "2026.01.01"); err == nil {
		t.Fatal("a version the catalog lacks must fail, not move")
	}
}

func TestCatalogEntriesCarryTheStoredOverride(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/catalog/plugins/index.json", func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `[
			{"slug":"comm-a","name":"A","versions":[{"version":"2026.10.01","download_url":"https://x/a.tar.gz"}]},
			{"slug":"comm-b","name":"B","versions":[{"version":"2026.10.01","download_url":"https://x/b.tar.gz"}]},
			{"slug":"comm-c","name":"C","versions":[{"version":"2026.10.01","download_url":"https://x/c.tar.gz"}]}
		]`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	previous := remoteHTTPClient
	remoteHTTPClient = srv.Client()
	defer func() { remoteHTTPClient = previous }()

	database := migratedPlugins(t)
	svc := NewService(database, fixedSources{{Name: "remote", URL: srv.URL + "/catalog/plugins/index.json"}}, "")
	ctx := context.Background()
	on, off := true, false
	for slug, v := range map[string]*bool{"comm-a": &on, "comm-b": &off} {
		if _, err := svc.SetChannelOverride(ctx, slug, v); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := svc.ListCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*bool{}
	for _, e := range entries {
		got[e.Slug] = e.PrereleaseOverride
	}
	if got["comm-a"] == nil || !*got["comm-a"] || got["comm-b"] == nil || *got["comm-b"] || got["comm-c"] != nil {
		t.Fatalf("overrides on the catalog = a:%v b:%v c:%v, want true, false, nil", got["comm-a"], got["comm-b"], got["comm-c"])
	}
}
