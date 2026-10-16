// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

// pluginsDir is where the three compute plugins' migrations live, relative to
// this package.
const pluginsDir = "../../../../plugins/Plugins"

// codeSideColumns mirrors the ALTERs the plugins run from Init rather than
// from a numbered migration, so the test schema matches what SeedDB meets.
var codeSideColumns = []string{
	"ALTER TABLE plugin_addon_kvm_vms ADD COLUMN current_snapshot_id TEXT",
	"ALTER TABLE plugin_addon_kvm_nics ADD COLUMN pci_addr TEXT",
	"ALTER TABLE plugin_addon_kvm_snapshots ADD COLUMN schedule_id TEXT",
}

func migratedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, plugin := range []string{"addon-compute", "addon-kvm", "addon-lxc"} {
		files, err := filepath.Glob(filepath.Join(pluginsDir, plugin, "migrations", "*.sql"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no migrations for %s (%v)", plugin, err)
		}
		sort.Strings(files)
		for _, file := range files {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(string(raw)); err != nil {
				t.Fatalf("%s: %v", filepath.Base(file), err)
			}
		}
	}
	for _, stmt := range codeSideColumns {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return db
}

func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

func TestSeedDBMatchesTheSimulatorAndIsIdempotent(t *testing.T) {
	db := migratedDB(t)
	e := estate.Build(estate.DefaultSeed, testNow)
	sim := New(e, estate.DefaultSeed, testNow)
	envIDs := map[string]string{"prod-eu-3": "env-not-compute"}
	for _, key := range sim.order {
		envIDs[key] = "env-" + key
	}

	ctx := context.Background()
	inserted, err := sim.seedRows(ctx, db, envIDs)
	if err != nil {
		t.Fatal(err)
	}
	vms, cts, pools, networks := sim.Counts()
	for table, want := range map[string]int{
		tblVMs: vms, tblCTs: cts, tblPools: pools, tblNetworks: networks, tblComputeEnvs: len(sim.order),
	} {
		if got := count(t, db, table); got != want {
			t.Fatalf("%s has %d rows, want %d", table, got, want)
		}
	}
	for _, table := range []string{tblVMDisks, tblVMNICs, tblVMSnapshots, tblCTNICs, tblCTSnapshots, tblLeases, tblImageCache} {
		if count(t, db, table) == 0 {
			t.Fatalf("%s is empty", table)
		}
	}

	var enabled, noDefaults int
	var caps string
	if err := db.QueryRow(`SELECT compute_enabled, capabilities FROM `+tblComputeEnvs+` WHERE environment_id = 'env-homelab'`).Scan(&enabled, &caps); err != nil {
		t.Fatal(err)
	}
	var capability struct {
		HostCPUs  int  `json:"hostCpus"`
		CanRunVMs bool `json:"canRunVms"`
	}
	if err := json.Unmarshal([]byte(caps), &capability); err != nil || enabled != 1 || capability.HostCPUs == 0 || !capability.CanRunVMs {
		t.Fatalf("homelab compute row: enabled %d, capability %+v, %v", enabled, capability, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + tblComputeEnvs + ` WHERE default_pool_id IS NULL OR default_network_id IS NULL`).Scan(&noDefaults); err != nil || noDefaults != 0 {
		t.Fatalf("%d compute rows without a default pool or network (%v)", noDefaults, err)
	}
	if n := count(t, db, tblComputeEnvs+` WHERE environment_id = 'env-not-compute'`); n != 0 {
		t.Fatal("a host without compute was seeded")
	}

	hs := sim.hosts["prod-eu-2"]
	for _, id := range hs.vmOrder {
		g := hs.vms[id]
		var state, current string
		if err := db.QueryRow(`SELECT state, COALESCE(current_snapshot_id, '') FROM `+tblVMs+` WHERE id = ?`, id).Scan(&state, &current); err != nil {
			t.Fatal(err)
		}
		if state != g.State || current != g.Current {
			t.Fatalf("%s row says %s/%q, simulator says %s/%q", g.Name, state, current, g.State, g.Current)
		}
	}

	again, err := sim.seedRows(ctx, db, envIDs)
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("a second seed wrote %d rows, want 0 (first wrote %d)", again, inserted)
	}
	if got := count(t, db, tblVMs); got != vms {
		t.Fatalf("after the second seed %d VM rows, want %d", got, vms)
	}
}

func TestSeedDBCompletesALazilyCreatedComputeRow(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.Exec(`INSERT INTO ` + tblComputeEnvs + ` (id, environment_id, compute_enabled, created_at, updated_at)
		VALUES ('lazy', 'env-dev', 0, '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	sim := New(estate.Build(estate.DefaultSeed, testNow), 1, testNow)
	if err := sim.SeedDB(context.Background(), db, map[string]string{"dev": "env-dev"}); err != nil {
		t.Fatal(err)
	}
	var enabled int
	var caps sql.NullString
	if err := db.QueryRow(`SELECT compute_enabled, capabilities FROM `+tblComputeEnvs+` WHERE environment_id = 'env-dev'`).Scan(&enabled, &caps); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 || !caps.Valid {
		t.Fatalf("lazy row after seed: enabled %d, capabilities set %v", enabled, caps.Valid)
	}
}
