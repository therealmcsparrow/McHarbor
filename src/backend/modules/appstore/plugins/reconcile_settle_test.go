// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"os"
	"path/filepath"
	"testing"

	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

const otherRelease = "2000.01.01"

func (f *upgradeFixture) row() (builtFor, repairErr string, enabled bool) {
	f.t.Helper()
	if err := f.inst.db.QueryRowContext(f.ctx,
		`SELECT built_for, repair_error, enabled FROM appstore_plugins WHERE slug = ?`, upgradeSlug).
		Scan(&builtFor, &repairErr, &enabled); err != nil {
		f.t.Fatal(err)
	}
	return builtFor, repairErr, enabled
}

// TestReconcileKeepsAPluginTheCatalogCannotReplace guards the first boot of a new
// release before the catalog is rebuilt: the plugin may well still load, so it is
// neither marked for repair nor switched off.
func TestReconcileKeepsAPluginTheCatalogCannotReplace(t *testing.T) {
	entry := `{"slug":"` + upgradeSlug + `","name":"Up","version":"2026.11.01","versions":[{"version":"2026.11.01","min_mcharbor":"9999.01.01","download_url":"URL"}]}`
	for name, catalogEntry := range map[string]string{
		"catalog does not list it":         "",
		"catalog requires a newer release": entry,
	} {
		f := newUpgradeFixture(t)
		f.exec(`UPDATE appstore_plugins SET built_for = ? WHERE slug = ?`, otherRelease, upgradeSlug)
		if catalogEntry != "" {
			if err := os.WriteFile(filepath.Join(f.catalog, upgradeSlug+".json"), []byte(catalogEntry), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		f.reconcile()
		builtFor, repairErr, enabled := f.row()
		if builtFor != otherRelease || repairErr != "" || !enabled {
			t.Errorf("%s: built_for=%q repair_error=%q enabled=%v, want the row untouched", name, builtFor, repairErr, enabled)
		}
		if n := f.downloads.Load(); n != 0 {
			t.Errorf("%s: reconcile downloaded %d artifacts", name, n)
		}
	}
}

// TestSettleRepairStateFollowsTheLoad: the badge follows plugin.Open. A plugin that
// loads loses a stale error and is switched back on; one that fails shows why.
func TestSettleRepairStateFollowsTheLoad(t *testing.T) {
	f := newUpgradeFixture(t)
	f.exec(`UPDATE appstore_plugins SET built_for = ?, repair_error = 'not in any catalog', enabled = 0 WHERE slug = ?`, otherRelease, upgradeSlug)

	f.h.SettleRepairState(f.ctx, map[string]string{}, nil)
	if _, repairErr, enabled := f.row(); repairErr != "not in any catalog" || enabled {
		t.Fatalf("a plugin the runtime did not attempt changed: %q, enabled=%v", repairErr, enabled)
	}

	f.h.SettleRepairState(f.ctx, map[string]string{upgradeSlug: ""}, nil)
	if builtFor, repairErr, enabled := f.row(); repairErr != "" || !enabled || builtFor != otherRelease {
		t.Fatalf("after a good load: built_for=%q repair_error=%q enabled=%v", builtFor, repairErr, enabled)
	}

	const failure = "plugin was built with a different version of package core/agent"
	f.h.SettleRepairState(f.ctx, map[string]string{upgradeSlug: failure}, nil)
	if _, repairErr, _ := f.row(); repairErr != failure {
		t.Fatalf("after a failed load: repair_error=%q", repairErr)
	}
}

// TestReconcileLeavesAPluginAloneWhenOnlyTheReleaseMoved is the reason min_mcharbor
// exists: a new host release with an unchanged catalog downloads nothing, and records
// the release so a version the catalog drops later is not read as a release upgrade.
func TestReconcileLeavesAPluginAloneWhenOnlyTheReleaseMoved(t *testing.T) {
	f := newUpgradeFixture(t)
	f.publish("2026.10.01", "", func(int64) []byte { return tarballWith(t, "2026.10.01", "", nil) })
	f.exec(`UPDATE appstore_plugins SET built_for = ?, repair_error = 'kept' WHERE slug = ?`, otherRelease, upgradeSlug)

	f.reconcile()

	if n := f.downloads.Load(); n != 0 {
		t.Fatalf("reconcile downloaded %d artifacts on a release change alone", n)
	}
	builtFor, repairErr, enabled := f.row()
	if builtFor != appversion.Current() || repairErr != "kept" || !enabled {
		t.Fatalf("built_for=%q repair_error=%q enabled=%v", builtFor, repairErr, enabled)
	}
}

// TestReconcileStillAppliesANewCatalogBuild keeps the delivery path for a real ABI
// change: a rebuild at the same version reaches the host without any release moving.
func TestReconcileStillAppliesANewCatalogBuild(t *testing.T) {
	f := newUpgradeFixture(t)
	f.publish("2026.10.01", "202611061200", func(int64) []byte { return tarballWith(t, "2026.10.01", "", nil) })

	f.reconcile()

	if n := f.downloads.Load(); n != 1 {
		t.Fatalf("reconcile downloaded %d artifacts, want the new build once", n)
	}
}

// TestReconcileFollowsAReleaseWhenTheCatalogDroppedTheVersion keeps a plugin bump
// moving with the host; without a release change the same catalog must not move it.
func TestReconcileFollowsAReleaseWhenTheCatalogDroppedTheVersion(t *testing.T) {
	f := newUpgradeFixture(t)
	f.publish("2026.10.02", "202611061200", func(int64) []byte { return tarballWith(t, "2026.10.02", "", nil) })

	f.reconcile()
	if n := f.downloads.Load(); n != 0 {
		t.Fatalf("same release: reconcile downloaded %d artifacts", n)
	}

	f.exec(`UPDATE appstore_plugins SET built_for = ? WHERE slug = ?`, otherRelease, upgradeSlug)
	f.reconcile()
	f.assertInstalled("2026.10.02")
}
