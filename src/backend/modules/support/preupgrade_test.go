// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	coreinstancebackup "github.com/therealmcsparrow/mcharbor/core/instancebackup"
)

// TestRangeAllows pins the manifest range grammar against the calendar notation, where a
// leading zero must still compare numerically.
func TestRangeAllows(t *testing.T) {
	cases := []struct {
		rng, v         string
		allowed, known bool
	}{
		{">=2026.09.01", "2026.09.02", true, true},
		{">=2026.09.01", "2026.08.30", false, true},
		{">=2.1.0 <3.0.0", "2.5.0", true, true},
		{">=2.1.0 <3.0.0", "3.0.0", false, true},
		{">=2.1.0<3.0.0", "2026.09.01", false, true},
		{"^2026.9.0", "2026.10.01", true, true},
		{"~2026.09.00", "2026.10.01", false, true},
		{"2026.09.01", "2026.09.01", true, true},
		{"", "2026.09.01", true, true},
		{"latest", "2026.09.01", false, false},
		{">=2026.09.01", "not-a-version", false, false},
	}
	for _, c := range cases {
		allowed, known := RangeAllows(c.rng, c.v)
		if allowed != c.allowed || known != c.known {
			t.Errorf("RangeAllows(%q, %q) = (%v, %v), want (%v, %v)", c.rng, c.v, allowed, known, c.allowed, c.known)
		}
	}
}

// TestPreUpgradeFlagsIncompatiblePlugins is the point of the check: an admin learns before
// the restart which installed plugin refuses the target version.
func TestPreUpgradeFlagsIncompatiblePlugins(t *testing.T) {
	svc := seededService(t)
	mustExec(t, svc.d.DB, `INSERT INTO appstore_plugins (id, slug, name, version, enabled)
		VALUES ('p1', 'addon-old', 'Old', '1.0.0', 1), ('p2', 'addon-new', 'New', '2026.09.01', 1)`)
	writeManifest(t, svc.d.DataDir, "addon-old@1.0.0", "slug: addon-old\nmcharbor: \">=2.0.0 <3.0.0\"\n")
	writeManifest(t, svc.d.DataDir, "addon-new@2026.09.01", "slug: addon-new\nmcharbor: \">=2026.09.01\"\n")
	rep, err := svc.PreUpgrade(context.Background(), "v2026.10.01")
	if err != nil {
		t.Fatal(err)
	}
	if rep.TargetVersion != "2026.10.01" {
		t.Errorf("target = %q", rep.TargetVersion)
	}
	verdict := map[string]string{}
	for _, p := range rep.Plugins {
		verdict[p.Slug] = p.Compatible
	}
	if verdict["addon-old"] != CompatNo || verdict["addon-new"] != CompatYes {
		t.Errorf("plugin verdicts = %v", verdict)
	}
	var compat CheckResult
	for _, c := range rep.Checks {
		if c.ID == CheckPluginCompat {
			compat = c
		}
	}
	if compat.Status != StatusWarn || compat.Reason != "incompatible" {
		t.Errorf("plugin check = %+v, want warn/incompatible", compat)
	}
	if _, err := svc.PreUpgrade(context.Background(), "../../etc"); !errors.Is(err, ErrInvalidVersion) {
		t.Errorf("an invalid target was accepted: %v", err)
	}
}

