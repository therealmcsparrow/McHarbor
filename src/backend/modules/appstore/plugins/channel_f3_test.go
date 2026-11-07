// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/core/versionorder"
)

const f3Pre = "2026.11.02-beta.1"

// asHost stands a host release in for the running one for the length of a test.
func asHost(t *testing.T, version string) {
	t.Helper()
	old := hostVersion
	hostVersion = func() string { return version }
	t.Cleanup(func() { hostVersion = old })
}

// writeCatalogSplit publishes the entry the way plugin-catalog v2 writes it:
// stable rows under versions, prerelease rows under prerelease_versions.
func writeCatalogSplit(t *testing.T, f *upgradeFixture, stable, pre string) {
	t.Helper()
	body := `{"slug":"` + upgradeSlug + `","name":"Up","versions":[` + stable + `],"prerelease_versions":[` + pre + `]}`
	if err := os.WriteFile(filepath.Join(f.catalog, upgradeSlug+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *upgradeFixture) stableAndPre(preFloor string) {
	f.t.Helper()
	writeCatalogSplit(f.t, f,
		`{"version":"2026.10.01","min_mcharbor":">=2026.09.01","download_url":"`+f.url+`"}`,
		`{"version":"`+f3Pre+`","min_mcharbor":"`+preFloor+`","download_url":"`+f.url+`"}`)
}

func (f *upgradeFixture) installedAt() string {
	f.t.Helper()
	prev, _, ok, err := f.svc.installedState(f.ctx, upgradeSlug)
	if err != nil || !ok {
		f.t.Fatalf("reading the installed row: ok=%v err=%v", ok, err)
	}
	return prev.Version
}

func (f *upgradeFixture) setOverride(on bool) {
	f.t.Helper()
	if _, err := f.svc.SetChannelOverride(f.ctx, upgradeSlug, &on); err != nil {
		f.t.Fatal(err)
	}
}

// TestEffectiveSettingIsGlobalOrOverride: an override can only add a plugin to the
// channel, so a stored false never beats the global switch.
func TestEffectiveSettingIsGlobalOrOverride(t *testing.T) {
	on, off := true, false
	for _, c := range []struct {
		name     string
		global   bool
		override *bool
		want     bool
	}{
		{"nothing set", false, nil, false},
		{"override on", false, &on, true},
		{"override off, global off", false, &off, false},
		{"global on", true, nil, true},
		{"global on, stored false", true, &off, true},
		{"global on, override on", true, &on, true},
	} {
		st := channelState{global: c.global, overrides: map[string]bool{}}
		if c.override != nil {
			st.overrides["p"] = *c.override
		}
		if got := st.prereleases("p"); got != c.want {
			t.Errorf("%s: effective = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestFilterChannelDropsUnparsableVersions: a row whose version does not parse is
// never read as stable, whatever the plugin's setting.
func TestFilterChannelDropsUnparsableVersions(t *testing.T) {
	bad := PluginVersion{Version: "2026.11.02-rc..1", DownloadURL: "file:///bad"}
	for name, st := range map[string]channelState{
		"off":      {overrides: map[string]bool{}},
		"opted in": {overrides: map[string]bool{"p": true}},
	} {
		got := filterChannel([]PluginEntry{{Slug: "p", Versions: []PluginVersion{bad, stableRow("2026.11.01")}}}, st, stableHost)
		if len(got) != 1 || len(got[0].Versions) != 1 || got[0].Versions[0].Version != "2026.11.01" || got[0].Version != "2026.11.01" {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if got := filterChannel([]PluginEntry{{Slug: "p", Versions: []PluginVersion{bad}}}, channelState{overrides: map[string]bool{}}, stableHost); len(got) != 0 {
		t.Errorf("an entry with only an unparsable row was offered: %+v", got)
	}
}

// TestReconcileMovesAStablePluginOntoAPrereleaseHost covers AC9: a stable plugin the
// catalog still carries follows a host that moved to a prerelease, and one with no
// prerelease build to follow is left alone.
func TestReconcileMovesAStablePluginOntoAPrereleaseHost(t *testing.T) {
	if versionorder.IsPrerelease(appversion.Current()) {
		t.Skip("the rows describe a stable tree")
	}
	asHost(t, f3Pre)

	f := newUpgradeFixture(t)
	f.exec(`UPDATE appstore_plugins SET built_for = ? WHERE slug = ?`, otherRelease, upgradeSlug)
	f.serve.Store(func(int64) []byte { return tarballWith(t, f3Pre, "", nil) })
	f.stableAndPre(">=2026.11.01")
	f.reconcile()
	if got := f.installedAt(); got != f3Pre {
		t.Fatalf("the stable plugin stayed at %q on a prerelease host that has its prerelease build", got)
	}

	g := newUpgradeFixture(t)
	g.exec(`UPDATE appstore_plugins SET built_for = ? WHERE slug = ?`, otherRelease, upgradeSlug)
	g.serve.Store(func(int64) []byte { return nil })
	writeCatalogSplit(t, g, `{"version":"2026.10.01","min_mcharbor":">=2026.09.01","download_url":"`+g.url+`"}`, ``)
	g.reconcile()
	if n := g.downloads.Load(); n != 0 || g.installedAt() != "2026.10.01" {
		t.Fatalf("no prerelease build: downloads=%d installed=%q, want the stable kept untouched", n, g.installedAt())
	}
}

func (f *upgradeFixture) postReconcile(version string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Post("/appstore/plugins/{slug}/reconcile", f.h.handleReconcile)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/appstore/plugins/"+upgradeSlug+"/reconcile",
		strings.NewReader(`{"version":"`+version+`","allow_unsigned":true}`)))
	return w
}

// TestReconcileRouteHonoursTheChannel covers AC4 for the operator route: a stable
// host with the plugin's setting off refuses a prerelease version it was named.
func TestReconcileRouteHonoursTheChannel(t *testing.T) {
	if versionorder.IsPrerelease(appversion.Current()) {
		t.Skip("the rows describe a stable tree")
	}
	f := newUpgradeFixture(t)
	f.serve.Store(func(int64) []byte { return tarballWith(t, f3Pre, "", nil) })
	f.stableAndPre(">=2026.11.01")

	w := f.postReconcile(f3Pre)
	if w.Code == http.StatusOK || f.downloads.Load() != 0 || f.installedAt() != "2026.10.01" {
		t.Fatalf("a prerelease installed with the setting off: %d %s downloads=%d installed=%q",
			w.Code, w.Body.String(), f.downloads.Load(), f.installedAt())
	}

	f.setOverride(true)
	if w := f.postReconcile(f3Pre); w.Code != http.StatusOK || f.installedAt() != f3Pre {
		t.Fatalf("with the override on: %d %s installed=%q", w.Code, w.Body.String(), f.installedAt())
	}
}

func (f *upgradeFixture) postInstallStream(version string) string {
	f.t.Helper()
	body, err := json.Marshal(InstallRequest{Tarball: tarballWith(f.t, version, "", nil), AllowUnsigned: true})
	if err != nil {
		f.t.Fatal(err)
	}
	w := httptest.NewRecorder()
	f.h.handleInstallStream(w, httptest.NewRequest(http.MethodPost, "/appstore/plugins/install/stream", strings.NewReader(string(body))))
	return w.Body.String()
}

// TestInstallStreamRefusesASuffixedManifestOnAStableHost covers AC4 for a sideload:
// the manifest is read after staging, and a prerelease is refused unless the plugin's
// own override is on.
func TestInstallStreamRefusesASuffixedManifestOnAStableHost(t *testing.T) {
	if versionorder.IsPrerelease(appversion.Current()) {
		t.Skip("the rows describe a stable tree")
	}
	f := newUpgradeFixture(t)
	if _, err := f.inst.Uninstall(f.ctx, upgradeSlug, UninstallOptions{PurgeData: true}, nil); err != nil {
		t.Fatal(err)
	}

	out := f.postInstallStream(f3Pre)
	if !strings.Contains(out, `"status":"error"`) || strings.Contains(out, `"status":"done"`) {
		t.Fatalf("a prerelease sideloaded with the setting off: %s", out)
	}
	if v, err := f.svc.LookupVersion(f.ctx, upgradeSlug); err != nil || v != "" {
		t.Fatalf("a refused sideload left a row at %q (%v)", v, err)
	}
	staging, err := filepath.Glob(filepath.Join(f.dataDir, "plugin-staging-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(staging) != 0 {
		t.Fatalf("a refused sideload left staging directories: %v", staging)
	}
	if _, err := os.Stat(filepath.Join(f.dataDir, "plugins", upgradeSlug+"@"+f3Pre)); !os.IsNotExist(err) {
		t.Fatalf("a refused sideload left an install directory (stat err=%v)", err)
	}

	if out := f.postInstallStream("2026.10.05"); !strings.Contains(out, `"status":"done"`) {
		t.Fatalf("a stable sideload was refused: %s", out)
	}
	if _, err := f.inst.Uninstall(f.ctx, upgradeSlug, UninstallOptions{PurgeData: true}, nil); err != nil {
		t.Fatal(err)
	}
	f.setOverride(true)
	if out := f.postInstallStream(f3Pre); !strings.Contains(out, `"status":"done"`) {
		t.Fatalf("a prerelease sideload with the override on was refused: %s", out)
	}
}

// TestReconcileClearsAStaleReturnBlock: once the plugin no longer leaves its channel
// (the override came on after a blocked return), no return is pending and the
// "return blocked" reason must go.
func TestReconcileClearsAStaleReturnBlock(t *testing.T) {
	rf := newReturnFixture(t)
	rf.reinstall(retPre, migB)
	reason := ReturnBlockedPrefix + `"0002_b.sql"`
	rf.exec(`UPDATE appstore_plugins SET built_for = ?, repair_error = ? WHERE slug = ?`, otherRelease, reason, upgradeSlug)
	rf.publishRows(
		rf.offer(retPre, tarballWith(t, retPre, "", migB), true),
		rf.offer(retStable, tarballWith(t, retStable, "", migA), true))

	rf.reconcile()
	if _, got, _ := rf.row(); got != reason {
		t.Fatalf("setting off: the blocked return must stay recorded, got %q", got)
	}

	rf.setOverride(true)
	rf.reconcile()
	builtFor, got, _ := rf.row()
	if got != "" || builtFor != appversion.Current() {
		t.Fatalf("override on, nothing to return: repair_error=%q built_for=%q, want it cleared and stamped", got, builtFor)
	}
	if v := rf.installedVersion(); v != retPre {
		t.Fatalf("installed %q, want the prerelease kept", v)
	}
}

// TestReturnClearsTheOverrideOnTheServer: the clear lives in the return operation, so
// the CLI and a direct API call get it as well as the UI.
func TestReturnClearsTheOverrideOnTheServer(t *testing.T) {
	rr := newReturnRig(t, `["plugins.manage"]`)
	rr.reinstall(retPre, migB)
	rr.publishRows(rr.offer(retStable, tarballWith(t, retStable, "", migB), true))
	rr.setOverride(true)

	if w := rr.call(http.MethodPost, returnPath, `{}`); w.Code != http.StatusOK {
		t.Fatalf("return: %d %s", w.Code, w.Body.String())
	}
	if st := mustChannelState(t, rr.svc); len(st.overrides) != 0 {
		t.Fatalf("the override survived the return: %v", st.overrides)
	}

	blocked := newReturnRig(t, `["plugins.manage"]`)
	blocked.reinstall(retPre, migB)
	blocked.publishRows(blocked.offer(retStable, tarballWith(t, retStable, "", migA), true))
	blocked.setOverride(true)
	if w := blocked.call(http.MethodPost, returnPath, `{}`); w.Code == http.StatusOK {
		t.Fatalf("expected the blocked return to be refused: %d", w.Code)
	}
	if st := mustChannelState(t, blocked.svc); !st.overrides[upgradeSlug] {
		t.Fatalf("a refused return cleared the override: %v", st.overrides)
	}
}

// TestReturnAuditRecordsUnsignedConsent: consent to unsigned code is part of the record.
func TestReturnAuditRecordsUnsignedConsent(t *testing.T) {
	rr := newReturnRig(t, `["plugins.manage"]`)
	rr.reinstall(retPre, migB)
	rr.publishRows(rr.offer(retStable, tarballWith(t, retStable, "", migB), false))
	if w := rr.call(http.MethodPost, returnPath, `{"allow_unsigned":true}`); w.Code != http.StatusOK {
		t.Fatalf("return: %d %s", w.Code, w.Body.String())
	}
	var details string
	if err := rr.inst.db.QueryRow(`SELECT details FROM audit_logs WHERE action = 'plugin_return'`).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(details, "unsigned") {
		t.Fatalf("audit details %q do not record that unsigned code was consented to", details)
	}

	signed := newReturnRig(t, `["plugins.manage"]`)
	signed.reinstall(retPre, migB)
	signed.publishRows(signed.offer(retStable, tarballWith(t, retStable, "", migB), true))
	if w := signed.call(http.MethodPost, returnPath, `{"allow_unsigned":true}`); w.Code != http.StatusOK {
		t.Fatalf("signed return: %d %s", w.Code, w.Body.String())
	}
	if err := signed.inst.db.QueryRow(`SELECT details FROM audit_logs WHERE action = 'plugin_return'`).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(details, "unsigned") {
		t.Fatalf("a signed return was recorded as unsigned: %q", details)
	}
}
