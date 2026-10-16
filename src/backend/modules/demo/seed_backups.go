// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

// backupsSeededKey marks demo.db as carrying the protection story. The version is
// bumped only when the seed changes shape and has to be written again.
const (
	backupsSeededKey   = "demo.backups_seeded"
	backupSeedVersion  = "1"
	demoLocalLocation  = "default-local-backup"
	demoOffsiteName    = "Offsite NAS"
	demoOffsiteBaseDir = "/mnt/offsite"
)

// backupSeedTables are every table the seed writes. The plugin-owned ones are
// absent until their plugin has run its migrations, so a missing table skips that
// half rather than failing the whole demo boot.
var (
	containerBackupTables = []string{
		"storage_locations", "container_backup_plans", "container_backup_plan_storage_locations",
		"container_backup_runs", "container_backup_run_destinations",
	}
	kvmBackupTables = []string{
		"plugin_addon_kvm_vms", "plugin_addon_kvm_backup_plans",
		"plugin_addon_kvm_backup_plan_destinations", "plugin_addon_kvm_backup_runs",
		"plugin_addon_kvm_backup_run_destinations",
	}
	kvmScheduleTables = []string{"plugin_addon_kvm_vms", "plugin_addon_kvm_snapshot_schedules"}
	lxcBackupTables   = []string{
		"plugin_addon_lxc_containers", "plugin_addon_lxc_backup_plans",
		"plugin_addon_lxc_backup_plan_destinations", "plugin_addon_lxc_backup_runs",
	}
)

// backupSeedCounts sizes what one seed wrote.
type backupSeedCounts struct {
	Locations, ContainerPlans, ContainerRuns int
	VMPlans, VMRuns, VMSchedules             int
	CTPlans, CTRuns                          int
}

// backupRunShape is one run in a plan's history: how long ago it started, how it
// ended, and how big the archive was.
type backupRunShape struct {
	daysAgo   int
	status    string
	kind      string
	minutes   int
	gib       int
	failure   string
	operation string
}

// recentRuns is the history every plan gets, newest last. A plan whose every run
// succeeded teaches an operator nothing, so the shape carries one real failure
// and one restore — the two rows whose presentation is easiest to get wrong.
func recentRuns() []backupRunShape {
	return []backupRunShape{
		{daysAgo: 21, status: "success", kind: "full", minutes: 14, gib: 42, operation: "backup"},
		{daysAgo: 14, status: "success", kind: "incremental", minutes: 3, gib: 4, operation: "backup"},
		{daysAgo: 7, status: "failure", kind: "incremental", minutes: 1, gib: 0, operation: "backup",
			failure: "storage location \"Offsite NAS\" refused the upload: no space left on device (wrote 3.1 GiB of 4.4 GiB)"},
		{daysAgo: 6, status: "success", kind: "full", minutes: 16, gib: 44, operation: "backup"},
		{daysAgo: 3, status: "success", kind: "incremental", minutes: 4, gib: 5, operation: "backup"},
		{daysAgo: 2, status: "success", kind: "full", minutes: 12, gib: 39, operation: "restore"},
		{daysAgo: 1, status: "success", kind: "incremental", minutes: 3, gib: 4, operation: "backup"},
	}
}

// backupsSeeded reports whether demo.db already carries this version of the seed.
func backupsSeeded(ctx context.Context, conn *sql.DB) (bool, error) {
	ok, err := tableExists(ctx, conn, "settings")
	if err != nil || !ok {
		return false, err
	}
	var value string
	err = conn.QueryRowContext(ctx, "SELECT COALESCE(value, '') FROM settings WHERE key = ?", backupsSeededKey).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("reading %s: %w", backupsSeededKey, err)
	}
	return value == backupSeedVersion, nil
}

// seedBackups writes the protection story into demo.db once: storage locations,
// container backup plans and their run history, the same for KVM and LXC, and the
// snapshot schedules the existing snapshot rows were missing.
//
// It is skipped entirely on a database that already carries this version, so a
// plan the viewer disabled or a run they deleted survives a restart.
func seedBackups(ctx context.Context, conn *sql.DB, e *estate.Estate,
	envIDs map[string]string, now time.Time) (backupSeedCounts, error) {
	var counts backupSeedCounts
	if !coredemo.Active() {
		return counts, errNotDemo
	}
	seeded, err := backupsSeeded(ctx, conn)
	if err != nil || seeded {
		return counts, err
	}

	// Each half is independent. A plugin whose schema differs from what this seed
	// expects must not starve the others of their rows, and the version key is
	// only written when every half that could run did — so a boot that failed one
	// section retries it next time rather than declaring the estate finished.
	var failed []error
	section := func(tables []string, run func() error) {
		ok, err := allTablesExist(ctx, conn, tables...)
		if err != nil {
			failed = append(failed, err)
			return
		}
		if !ok {
			return
		}
		if err := run(); err != nil {
			failed = append(failed, err)
		}
	}

	section(containerBackupTables, func() error {
		n, err := seedStorageLocations(ctx, conn, now)
		counts.Locations += n
		if err != nil {
			return err
		}
		counts.ContainerPlans, counts.ContainerRuns, err = seedContainerBackups(ctx, conn, e, envIDs, now)
		return err
	})
	section(kvmBackupTables, func() (err error) {
		counts.VMPlans, counts.VMRuns, err = seedVMBackups(ctx, conn, now)
		return err
	})
	section(kvmScheduleTables, func() (err error) {
		counts.VMSchedules, err = seedVMSnapshotSchedules(ctx, conn, now)
		return err
	})
	section(lxcBackupTables, func() (err error) {
		counts.CTPlans, counts.CTRuns, err = seedCTBackups(ctx, conn, now)
		return err
	})

	if len(failed) > 0 {
		return counts, errors.Join(failed...)
	}

	if err := markSeeded(ctx, conn, backupsSeededKey, backupSeedVersion, now); err != nil {
		return counts, err
	}
	return counts, nil
}

// seedStorageLocations adds a second local destination, so a run can show more
// than one and the multi-destination presentation is exercised. Both are local
// paths on purpose: a credentialed location would invite a background job to
// attempt a real upload out of a demo.
func seedStorageLocations(ctx context.Context, conn *sql.DB, now time.Time) (int, error) {
	stamp := now.UTC().Format(time.RFC3339)
	id := stableID("storage-location", demoOffsiteName)

	var existing string
	err := conn.QueryRowContext(ctx, "SELECT id FROM storage_locations WHERE id = ?", id).Scan(&existing)
	if err == nil {
		return 0, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("reading storage location: %w", err)
	}
	_, err = conn.ExecContext(ctx, `
		INSERT INTO storage_locations (id, name, location_type, enabled, base_path, created_at, updated_at)
		VALUES (?, ?, 'local', 1, ?, ?, ?)`,
		id, demoOffsiteName, demoOffsiteBaseDir, stamp, stamp)
	if err != nil {
		return 0, fmt.Errorf("inserting storage location: %w", err)
	}
	return 1, nil
}