func writeManifest(t *testing.T, dataDir, dir, body string) {
	t.Helper()
	p := filepath.Join(dataDir, "plugins", dir)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "plugin.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEveryInstanceBackupRootHasAnInventoryCategory keeps the state inventory honest when the
// instance backup starts carrying a new DATA_DIR root: it must be listed, as included.
func TestEveryInstanceBackupRootHasAnInventoryCategory(t *testing.T) {
	if len(coreinstancebackup.FileRoots) < 3 {
		t.Fatalf("only %d file roots; refusing an empty scan", len(coreinstancebackup.FileRoots))
	}
	for _, root := range coreinstancebackup.FileRoots {
		found := false
		for _, c := range stateCategories {
			if c.path == root {
				found = true
				if backupFor(c) != BackupIncluded {
					t.Errorf("%s is exported by the instance backup but the inventory says %s", root, backupFor(c))
				}
			}
		}
		if !found {
			t.Errorf("instance backup root %q has no inventory category", root)
		}
	}
}

// TestInventoryMeasuresAndFlagsUnknownEntries checks the sizes add up and that something
// McHarbor did not create is shown as "other" rather than hidden.
func TestInventoryMeasuresAndFlagsUnknownEntries(t *testing.T) {
	svc := seededService(t)
	dir := svc.d.DataDir
	if err := os.MkdirAll(filepath.Join(dir, "stacks", "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stacks", "web", "compose.yml"), make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stray.bin"), make([]byte, 7), 0o644); err != nil {
		t.Fatal(err)
	}
	inv, err := svc.StateInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]InventoryEntry{}
	for _, e := range inv.Entries {
		byPath[e.Path] = e
	}
	if e := byPath["stacks"]; e.SizeBytes != 100 || e.Files != 1 || !e.Writable || e.Backup != BackupIncluded {
		t.Errorf("stacks = %+v", e)
	}
	if e := byPath["stray.bin"]; e.Category != "other" || e.Cleanup != CleanupUnknown {
		t.Errorf("stray entry = %+v", e)
	}
	if e := byPath["git"]; e.Exists || e.Backup != BackupExcluded {
		t.Errorf("missing git dir = %+v", e)
	}
}

// TestTLSVerdict pins the expiry thresholds the diagnostics warn and fail on.
func TestTLSVerdict(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		after  time.Duration
		status string
	}{{-time.Hour, StatusFail}, {10 * 24 * time.Hour, StatusWarn}, {90 * 24 * time.Hour, StatusPass}} {
		if got := tlsVerdict(now.Add(c.after), false, now); got.Status != c.status {
			t.Errorf("expiry in %v: %s, want %s", c.after, got.Status, c.status)
		}
	}
}

// TestEncryptionKeyFromFileWarns is the auto-generated key warning: a key that lives only in
// DATA_DIR is lost with the volume and differs on every HA node.
func TestEncryptionKeyFromFileWarns(t *testing.T) {
	svc := seededService(t)
	svc.d.EncryptionKeySet = false
	if err := os.WriteFile(filepath.Join(svc.d.DataDir, ".encryption_key"), make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	res := svc.checkEncryptionKey()
	if res.Status != StatusWarn || res.Reason != "auto_generated" {
		t.Errorf("file key = %+v, want warn/auto_generated", res)
	}
	if ks := svc.EncryptionKeySource(); ks.Source != KeySourceFile || !ks.FileSeen {
		t.Errorf("key source = %+v", ks)
	}
	svc.d.EncryptionKeySet = true
	if res := svc.checkEncryptionKey(); res.Status != StatusPass {
		t.Errorf("env key = %+v, want pass", res)
	}
}

// TestMigrationCheckIgnoresSquashedNames keeps a database upgraded across the migration squash
// from being told it was written by a newer McHarbor, while a genuinely unknown name still is.
func TestMigrationCheckIgnoresSquashedNames(t *testing.T) {
	svc := seededService(t)
	ctx := context.Background()
	retired := coredb.RetiredMigrationNames()
	mustExec(t, svc.d.DB, `INSERT INTO schema_migrations (name) VALUES (?)`, retired[len(retired)-1])
	if res := svc.checkMigrations(ctx); res.Status != StatusPass {
		t.Fatalf("a retired name was reported: %+v", res)
	}
	mustExec(t, svc.d.DB, `INSERT INTO schema_migrations (name) VALUES (?)`, "999_from_the_future.sql")
	res := svc.checkMigrations(ctx)
	if res.Reason != "ahead" || res.Details["names"] != "999_from_the_future.sql" {
		t.Errorf("check = %+v, want ahead naming only the unknown file", res)
	}
}
