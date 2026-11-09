// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func (f *swFixture) setSaved(value string) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO settings (id, key, value, category) VALUES ('s-dir', 'backup_temp_dir', ?, 'backups')`, value); err != nil {
		f.t.Fatal(err)
	}
}

// TestResolveTempRootRejectsAStoredRootThatFailsRevalidation: a row written outside the dedicated route is not trusted.
func TestResolveTempRootRejectsAStoredRootThatFailsRevalidation(t *testing.T) {
	f := newSwFixture(t)
	fallback := defaultTempRoot(f.dataDir)
	cases := map[string]string{
		"relative":            filepath.Join("rel", "dir"),
		"dotdot":              filepath.Join(f.root, "a") + string(filepath.Separator) + ".." + string(filepath.Separator) + "b",
		"ancestor of data":    filepath.Dir(f.dataDir),
		"the data dir itself": f.dataDir,
	}
	for name, saved := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.db.Exec(`DELETE FROM settings WHERE key = 'backup_temp_dir'`); err != nil {
				t.Fatal(err)
			}
			f.setSaved(saved)
			root, source, _ := resolveTempRoot(context.Background(), f.db, f.dataDir, noEnv)
			if root != fallback || source != tempSourceDefault {
				t.Fatalf("resolveTempRoot = %q (%s), want the default %q", root, source, fallback)
			}
		})
	}
}

// TestResolveTempRootKeepsAValidStoredRoot guards the other side: re-validation must not reject a good row.
func TestResolveTempRootKeepsAValidStoredRoot(t *testing.T) {
	f := newSwFixture(t)
	f.setSaved(f.root)
	root, source, _ := resolveTempRoot(context.Background(), f.db, f.dataDir, noEnv)
	if root != filepath.Clean(f.root) || source != tempSourceSetting {
		t.Fatalf("resolveTempRoot = %q (%s), want %q from the setting", root, source, f.root)
	}
}

// TestSweepSkipsRetiredRootsThatFailRevalidation: a tampered retired list must not point the sweep at a protected folder.
func TestSweepSkipsRetiredRootsThatFailRevalidation(t *testing.T) {
	f := newSwFixture(t)
	protected := filepath.Dir(f.dataDir)
	victim := f.entry(protected, "mcharbor-run-victim", 5*time.Hour)
	f.setRetired(protected, filepath.Join("rel", "dir"))

	f.sweep()
	if !swExists(t, victim) {
		t.Fatal("sweep deleted an entry inside a retired root that fails re-validation")
	}

	f.cleanup()
	if !swExists(t, victim) {
		t.Fatal("cleanup deleted an entry inside a retired root that fails re-validation")
	}
}

func TestSafeRetiredRootsFiltersAndCleans(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	good := filepath.Join(t.TempDir(), "old")
	got := safeRetiredRoots([]string{good + string(filepath.Separator), "rel", filepath.Dir(data)}, data)
	if len(got) != 1 || got[0] != good {
		t.Fatalf("safeRetiredRoots = %v, want only %q", got, good)
	}
}
