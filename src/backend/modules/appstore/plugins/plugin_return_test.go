// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/core/versionorder"
)

const (
	retPre    = "2026.10.02-beta.1"
	retStable = "2026.10.03"
)

var (
	migA = map[string]string{"migrations/0001_a.sql": "CREATE TABLE plugin_comm_up_a (id TEXT);"}
	migB = map[string]string{
		"migrations/0001_a.sql": "CREATE TABLE plugin_comm_up_a (id TEXT);",
		"migrations/0002_b.sql": "CREATE TABLE plugin_comm_up_b (id TEXT);",
	}
)

// returnFixture is an upgrade fixture whose host trusts one key and whose catalog
// is served from artifacts it registers, so a signed and an unsigned target can
// sit side by side.
type returnFixture struct {
	*upgradeFixture
	priv      ed25519.PrivateKey
	base      string
	mu        sync.Mutex
	artifacts map[string][]byte
}

func newReturnFixture(t *testing.T) *returnFixture {
	t.Helper()
	if versionorder.IsPrerelease(appversion.Current()) {
		t.Skip("these rows describe a stable host")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rf := &returnFixture{upgradeFixture: newUpgradeFixture(t), priv: priv, artifacts: map[string][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rf.mu.Lock()
		body, ok := rf.artifacts[r.URL.Path]
		rf.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		if _, err := w.Write(body); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	rf.base = srv.URL
	rf.inst.trustKeys = []ed25519.PublicKey{pub}
	return rf
}

// reinstall replaces the starting install with version and migration files.
func (rf *returnFixture) reinstall(version string, files map[string]string) {
	rf.t.Helper()
	if _, err := rf.inst.Uninstall(rf.ctx, upgradeSlug, UninstallOptions{PurgeData: true}, nil); err != nil {
		rf.t.Fatal(err)
	}
	if _, err := rf.inst.Install(rf.ctx, Source{Tarball: tarballWith(rf.t, version, "", files), AllowUnsigned: true}, nil); err != nil {
		rf.t.Fatal(err)
	}
}

// offer registers a tarball and returns its catalog row.
func (rf *returnFixture) offer(version string, tarball []byte, signed bool) string {
	rf.t.Helper()
	path := "/" + version + ".tar.gz"
	rf.mu.Lock()
	rf.artifacts[path] = tarball
	rf.mu.Unlock()
	row := map[string]string{"version": version, "download_url": rf.base + path, "min_mcharbor": ">=2026.09.01"}
	if signed {
		row["signature"] = base64.StdEncoding.EncodeToString(ed25519.Sign(rf.priv, tarball))
	}
	b, err := json.Marshal(row)
	if err != nil {
		rf.t.Fatal(err)
	}
	return string(b)
}

func (rf *returnFixture) publishRows(rows ...string) {
	rf.t.Helper()
	writeCatalogEntry(rf.t, rf.upgradeFixture, strings.Join(rows, ","))
}

// installedVersion reads the installed row's version, failing the test on a read error.
func (rf *returnFixture) installedVersion() string {
	rf.t.Helper()
	prev, _, ok, err := rf.svc.installedState(rf.ctx, upgradeSlug)
	if err != nil || !ok {
		rf.t.Fatalf("reading the installed row: ok=%v err=%v", ok, err)
	}
	return prev.Version
}

// state is everything a dry-run must leave alone: the row, the ledger, the
// install directory and any staging directory.
func (rf *returnFixture) state() string {
	rf.t.Helper()
	var version, builtFor, repairErr, updated string
	var enabled bool
	if err := rf.inst.db.QueryRowContext(rf.ctx,
		`SELECT version, built_for, repair_error, enabled, updated_at FROM appstore_plugins WHERE slug = ?`, upgradeSlug).
		Scan(&version, &builtFor, &repairErr, &enabled, &updated); err != nil {
		rf.t.Fatal(err)
	}
	applied, err := rf.inst.appliedMigrations(rf.ctx, upgradeSlug)
	if err != nil {
		rf.t.Fatal(err)
	}
	staging, err := filepath.Glob(filepath.Join(rf.dataDir, "plugin-staging-*"))
	if err != nil {
		rf.t.Fatal(err)
	}
	_, statErr := os.Stat(filepath.Join(rf.dataDir, "plugins", upgradeSlug+"@"+version, "plugin.yaml"))
	return fmt.Sprintf("%s|%s|%s|%v|%s|ledger=%d|dir=%v|staging=%d",
		version, builtFor, repairErr, enabled, updated, len(applied), statErr == nil, len(staging))
}

// TestLedgerCheckedOnReturn is the AC13 table: only a prerelease moving to a
// stable is checked, and a refusal names the file and changes nothing.
func TestLedgerCheckedOnReturn(t *testing.T) {
	cases := []struct {
		name      string
		installed string
		have      map[string]string
		target    string
		carries   map[string]string
		blocked   []string
	}{
		{"prerelease to a stable lacking a ledgered file", retPre, migB, retStable, migA, []string{"0002_b.sql"}},
		{"prerelease to a stable carrying them all", retPre, migB, retStable, migB, nil},
		{"stable to a newer stable that drops one still installs", "2026.10.01", migB, retStable, migA, nil},
		{"prerelease to a newer prerelease that drops one still installs", retPre, migB, "2026.10.03-beta.2", migA, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rf := newReturnFixture(t)
			rf.reinstall(c.installed, c.have)
			before := rf.state()
			_, err := rf.h.upgrade(rf.ctx, upgradeSlug,
				Source{Tarball: tarballWith(t, c.target, "", c.carries), AllowUnsigned: true}, nil)
			if c.blocked == nil {
				if err != nil {
					t.Fatalf("upgrade refused: %v", err)
				}
				if v := rf.installedVersion(); v != c.target {
					t.Fatalf("installed %q, want %q", v, c.target)
				}
				return
			}
			var blocked *MigrationBlockError
			if !errors.As(err, &blocked) || !errors.Is(err, ErrUpgradeRefused) || !errors.Is(err, ErrMigrationBlocksReturn) {
				t.Fatalf("err = %v, want a MigrationBlockError wrapping ErrUpgradeRefused", err)
			}
			if strings.Join(blocked.Files, ",") != strings.Join(c.blocked, ",") {
				t.Fatalf("blocked %v, want %v", blocked.Files, c.blocked)
			}
			if !strings.HasPrefix(err.Error(), ReturnBlockedPrefix) {
				t.Fatalf("reason %q lacks the fixed prefix", err.Error())
			}
			if after := rf.state(); after != before {
				t.Fatalf("a refused return changed state:\n%s\n%s", before, after)
			}
		})
	}
}

func TestReturnCheckReportsEachOutcome(t *testing.T) {
	rf := newReturnFixture(t)
	rf.reinstall(retPre, migB)

	check := func() ReturnCheckResult {
		t.Helper()
		res, err := rf.h.ReturnCheck(rf.ctx, upgradeSlug)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	before := rf.state()

	rf.publishRows(rf.offer(retPre, tarballWith(t, retPre, "", migB), true))
	if res := check(); res.Outcome != OutcomeNoStable || res.OK {
		t.Fatalf("prerelease-only catalog: %+v", res)
	}

	rf.publishRows(rf.offer(retStable, tarballWith(t, retStable, "", migB), true))
	if res := check(); res.Outcome != OutcomeClear || !res.OK || res.TargetVersion != retStable || res.Unsigned {
		t.Fatalf("clear: %+v", res)
	}

	rf.publishRows(rf.offer(retStable, tarballWith(t, retStable, "", migA), true))
	res := check()
	if res.Outcome != OutcomeBlocked || res.OK || strings.Join(res.BlockingMigrations, ",") != "0002_b.sql" || res.BlockingMigration != "0002_b.sql" {
		t.Fatalf("blocked: %+v", res)
	}

	rf.publishRows(rf.offer(retStable, tarballWith(t, retStable, "", migB), false))
	if res := check(); res.Outcome != OutcomeUnsigned || res.OK || !res.Unsigned || res.TargetVersion != retStable {
		t.Fatalf("unsigned: %+v", res)
	}

	if after := rf.state(); after != before {
		t.Fatalf("the dry-run changed state:\n%s\n%s", before, after)
	}
}

// TestReturnCheckUnpacksNothingUnsigned: the target is unsigned and hostile (a
// traversal entry). The GET reports unsigned, names no file, extracts nothing,
// and the entry never lands outside the staging area.
func TestReturnCheckUnpacksNothingUnsigned(t *testing.T) {
	rf := newReturnFixture(t)
	rf.reinstall(retPre, migB)
	hostile := tarballWith(t, retStable, "", map[string]string{"../evil.txt": "x", "migrations/0001_a.sql": "SELECT 1;"})
	rf.publishRows(rf.offer(retStable, hostile, false))
	before := rf.state()

	res, err := rf.h.ReturnCheck(rf.ctx, upgradeSlug)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != OutcomeUnsigned || len(res.BlockingMigrations) != 0 || res.BlockingMigration != "" {
		t.Fatalf("outcome %+v, want unsigned with no file named", res)
	}
	if after := rf.state(); after != before {
		t.Fatalf("state changed:\n%s\n%s", before, after)
	}
	for _, p := range []string{filepath.Join(rf.dataDir, "evil.txt"), filepath.Join(filepath.Dir(rf.dataDir), "evil.txt")} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s was written", p)
		}
	}

	var probe *Staged
	up, ok := rf.h.installer.(Upgrader)
	if !ok {
		t.Fatal("installer is not an Upgrader")
	}
	probe, err = up.Preflight(rf.ctx, Source{URL: rf.base + "/" + retStable + ".tar.gz"}, nil)
	if !errors.Is(err, ErrUnsignedNotConfirmed) || probe != nil {
		t.Fatalf("Preflight without consent = %v, %v; want ErrUnsignedNotConfirmed before any extraction", probe, err)
	}
}

func TestReturnCheckRefusesOnAPrereleaseHost(t *testing.T) {
	rf := newReturnFixture(t)
	rf.reinstall(retPre, migB)
	rf.publishRows(rf.offer(retStable, tarballWith(t, retStable, "", migB), true))
	old := hostVersion
	hostVersion = func() string { return "2026.11.02-beta.1" }
	t.Cleanup(func() { hostVersion = old })

	res, err := rf.h.ReturnCheck(rf.ctx, upgradeSlug)
	if err != nil || res.Outcome != OutcomePrereleaseHost || res.OK {
		t.Fatalf("check = %+v, %v", res, err)
	}
	if _, err := rf.h.returnPlugin(rf.ctx, upgradeSlug, false); !errors.Is(err, ErrReturnOnPrereleaseHost) {
		t.Fatalf("return on a prerelease host: %v", err)
	}
}

type returnRig struct {
	*returnFixture
	handler http.Handler
}

func newReturnRig(t *testing.T, perms string) *returnRig {
	t.Helper()
	rf := newReturnFixture(t)
	if err := rbac.UpsertCatalog(rf.inst.db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r1', 'r1', '` + perms + `')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur1', 'u1', 'r1')`,
	} {
		if _, err := rf.inst.db.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	rf.h.audit = audit.NewLogger(rf.inst.db, "k")
	app := &router.AppDeps{RBACService: rbac.NewService(rf.inst.db)}
	mux := chi.NewRouter()
	mountRoutes(apispec.NewRouter(mux, apispec.NewRegistry(), "/api", apispec.GroupProtected), app, rf.h)
	return &returnRig{returnFixture: rf, handler: mux}
}

func (rr *returnRig) call(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "op"}))
	w := httptest.NewRecorder()
	rr.handler.ServeHTTP(w, req)
	return w
}

func (rr *returnRig) auditRows() []string {
	rr.t.Helper()
	rows, err := rr.inst.db.Query(`SELECT entity_id || '|' || state_before || '|' || state_after FROM audit_logs WHERE action = 'plugin_return' ORDER BY timestamp`)
	if err != nil {
		rr.t.Fatal(err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			rr.t.Errorf("close rows: %v", cErr)
		}
	}()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rr.t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		rr.t.Fatal(err)
	}
	return out
}

const (
	returnPath = "/appstore/plugins/" + upgradeSlug + "/return"
	checkPath  = returnPath + "/check"
)

func TestReturnRoutesAreGated(t *testing.T) {
	rr := newReturnRig(t, `["plugins.view"]`)
	rr.reinstall(retPre, migB)
	rr.publishRows(rr.offer(retStable, tarballWith(t, retStable, "", migB), true))
	before := rr.state()
	if w := rr.call(http.MethodPost, returnPath, `{}`); w.Code != http.StatusForbidden {
		t.Fatalf("a viewer reached return: %d %s", w.Code, w.Body.String())
	}
	if w := rr.call(http.MethodGet, checkPath, ""); w.Code != http.StatusForbidden {
		t.Fatalf("a viewer reached the check: %d %s", w.Code, w.Body.String())
	}
	if after := rr.state(); after != before || len(rr.auditRows()) != 0 {
		t.Fatalf("a refused request changed state:\n%s\n%s", before, after)
	}
}

func TestReturnRouteMovesAndAudits(t *testing.T) {
	rr := newReturnRig(t, `["plugins.manage"]`)
	rr.reinstall(retPre, migB)
	rr.publishRows(rr.offer(retStable, tarballWith(t, retStable, "", migB), false))
	before := rr.state()

	// No consent in the body: the unsigned target refuses and nothing moves.
	w := rr.call(http.MethodPost, returnPath, `{}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "err.plugin.unsigned_confirmation_required") {
		t.Fatalf("unconsented return: %d %s", w.Code, w.Body.String())
	}
	if after := rr.state(); after != before || len(rr.auditRows()) != 0 {
		t.Fatalf("a refusal changed state:\n%s\n%s", before, after)
	}

	w = rr.call(http.MethodPost, returnPath, `{"allow_unsigned":true}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), retStable) {
		t.Fatalf("return: %d %s", w.Code, w.Body.String())
	}
	if v := rr.installedVersion(); v != retStable {
		t.Fatalf("installed %q after the return", v)
	}
	if ledger, err := rr.inst.appliedMigrations(rr.ctx, upgradeSlug); err != nil || len(ledger) != 2 {
		t.Fatalf("plugin data not kept: %v %v", ledger, err)
	}
	want := `comm-up|{"version":"` + retPre + `"}|{"version":"` + retStable + `"}`
	if rows := rr.auditRows(); len(rows) != 1 || rows[0] != want {
		t.Fatalf("audit rows %q, want %q", rows, want)
	}
}

func TestReturnRouteRefusesABlockedOrMissingTarget(t *testing.T) {
	rr := newReturnRig(t, `["plugins.manage"]`)
	rr.reinstall(retPre, migB)
	before := rr.state()

	rr.publishRows(rr.offer(retPre, tarballWith(t, retPre, "", migB), true))
	if w := rr.call(http.MethodPost, returnPath, `{}`); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "err.plugin.no_stable_release") {
		t.Fatalf("no stable: %d %s", w.Code, w.Body.String())
	}

	rr.publishRows(rr.offer(retStable, tarballWith(t, retStable, "", migA), true))
	w := rr.call(http.MethodPost, returnPath, `{}`)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "err.update.migration_blocks_stable") {
		t.Fatalf("blocked confirm: %d %s", w.Code, w.Body.String())
	}
	if after := rr.state(); after != before || len(rr.auditRows()) != 0 {
		t.Fatalf("a refused return changed state:\n%s\n%s", before, after)
	}
}

func TestReturnRoutesRefuseHostileInput(t *testing.T) {
	rr := newReturnRig(t, `["plugins.manage"]`)
	rr.reinstall(retPre, migB)
	rr.publishRows(rr.offer(retStable, tarballWith(t, retStable, "", migB), true))
	before := rr.state()
	for name, c := range map[string]struct{ method, path, body string }{
		"dotted slug":    {http.MethodPost, "/appstore/plugins/..%2Fx/return", `{}`},
		"upper slug":     {http.MethodPost, "/appstore/plugins/Comm/return", `{}`},
		"unknown field":  {http.MethodPost, returnPath, `{"allow_unsigned":false,"x":1}`},
		"string consent": {http.MethodPost, returnPath, `{"allow_unsigned":"true"}`},
		"oversized body": {http.MethodPost, returnPath, `{"pad":"` + strings.Repeat("a", 5000) + `"}`},
		"check slug":     {http.MethodGet, "/appstore/plugins/..%2Fx/return/check", ""},
	} {
		if w := rr.call(c.method, c.path, c.body); w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if after := rr.state(); after != before {
		t.Fatalf("hostile input changed state:\n%s\n%s", before, after)
	}
}

// TestRefusedRepairRecordsWhy: a boot repair that would strand a ledgered
// migration keeps the plugin enabled at its old stamp and says why.
func TestRefusedRepairRecordsWhy(t *testing.T) {
	rf := newReturnFixture(t)
	rf.reinstall(retPre, migB)
	rf.exec(`UPDATE appstore_plugins SET built_for = ? WHERE slug = ?`, otherRelease, upgradeSlug)
	rf.publishRows(rf.offer(retStable, tarballWith(t, retStable, "", migA), true))

	rf.reconcile()

	builtFor, reason, enabled := rf.row()
	if !strings.HasPrefix(reason, ReturnBlockedPrefix) || !strings.Contains(reason, "0002_b.sql") {
		t.Fatalf("repair_error = %q", reason)
	}
	if builtFor != otherRelease || !enabled {
		t.Fatalf("built_for=%q enabled=%v, want the old stamp and still enabled", builtFor, enabled)
	}
	if v := rf.installedVersion(); v != retPre {
		t.Fatalf("installed %q, want the prerelease kept", v)
	}
}

func TestSettleKeepsAReturnBlockedReason(t *testing.T) {
	rf := newReturnFixture(t)
	reason := ReturnBlockedPrefix + `"0002_b.sql"`
	rf.exec(`UPDATE appstore_plugins SET built_for = ?, repair_error = ? WHERE slug = ?`, otherRelease, reason, upgradeSlug)

	rf.h.SettleRepairState(rf.ctx, map[string]string{upgradeSlug: ""}, nil)
	if _, got, enabled := rf.row(); got != reason || !enabled {
		t.Fatalf("a clean load erased the reason: %q enabled=%v", got, enabled)
	}

	const failure = "plugin was built with a different version of package core/agent"
	for range 2 {
		rf.h.SettleRepairState(rf.ctx, map[string]string{upgradeSlug: failure}, nil)
	}
	_, got, _ := rf.row()
	if !strings.HasPrefix(got, reason) || strings.Count(got, failure) != 1 {
		t.Fatalf("a failed load should add its error once: %q", got)
	}

	rf.h.SettleRepairState(rf.ctx, map[string]string{upgradeSlug: ""}, nil)
	if _, got, _ := rf.row(); !strings.HasPrefix(got, reason) || strings.Contains(got, failure) {
		t.Fatalf("a later clean load should drop only the load error: %q", got)
	}
}
