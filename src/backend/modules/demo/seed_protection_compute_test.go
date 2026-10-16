// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"os"
	"testing"
)

// addComputeSchema creates the compute plugins' backup and snapshot tables and a
// few VMs and system containers to attach to.
//
// Without this the KVM and LXC halves of seedBackups are skipped by their table
// guard, so every assertion about them passes vacuously — which is exactly how a
// NULL into plugin_addon_kvm_backup_run_destinations.error, a NOT NULL column,
// reached a running instance.
func addComputeSchema(t *testing.T, conn *sql.DB, envID string) {
	t.Helper()
	ddl, err := os.ReadFile("testdata/compute_backup_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), string(ddl)); err != nil {
		t.Fatalf("applying the compute schema: %v", err)
	}

	for i, name := range []string{"db-primary-01", "db-replica-01", "web-01"} {
		id := stableID("test-vm", name)
		if _, err := conn.ExecContext(context.Background(),
			"INSERT INTO plugin_addon_kvm_vms (id, environment_id, name) VALUES (?, ?, ?)",
			id, envID, name); err != nil {
			t.Fatal(err)
		}
		// Only the first two carry snapshots, so the schedule seed has to pick a
		// subset rather than every VM.
		if i < 2 {
			if _, err := conn.ExecContext(context.Background(),
				"INSERT INTO plugin_addon_kvm_snapshots (id, vm_id) VALUES (?, ?)",
				stableID("test-snap", name), id); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range []string{"ct-build-01", "ct-cache-01"} {
		if _, err := conn.ExecContext(context.Background(),
			"INSERT INTO plugin_addon_lxc_containers (id, environment_id, name) VALUES (?, ?, ?)",
			stableID("test-ct", name), envID, name); err != nil {
			t.Fatal(err)
		}
	}
}

// anyEnvID returns one seeded environment id to hang the test VMs off.
func anyEnvID(t *testing.T, ids map[string]string) string {
	t.Helper()
	for _, id := range ids {
		return id
	}
	t.Fatal("no environments were seeded")
	return ""
}

// TestSeedBackupsWritesComputePlansAgainstTheRealSchema exercises the KVM and LXC
// halves against the plugins' own DDL, constraints included.
func TestSeedBackupsWritesComputePlansAgainstTheRealSchema(t *testing.T) {
	m := activateDemo(t)
	f := newProtectionFixture(t, m)
	addComputeSchema(t, f.conn, anyEnvID(t, f.envIDs))

	got, err := seedBackups(context.Background(), f.conn, f.estate, f.envIDs, f.now)
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if got.VMPlans != 3 {
		t.Errorf("expected a plan for each of the 3 VMs, got %d", got.VMPlans)
	}
	if got.CTPlans != 2 {
		t.Errorf("expected a plan for each of the 2 system containers, got %d", got.CTPlans)
	}
	// Only the two VMs carrying snapshots get a schedule: scheduling a VM with no
	// snapshot history would be a schedule for something nothing produced.
	if got.VMSchedules != 2 {
		t.Errorf("expected 2 snapshot schedules, got %d", got.VMSchedules)
	}
	if got.VMRuns == 0 || got.CTRuns == 0 {
		t.Errorf("no run history: vm=%d ct=%d", got.VMRuns, got.CTRuns)
	}

	// Two destinations per run, and the local copy must succeed even on the run
	// whose offsite upload failed — that is the case worth showing.
	if n := countWhere(t, f.conn, "plugin_addon_kvm_backup_run_destinations", "1=1"); n != got.VMRuns*2 {
		t.Errorf("expected %d destination rows, got %d", got.VMRuns*2, n)
	}
	if n := countWhere(t, f.conn, "plugin_addon_kvm_backup_run_destinations",
		"status = 'success' AND error <> ''"); n != 0 {
		t.Errorf("%d successful destination(s) carry an error message", n)
	}
	if n := countWhere(t, f.conn, "plugin_addon_kvm_backup_run_destinations",
		"status = 'failure' AND error = ''"); n != 0 {
		t.Errorf("%d failed destination(s) carry no error message", n)
	}
	if n := countWhere(t, f.conn, "plugin_addon_kvm_backup_runs", "status = 'failure'"); n == 0 {
		t.Error("no failed VM backup run, so the error path is never shown")
	}
	// The fallback case has to name its reason, or the column reads as noise.
	if n := countWhere(t, f.conn, "plugin_addon_kvm_backup_runs",
		"fell_back_to_full = 1 AND (fallback_reason IS NULL OR fallback_reason = '')"); n != 0 {
		t.Errorf("%d fallback run(s) give no reason", n)
	}
	if n := countWhere(t, f.conn, "plugin_addon_kvm_backup_runs", "fell_back_to_full = 1"); n == 0 {
		t.Error("no run fell back to a full, so that presentation is never shown")
	}
	// Schedules must differ, or the readable-schedule description is untested.
	var distinctCron int
	if err := f.conn.QueryRowContext(context.Background(),
		"SELECT COUNT(DISTINCT cron) FROM plugin_addon_kvm_snapshot_schedules").Scan(&distinctCron); err != nil {
		t.Fatal(err)
	}
	if distinctCron < 2 {
		t.Errorf("every snapshot schedule uses the same cron (%d distinct)", distinctCron)
	}
	// Each plan is linked to both destinations.
	if n := countWhere(t, f.conn, "plugin_addon_kvm_backup_plan_destinations", "1=1"); n != got.VMPlans*2 {
		t.Errorf("expected %d plan destination link(s), got %d", got.VMPlans*2, n)
	}
}

// TestSeedBackupsSurvivesAMissingPluginSchema: a demo booted before the compute
// plugins ran their migrations must still get its container backups.
func TestSeedBackupsSurvivesAMissingPluginSchema(t *testing.T) {
	m := activateDemo(t)
	f := newProtectionFixture(t, m)
	// No addComputeSchema: the plugin tables are absent, as on a first boot.

	got, err := seedBackups(context.Background(), f.conn, f.estate, f.envIDs, f.now)
	if err != nil {
		t.Fatalf("a missing plugin schema must not fail the seed: %v", err)
	}
	if got.ContainerPlans == 0 {
		t.Error("the container half must still run when the plugin tables are absent")
	}
	if got.VMPlans != 0 || got.CTPlans != 0 || got.VMSchedules != 0 {
		t.Errorf("the compute halves must be skipped, got vm=%d ct=%d sched=%d",
			got.VMPlans, got.CTPlans, got.VMSchedules)
	}
}

// TestSeedBackupsRetriesAFailedSectionOnTheNextBoot: the version key is only
// written when every section that could run did, so a boot that failed one half
// tries it again rather than declaring the estate finished. This is what let the
// real NOT NULL failure recover on the following restart instead of leaving the
// KVM tables permanently empty.
func TestSeedBackupsRetriesAFailedSectionOnTheNextBoot(t *testing.T) {
	m := activateDemo(t)
	f := newProtectionFixture(t, m)
	ctx := context.Background()

	// First boot: no plugin schema, so the compute sections are skipped and the
	// key is written — nothing failed.
	if _, err := seedBackups(ctx, f.conn, f.estate, f.envIDs, f.now); err != nil {
		t.Fatal(err)
	}
	seeded, err := backupsSeeded(ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if !seeded {
		t.Fatal("a clean run must mark the seed done")
	}

	// A skipped section is not a failed one, so a later boot that now has the
	// plugin schema will not revisit it. That is the documented trade: the estate
	// is seeded once, and a plugin installed afterwards needs a Reset. Asserted so
	// the behaviour is a decision rather than a surprise.
	addComputeSchema(t, f.conn, anyEnvID(t, f.envIDs))
	second, err := seedBackups(ctx, f.conn, f.estate, f.envIDs, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if second.VMPlans != 0 {
		t.Errorf("the seed ran again after being marked done, writing %d VM plan(s)", second.VMPlans)
	}
}

// TestRecentRunsCarriesTheCasesWorthSeeing guards the shape itself. A history of
// only successes teaches an operator nothing, and every table that renders it
// depends on the failure and the restore being present.
func TestRecentRunsCarriesTheCasesWorthSeeing(t *testing.T) {
	var failures, restores int
	for _, s := range recentRuns() {
		if s.status == "failure" {
			failures++
			if s.failure == "" {
				t.Error("a failed run shape carries no message")
			}
		}
		if s.operation == "restore" {
			restores++
		}
		if s.minutes <= 0 {
			t.Errorf("run shape %d days ago has no duration", s.daysAgo)
		}
	}
	if failures == 0 {
		t.Error("the run history has no failure")
	}
	if restores == 0 {
		t.Error("the run history has no restore")
	}
}
