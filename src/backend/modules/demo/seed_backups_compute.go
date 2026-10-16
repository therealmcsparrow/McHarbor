// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// computeSubject is one VM or system container the protection seed attaches to.
type computeSubject struct {
	id, name, envID string
}

// pickComputeSubjects reads the rows the compute sim already wrote, so the plans
// point at machines that exist. The seed owns no VMs of its own: inventing ids
// would produce plans whose subject the page cannot resolve.
func pickComputeSubjects(ctx context.Context, conn *sql.DB, table string, limit int) ([]computeSubject, error) {
	query := fmt.Sprintf("SELECT id, name, environment_id FROM %s ORDER BY name LIMIT ?", table) // SQL identifier: table is a plugin table-name literal at each call site
	rows, err := conn.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", table, err)
	}
	defer func() { logCloseErr(rows.Close()) }()

	var out []computeSubject
	for rows.Next() {
		var s computeSubject
		if err := rows.Scan(&s.id, &s.name, &s.envID); err != nil {
			return nil, fmt.Errorf("scanning %s: %w", table, err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// seedVMBackups gives a slice of the demo's VMs a backup plan with destinations
// and a run history, including one incremental that fell back to a full — the
// case whose reason column is easy to leave empty.
func seedVMBackups(ctx context.Context, conn *sql.DB, now time.Time) (plans, runs int, err error) {
	stamp := now.UTC().Format(time.RFC3339)
	subjects, err := pickComputeSubjects(ctx, conn, "plugin_addon_kvm_vms", 18)
	if err != nil {
		return 0, 0, err
	}
	offsite := stableID("storage-location", demoOffsiteName)

	// bounded by the fixed demo estate; the seed is marked done only after it completes
	for i, vm := range subjects {
		planID := stableID("kvm-backup-plan", vm.id)
		enabled := 1
		if i%6 == 4 {
			enabled = 0
		}
		_, err = conn.ExecContext(ctx, `
			INSERT INTO plugin_addon_kvm_backup_plans (
				id, vm_id, name, cron, enabled, retention_count, retention_days,
				full_every, quiesce, last_run_at, next_run_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO NOTHING`,
			planID, vm.id, "Nightly · "+vm.name, backupCron(i), enabled, 7, 30, 7,
			boolToInt(i%2 == 0),
			now.AddDate(0, 0, -1).UTC().Format(time.RFC3339),
			now.AddDate(0, 0, 1).UTC().Format(time.RFC3339), stamp, stamp)
		if err != nil {
			return plans, runs, fmt.Errorf("inserting kvm backup plan: %w", err)
		}
		plans++

		// bounded by the fixed demo estate; the seed is marked done only after it completes
		for _, locID := range []string{demoLocalLocation, offsite} {
			if _, err = conn.ExecContext(ctx, `
				INSERT INTO plugin_addon_kvm_backup_plan_destinations (plan_id, storage_location_id, created_at)
				VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, planID, locID, stamp); err != nil {
				return plans, runs, fmt.Errorf("linking kvm plan destination: %w", err)
			}
		}

		chainID := stableID("kvm-backup-chain", vm.id)
		// bounded by the fixed demo estate; the seed is marked done only after it completes
		for _, shape := range recentRuns() {
			runID := stableID("kvm-backup-run", fmt.Sprintf("%s/%d/%s", planID, shape.daysAgo, shape.operation))
			started := now.AddDate(0, 0, -shape.daysAgo)
			var completedAt any = started.Add(time.Duration(shape.minutes) * time.Minute).UTC().Format(time.RFC3339)
			var failure, fallbackReason any
			fellBack := 0
			if shape.failure != "" {
				failure = shape.failure
			}
			// One incremental in the history fell back, because the chain's base
			// was pruned by retention. A fallback with no reason is unreadable.
			if shape.daysAgo == 3 {
				fellBack = 1
				fallbackReason = "the chain's base backup was outside the 30-day retention window, so an incremental had no parent"
			}
			_, err = conn.ExecContext(ctx, `
				INSERT INTO plugin_addon_kvm_backup_runs (
					id, plan_id, vm_id, environment_id, operation, status, kind, chain_id,
					fell_back_to_full, fallback_reason, consistency, bytes_total, included_disks,
					error, progress_stage, progress_message, progress_updated_at,
					started_at, completed_at, duration_ms, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO NOTHING`,
				runID, planID, vm.id, vm.envID, shape.operation, shape.status, shape.kind, chainID,
				fellBack, fallbackReason, consistencyFor(i), int64(shape.gib)<<30, `["vda"]`,
				failure, progressStageFor(shape.status), completedAt,
				started.UTC().Format(time.RFC3339), completedAt,
				int64(shape.minutes)*60_000, stamp, stamp)
			if err != nil {
				return plans, runs, fmt.Errorf("inserting kvm backup run: %w", err)
			}
			runs++

			// bounded by the fixed demo estate; the seed is marked done only after it completes
			for _, locID := range []string{demoLocalLocation, offsite} {
				status := shape.status
				if locID == demoLocalLocation && status == "failure" {
					status = "success"
				}
				var uploadedAt any
				if status == "success" {
					uploadedAt = stamp
				}
				// error is NOT NULL DEFAULT '' on this table, so a successful
				// destination carries the empty string rather than NULL.
				destErr := ""
				if status == "failure" {
					destErr = shape.failure
				}
				if _, err = conn.ExecContext(ctx, `
					INSERT INTO plugin_addon_kvm_backup_run_destinations (
						id, run_id, storage_location_id, storage_location_name, disk_id, status,
						remote_path, key_id, bytes_uploaded, error, uploaded_at, created_at, updated_at
					) VALUES (?, ?, ?, ?, 'vda', ?, ?, 'demo-key-1', ?, ?, ?, ?, ?)
					ON CONFLICT(id) DO NOTHING`,
					stableID("kvm-backup-dest", runID+"/"+locID), runID, locID,
					nameForLocation(locID), status, "/"+vm.name+"/vda.qcow2",
					int64(shape.gib)<<30, destErr, uploadedAt, stamp, stamp); err != nil {
					return plans, runs, fmt.Errorf("inserting kvm run destination: %w", err)
				}
			}
		}
	}
	return plans, runs, nil
}

// seedVMSnapshotSchedules gives the VMs that already carry snapshots a schedule.
// The snapshot rows were seeded by the compute sim; without a schedule the page
// shows a history that apparently nothing produced.
func seedVMSnapshotSchedules(ctx context.Context, conn *sql.DB, now time.Time) (int, error) {
	stamp := now.UTC().Format(time.RFC3339)
	vmIDs, err := snapshotVMIDs(ctx, conn)
	if err != nil {
		return 0, err
	}
	count := 0
	// bounded by the fixed demo estate; the seed is marked done only after it completes
	for i, vmID := range vmIDs {
		id := stableID("kvm-snapshot-schedule", vmID)
		// Hourly for a few, then six-hourly, then daily: the readable description
		// is the thing being demonstrated, so the shapes have to differ.
		cron := "0 */6 * * *"
		name := "Six-hourly"
		switch i % 5 {
		case 0:
			cron, name = "0 * * * *", "Hourly"
		case 1:
			cron, name = "30 2 * * *", "Nightly"
		}
		var lastErr any
		if i%9 == 5 {
			lastErr = "the guest agent did not acknowledge the freeze within 30s, so the snapshot was taken unquiesced"
		}
		_, err := conn.ExecContext(ctx, `
			INSERT INTO plugin_addon_kvm_snapshot_schedules (
				id, vm_id, name, cron, enabled, keep_count, with_memory,
				last_run_at, next_run_at, last_error, created_at, updated_at, pause_guest
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO NOTHING`,
			id, vmID, name, cron, boolToInt(i%7 != 3), 12, boolToInt(i%4 == 0),
			now.Add(-2*time.Hour).UTC().Format(time.RFC3339),
			now.Add(4*time.Hour).UTC().Format(time.RFC3339),
			lastErr, stamp, stamp, boolToInt(i%3 == 0))
		if err != nil {
			return count, fmt.Errorf("inserting kvm snapshot schedule: %w", err)
		}
		count++
	}
	return count, nil
}

// seedCTBackups is seedVMBackups for system containers. LXC has no snapshot
// schedule table, so the snapshot half is VM-only.
func seedCTBackups(ctx context.Context, conn *sql.DB, now time.Time) (plans, runs int, err error) {
	stamp := now.UTC().Format(time.RFC3339)
	subjects, err := pickComputeSubjects(ctx, conn, "plugin_addon_lxc_containers", 14)
	if err != nil {
		return 0, 0, err
	}
	offsite := stableID("storage-location", demoOffsiteName)

	// bounded by the fixed demo estate; the seed is marked done only after it completes
	for i, ct := range subjects {
		planID := stableID("lxc-backup-plan", ct.id)
		_, err = conn.ExecContext(ctx, `
			INSERT INTO plugin_addon_lxc_backup_plans (
				id, container_id, name, cron, enabled, retention_count, retention_days,
				full_every, last_run_at, next_run_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO NOTHING`,
			planID, ct.id, "Nightly · "+ct.name, backupCron(i), boolToInt(i%6 != 4), 5, 21, 7,
			now.AddDate(0, 0, -1).UTC().Format(time.RFC3339),
			now.AddDate(0, 0, 1).UTC().Format(time.RFC3339), stamp, stamp)
		if err != nil {
			return plans, runs, fmt.Errorf("inserting lxc backup plan: %w", err)
		}
		plans++

		// bounded by the fixed demo estate; the seed is marked done only after it completes
		for _, locID := range []string{demoLocalLocation, offsite} {
			if _, err = conn.ExecContext(ctx, `
				INSERT INTO plugin_addon_lxc_backup_plan_destinations (plan_id, storage_location_id, created_at)
				VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, planID, locID, stamp); err != nil {
				return plans, runs, fmt.Errorf("linking lxc plan destination: %w", err)
			}
		}

		chainID := stableID("lxc-backup-chain", ct.id)
		// bounded by the fixed demo estate; the seed is marked done only after it completes
		for _, shape := range recentRuns() {
			runID := stableID("lxc-backup-run", fmt.Sprintf("%s/%d/%s", planID, shape.daysAgo, shape.operation))
			started := now.AddDate(0, 0, -shape.daysAgo)
			var completedAt any = started.Add(time.Duration(shape.minutes) * time.Minute).UTC().Format(time.RFC3339)
			var failure any
			if shape.failure != "" {
				failure = shape.failure
			}
			_, err = conn.ExecContext(ctx, `
				INSERT INTO plugin_addon_lxc_backup_runs (
					id, plan_id, container_id, container_name, environment_id, operation, status, kind,
					chain_id, fell_back_to_full, consistency, bytes_total, error,
					progress_stage, progress_message, progress_updated_at,
					started_at, completed_at, duration_ms, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO NOTHING`,
				runID, planID, ct.id, ct.name, ct.envID, shape.operation, shape.status, shape.kind,
				chainID, consistencyFor(i), int64(shape.gib)<<30, failure,
				progressStageFor(shape.status), completedAt,
				started.UTC().Format(time.RFC3339), completedAt,
				int64(shape.minutes)*60_000, stamp, stamp)
			if err != nil {
				return plans, runs, fmt.Errorf("inserting lxc backup run: %w", err)
			}
			runs++
		}
	}
	return plans, runs, nil
}

// nameForLocation maps a seeded location id back to its display name, which the
// destination rows carry so the page reads correctly after a location is deleted.
func nameForLocation(id string) string {
	if id == demoLocalLocation {
		return "Local storage"
	}
	return demoOffsiteName
}

// snapshotVMIDs lists up to 24 VMs that already carry snapshots and closes the cursor before schedules are written.
func snapshotVMIDs(ctx context.Context, conn *sql.DB) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT DISTINCT vm_id FROM plugin_addon_kvm_snapshots ORDER BY vm_id LIMIT 24`)
	if err != nil {
		return nil, fmt.Errorf("reading kvm snapshots: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	var vmIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning kvm snapshot vm: %w", err)
		}
		vmIDs = append(vmIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return vmIDs, nil
}
