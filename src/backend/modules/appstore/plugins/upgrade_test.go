// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
)

const upgradeSlug = "comm-up"

// tarballWith packs a plugin whose plugin.yaml carries extra manifest lines,
// plus any further files (a migration, say).
func tarballWith(t *testing.T, version, extraManifest string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	all := map[string]string{"plugin.yaml": "schema: \"mcharbor.plugin/v1\"\nslug: \"" + upgradeSlug +
		"\"\nname: \"Up\"\nauthor: \"Test\"\nversion: \"" + version + "\"\nmcharbor: \">=2026.09.01\"\n" + extraManifest}
	for k, v := range files {
		all[k] = v
	}
	for name, body := range all {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type upgradeFixture struct {
	t         *testing.T
	ctx       context.Context
	inst      *DefaultInstaller
	svc       *Service
	h         *Handler
	dataDir   string
	catalog   string
	serve     atomic.Value // func(n int64) []byte
	downloads atomic.Int64
	url       string
}

// newUpgradeFixture installs upgradeSlug@2026.10.01, enabled, with one plugin
// key granted to a custom role — the grant an uninstall strips and only the
// snapshot can bring back.
func newUpgradeFixture(t *testing.T) *upgradeFixture {
	t.Helper()
	f := &upgradeFixture{t: t, ctx: context.Background()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/up.tar.gz" {
			http.NotFound(w, r)
			return
		}
		n := f.downloads.Add(1)
		body := f.serve.Load().(func(int64) []byte)(n)
		if _, err := w.Write(body); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	f.url = srv.URL + "/up.tar.gz"
	f.catalog = t.TempDir()

	inst, _, dataDir := unsignedInstaller(t, nil)
	f.inst, f.dataDir = inst, dataDir
	f.svc = NewService(inst.db, nil, f.catalog)
	inst.svc = f.svc
	f.h = NewHandler(inst, f.svc, nil)

	if _, err := inst.Install(f.ctx, Source{Tarball: tarballWith(t, "2026.10.01", "", nil), AllowUnsigned: true}, nil); err != nil {
		t.Fatalf("installing the starting version: %v", err)
	}
	f.exec(`INSERT INTO plugin_permissions (origin, permission_key, description, scope) VALUES (?, 'up.manage', '', 'global')`, upgradeSlug)
	f.exec(`INSERT INTO roles (id, name, permissions) VALUES ('role_custom_up', 'Custom up', '["up.manage"]')`)
	return f
}

func (f *upgradeFixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.inst.db.ExecContext(f.ctx, q, args...); err != nil {
		f.t.Fatal(err)
	}
}

// publish puts one catalog entry for upgradeSlug at version/build, served by
// serve (called with the 1-based download count).
func (f *upgradeFixture) publish(version, build string, serve func(int64) []byte) {
	f.t.Helper()
	f.serve.Store(serve)
	entry := `{"slug":"` + upgradeSlug + `","name":"Up","version":"` + version + `","build":"` + build +
		`","download_url":"` + f.url + `","versions":[{"version":"` + version + `","build":"` + build +
		`","download_url":"` + f.url + `"}]}`
	if err := os.WriteFile(filepath.Join(f.catalog, upgradeSlug+".json"), []byte(entry), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *upgradeFixture) update() string {
	r := chi.NewRouter()
	r.Post("/appstore/plugins/{slug}/update/stream", f.h.handleUpdateStream)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/appstore/plugins/"+upgradeSlug+"/update/stream",
		strings.NewReader(`{"allow_unsigned":true}`)))
	return w.Body.String()
}

func (f *upgradeFixture) reconcile() {
	f.h.Reconcile(f.ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// assertInstalled checks the plugin is at version, enabled, on disk, and that
// the custom role still grants its key; snapshot and rollback copy are gone.
func (f *upgradeFixture) assertInstalled(version string) {
	f.t.Helper()
	prev, enabled, ok, err := f.svc.installedState(f.ctx, upgradeSlug)
	if err != nil || !ok || prev.Version != version || !enabled {
		f.t.Fatalf("installed state: ok=%v version=%q enabled=%v err=%v, want %s enabled", ok, prev.Version, enabled, err, version)
	}
	if _, err := os.Stat(filepath.Join(f.dataDir, "plugins", upgradeSlug+"@"+version, "plugin.yaml")); err != nil {
		f.t.Fatalf("install directory missing: %v", err)
	}
	grants, err := f.svc.RoleGrantsForKeys(f.ctx, []string{"up.manage"})
	if err != nil || len(grants["role_custom_up"]) != 1 {
		f.t.Fatalf("custom role lost its grant: %v %v", grants, err)
	}
	if snap, err := f.svc.UpdateSnapshotFor(f.ctx, upgradeSlug); err != nil || snap != nil {
		f.t.Fatalf("snapshot left behind: %+v %v", snap, err)
	}
	if f.inst.HasPrevious(upgradeSlug, "2026.10.01") {
		f.t.Fatal("rollback copy left behind")
	}
}

func (f *upgradeFixture) repairError() string {
	f.t.Helper()
	var reason string
	if err := f.inst.db.QueryRowContext(f.ctx, `SELECT repair_error FROM appstore_plugins WHERE slug = ?`, upgradeSlug).Scan(&reason); err != nil {
		f.t.Fatal(err)
	}
	return reason
}

const unknownScope = "permissions:\n  - \"agent.rpc.not-a-token\"\n"

// TestUpdateToAnUnknownScopeLeavesThePluginInstalled guards TOFIX-02's incident:
// the Store Update refuses an artifact the host's manifest check rejects before
// anything is uninstalled, and says why with the manifest code.
func TestUpdateToAnUnknownScopeLeavesThePluginInstalled(t *testing.T) {
	f := newUpgradeFixture(t)
	f.publish("2026.10.02", "202610010900", func(int64) []byte { return tarballWith(t, "2026.10.02", unknownScope, nil) })

	body := f.update()
	if !strings.Contains(body, `"error_code":"err.plugin.manifest_invalid"`) {
		t.Fatalf("update stream did not report the manifest refusal:\n%s", body)
	}
	if strings.Contains(body, `"phase":"step1"`) {
		t.Fatalf("the refused update started an uninstall:\n%s", body)
	}
	f.assertInstalled("2026.10.01")
}

// TestRepairToAnUnknownScopeLeavesThePluginInstalled is the boot-repair half:
// a rebuilt catalog artifact the host refuses keeps the installed row as it was.
func TestRepairToAnUnknownScopeLeavesThePluginInstalled(t *testing.T) {
	f := newUpgradeFixture(t)
	f.publish("2026.10.01", "202610010900", func(int64) []byte { return tarballWith(t, "2026.10.01", unknownScope, nil) })

	f.reconcile()
	f.assertInstalled("2026.10.01")
	if reason := f.repairError(); reason != "" {
		t.Fatalf("a refused repair touched the row: repair_error=%q", reason)
	}
}

// TestRepairWhoseInstallFailsRollsBack guards the rollback: the uninstall has
// already happened when the new install's migration fails, and the previous
// version must come back enabled with the custom role's grant.
func TestRepairWhoseInstallFailsRollsBack(t *testing.T) {
	f := newUpgradeFixture(t)
	f.publish("2026.10.01", "202610010900", func(int64) []byte {
		return tarballWith(t, "2026.10.01", "", map[string]string{"migrations/0001_broken.sql": "THIS IS NOT SQL;"})
	})

	f.reconcile()
	f.assertInstalled("2026.10.01")
	if reason := f.repairError(); !strings.Contains(reason, "restored") {
		t.Fatalf("a rolled-back repair should say so: repair_error=%q", reason)
	}
}

// TestInterruptedUpgradeRestoresGrantsOnBoot simulates a crash between the
// uninstall and the install: the snapshot is persisted and the plugin set
// aside, then nothing else runs. The next boot pass must put it all back.
func TestInterruptedUpgradeRestoresGrantsOnBoot(t *testing.T) {
	f := newUpgradeFixture(t)
	prev, enabled, _, err := f.svc.installedState(f.ctx, upgradeSlug)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := f.h.snapshotFor(f.ctx, prev, enabled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SaveUpdateSnapshot(f.ctx, snap); err != nil {
		t.Fatal(err)
	}
	if _, err := f.inst.Uninstall(f.ctx, upgradeSlug, UninstallOptions{KeepTables: true, SetAside: true}, nil); err != nil {
		t.Fatal(err)
	}

	f.reconcile()
	f.assertInstalled("2026.10.01")
}

// TestInterruptedUpgradeWithNoCopyRestoresGrantsOnReinstall covers the case
// boot cannot finish: the set-aside copy is gone, so the snapshot waits for the
// plugin to be installed again and restores the custom grant then.
func TestInterruptedUpgradeWithNoCopyRestoresGrantsOnReinstall(t *testing.T) {
	f := newUpgradeFixture(t)
	prev, enabled, _, err := f.svc.installedState(f.ctx, upgradeSlug)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := f.h.snapshotFor(f.ctx, prev, enabled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SaveUpdateSnapshot(f.ctx, snap); err != nil {
		t.Fatal(err)
	}
	if _, err := f.inst.Uninstall(f.ctx, upgradeSlug, UninstallOptions{KeepTables: true}, nil); err != nil {
		t.Fatal(err)
	}
	f.reconcile()
	if s, err := f.svc.UpdateSnapshotFor(f.ctx, upgradeSlug); err != nil || s == nil {
		t.Fatalf("with nothing to restore, the snapshot must be kept: %v", err)
	}

	f.publish("2026.10.01", "", func(int64) []byte { return tarballWith(t, "2026.10.01", "", nil) })
	f.update()
	f.assertInstalled("2026.10.01")
}

// TestUpdateInstallsTheValidatedBytes guards "no second download": the server
// hands out a valid artifact once and a broken one after, so an update that
// fetched again after preflight would install something it never validated.
func TestUpdateInstallsTheValidatedBytes(t *testing.T) {
	f := newUpgradeFixture(t)
	f.publish("2026.10.02", "202610010900", func(n int64) []byte {
		if n == 1 {
			return tarballWith(t, "2026.10.02", "", nil)
		}
		return tarballWith(t, "2026.10.02", unknownScope, nil)
	})

	body := f.update()
	if !strings.Contains(body, `"status":"done"`) {
		t.Fatalf("update did not finish:\n%s", body)
	}
	if n := f.downloads.Load(); n != 1 {
		t.Fatalf("the artifact was downloaded %d times, want 1", n)
	}
	f.assertInstalled("2026.10.02")
	if _, err := os.Stat(filepath.Join(f.dataDir, "plugins", upgradeSlug+"@2026.10.01")); !os.IsNotExist(err) {
		t.Fatalf("the previous version's directory is still under plugins/: %v", err)
	}
}
