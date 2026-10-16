// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

// markSeeded records that a demo seed of this version has been written, so the
// next boot leaves the viewer's edits alone.
func markSeeded(ctx context.Context, conn *sql.DB, key, version string, now time.Time) error {
	stamp := now.UTC().Format(time.RFC3339)
	_, err := conn.ExecContext(ctx, `
		INSERT INTO settings (id, key, value, category, created_at, updated_at)
		VALUES (?, ?, ?, 'demo', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		stableID("setting", key), key, version, stamp, stamp)
	if err != nil {
		return fmt.Errorf("marking %s: %w", key, err)
	}
	return nil
}

// backupCron spreads plans across the night rather than giving every plan the same
// window, because a schedule list where every row reads 02:00 hides whether the
// readable description is working at all.
func backupCron(i int) string {
	return fmt.Sprintf("%d %d * * *", (i*7)%60, 1+(i%4))
}

// seedContainerBackups gives a slice of the demo's containers a plan and a run
// history. Only running containers are picked: a plan on a dead container is a
// legitimate state but a poor first impression of the page.
func seedContainerBackups(ctx context.Context, conn *sql.DB, e *estate.Estate,
	envIDs map[string]string, now time.Time) (plans, runs int, err error) {
	stamp := now.UTC().Format(time.RFC3339)

	type target struct {
		envID, containerID, containerName string
	}
	var targets []target
	for _, h := range e.Hosts {
		envID, ok := envIDs[h.Key]
		if !ok {
			continue
		}
		perHost := 0
		for _, c := range h.Containers() {
			if c.State != estate.StateRunning || perHost >= 3 {
				continue
			}
			perHost++
			targets = append(targets, target{envID: envID, containerID: c.ID, containerName: c.Name})
		}
		if len(targets) >= 24 {
			break
		}
	}

	// bounded by the fixed demo estate; the seed is marked done only after it completes
	for i, t := range targets {
		planID := stableID("container-backup-plan", t.envID+"/"+t.containerName)
		cron := backupCron(i)
		// Every third plan is disabled or unscheduled, so the list shows the
		// states an operator has to be able to tell apart.
		enabled := 1
		if i%7 == 3 {
			enabled = 0
		}
		lastRun := now.AddDate(0, 0, -1).UTC().Format(time.RFC3339)
		nextRun := now.AddDate(0, 0, 1).UTC().Format(time.RFC3339)
		_, err = conn.ExecContext(ctx, `
			INSERT INTO container_backup_plans (
				id, name, environment_id, container_id, container_name, storage_location_id,
				include_config, include_logs, include_filesystem, include_image, selected_mounts,
				cron, enabled, last_run_at, next_run_at, created_at, updated_at,
				retention_count, retention_days, log_tail_lines, consistency_mode, selections
			) VALUES (?, ?, ?, ?, ?, ?, 1, 1, 1, ?, '[]', ?, ?, ?, ?, ?, ?, ?, ?, 2000, ?, '{}')
			ON CONFLICT(id) DO NOTHING`,
			planID, "Nightly · "+t.containerName, t.envID, t.containerID, t.containerName,
			demoLocalLocation, boolToInt(i%3 == 0), cron, enabled, lastRun, nextRun, stamp, stamp,
			7, 30, consistencyFor(i))
		if err != nil {
			return plans, runs, fmt.Errorf("inserting container backup plan: %w", err)
		}
		plans++

		// bounded by the fixed demo estate; the seed is marked done only after it completes
		for _, locID := range []string{demoLocalLocation, stableID("storage-location", demoOffsiteName)} {
			if _, err = conn.ExecContext(ctx, `
				INSERT INTO container_backup_plan_storage_locations (plan_id, storage_location_id, created_at)
				VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, planID, locID, stamp); err != nil {
				return plans, runs, fmt.Errorf("linking plan destination: %w", err)
			}
		}

		// bounded by the fixed demo estate; the seed is marked done only after it completes
		for _, shape := range recentRuns() {
			runID := stableID("container-backup-run", fmt.Sprintf("%s/%d/%s", planID, shape.daysAgo, shape.operation))
			started := now.AddDate(0, 0, -shape.daysAgo)
			completed := started.Add(time.Duration(shape.minutes) * time.Minute)
			var completedAt any
			if shape.status != "running" {
				completedAt = completed.UTC().Format(time.RFC3339)
			}
			var failure any
			if shape.failure != "" {
				failure = shape.failure
			}
			_, err = conn.ExecContext(ctx, `
				INSERT INTO container_backup_runs (
					id, plan_id, environment_id, container_id, status, archive_path, archive_size,
					error, started_at, completed_at, duration_ms, created_at, updated_at,
					archive_encryption, archive_key_id, progress_stage, progress_message,
					progress_updated_at, operation, container_name, included_items, available_items
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'aes-256-gcm', ?, ?, '', ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO NOTHING`,
				runID, planID, t.envID, t.containerID, shape.status,
				fmt.Sprintf("/mnt/backup/%s/%s.tar.zst", t.containerName, started.UTC().Format("20060102-1504")),
				int64(shape.gib)<<30, failure,
				started.UTC().Format(time.RFC3339), completedAt,
				int64(shape.minutes)*60_000, stamp, stamp,
				"demo-key-1", progressStageFor(shape.status),
				completedAt, shape.operation, t.containerName,
				`["config","logs","filesystem"]`, `["config","logs","filesystem","image"]`)
			if err != nil {
				return plans, runs, fmt.Errorf("inserting container backup run: %w", err)
			}
			runs++

			if err = seedRunDestinations(ctx, conn, runID, shape, stamp); err != nil {
				return plans, runs, err
			}
		}
	}
	return plans, runs, nil
}

// seedRunDestinations writes the per-destination rows behind one run. The failed
// run fails on the offsite copy only, which is the case worth showing: the local
// copy exists and the operator still has to act.
func seedRunDestinations(ctx context.Context, conn *sql.DB, runID string, shape backupRunShape, stamp string) error {
	offsite := stableID("storage-location", demoOffsiteName)
	// bounded by the fixed demo estate; the seed is marked done only after it completes
	for _, d := range []struct {
		locID, name, status, errText string
	}{
		{demoLocalLocation, "Local storage", "success", ""},
		{offsite, demoOffsiteName, shape.status, shape.failure},
	} {
		status := d.status
		if status == "failure" && d.errText == "" {
			status = "success"
		}
		var uploadedAt any
		if status == "success" {
			uploadedAt = stamp
		}
		_, err := conn.ExecContext(ctx, `
			INSERT INTO container_backup_run_destinations (
				id, run_id, storage_location_id, storage_location_name, location_type, status,
				remote_path, error, uploaded_at, created_at, updated_at, bytes_uploaded, bytes_total
			) VALUES (?, ?, ?, ?, 'local', ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO NOTHING`,
			stableID("container-backup-dest", runID+"/"+d.locID), runID, d.locID, d.name, status,
			"/"+d.name+"/demo", d.errText, uploadedAt, stamp, stamp,
			int64(shape.gib)<<30, int64(shape.gib)<<30)
		if err != nil {
			return fmt.Errorf("inserting run destination: %w", err)
		}
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// consistencyFor varies the consistency mode across plans so the column is not a
// single repeated value.
func consistencyFor(i int) string {
	switch i % 3 {
	case 0:
		return "snapshot"
	case 1:
		return "pause"
	default:
		return "none"
	}
}

// progressStageFor is the stage a finished run rests on.
func progressStageFor(status string) string {
	switch status {
	case "success":
		return "done"
	case "failure":
		return "upload"
	default:
		return "archive"
	}
}
