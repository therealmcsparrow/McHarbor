// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// backupPlanColumns is the select list every plan read shares.
const backupPlanColumns = `
	id, container_id, name, COALESCE(cron, ''), enabled,
	retention_count, retention_days, full_every,
	COALESCE(last_run_at, ''), COALESCE(next_run_at, ''),
	created_at, updated_at`

// backupRunColumns is the select list every run read shares.
const backupRunColumns = `
	id, COALESCE(plan_id, ''), container_id, container_name, environment_id,
	operation, COALESCE(source_run_id, ''), status, kind, COALESCE(chain_id, ''),
	fell_back_to_full, COALESCE(fallback_reason, ''), consistency,
	bytes_total, target_container_id, COALESCE(error, ''),
	progress_stage, progress_message, COALESCE(progress_updated_at, ''),
	started_at, COALESCE(completed_at, ''), duration_ms,
	COALESCE((SELECT s.id_shift FROM plugin_addon_lxc_backup_run_idshifts s
	           WHERE s.id = plugin_addon_lxc_backup_runs.id), '')`

// backupChainColumns is the select list every chain read shares.
const backupChainColumns = `
	id, container_id, COALESCE(plan_id, ''), listing_name,
	COALESCE(last_run_id, ''), COALESCE(last_full_at, ''),
	COALESCE(last_run_at, ''), invalidated, COALESCE(invalid_why, '')`

// backupDestinationColumns is the select list every destination read shares.
const backupDestinationColumns = `
	id, run_id, storage_location_id, storage_location_name, part,
	status, remote_path, key_id, bytes_uploaded, error,
	COALESCE(uploaded_at, '')`

func scanBackupPlan(rows plugin.Rows) (BackupPlan, error) {
	var plan BackupPlan
	var enabled int
	var lastRun, nextRun, created, updated string

	if err := rows.Scan(&plan.ID, &plan.ContainerID, &plan.Name, &plan.Cron, &enabled,
		&plan.RetentionCount, &plan.RetentionDays, &plan.FullEvery,
		&lastRun, &nextRun, &created, &updated); err != nil {
		return BackupPlan{}, err
	}

	plan.Enabled = enabled == 1
	plan.LastRunAt = parseTimePtr(lastRun)
	plan.NextRunAt = parseTimePtr(nextRun)
	plan.CreatedAt = parseTime(created)
	plan.UpdatedAt = parseTime(updated)
	plan.DestinationIDs = []string{}
	return plan, nil
}

func scanBackupRun(rows plugin.Rows) (BackupRun, error) {
	var run BackupRun
	var fellBack int
	var progressUpdated, started, completed string

	if err := rows.Scan(&run.ID, &run.PlanID, &run.ContainerID, &run.ContainerName,
		&run.EnvironmentID, &run.Operation, &run.SourceRunID, &run.Status,
		&run.Kind, &run.ChainID, &fellBack, &run.FallbackReason, &run.Consistency,
		&run.BytesTotal, &run.TargetContainerID, &run.Error,
		&run.ProgressStage, &run.ProgressMessage, &progressUpdated,
		&started, &completed, &run.DurationMS, &run.IDShift); err != nil {
		return BackupRun{}, err
	}

	run.FellBackToFull = fellBack == 1
	run.ProgressUpdatedAt = progressUpdated
	run.StartedAt = parseTime(started)
	run.CompletedAt = parseTimePtr(completed)
	run.ErrorHint = backupErrorHint(run.Error)
	run.Destinations = []BackupRunDestination{}
	return run, nil
}

func scanBackupChain(rows plugin.Rows) (BackupChain, error) {
	var chain BackupChain
	var invalidated int
	var lastFull, lastRun string

	if err := rows.Scan(&chain.ID, &chain.ContainerID, &chain.PlanID,
		&chain.ListingName, &chain.LastRunID, &lastFull, &lastRun,
		&invalidated, &chain.InvalidWhy); err != nil {
		return BackupChain{}, err
	}

	chain.Invalidated = invalidated == 1
	chain.LastFullAt = parseTimePtr(lastFull)
	chain.LastRunAt = parseTimePtr(lastRun)
	return chain, nil
}

func scanBackupRunDestination(rows plugin.Rows) (BackupRunDestination, error) {
	var dest BackupRunDestination
	var uploaded string

	if err := rows.Scan(&dest.ID, &dest.RunID, &dest.LocationID, &dest.LocationName,
		&dest.Part, &dest.Status, &dest.RemotePath, &dest.KeyID,
		&dest.BytesUploaded, &dest.Error, &uploaded); err != nil {
		return BackupRunDestination{}, err
	}
	dest.UploadedAt = parseTimePtr(uploaded)
	return dest, nil
}

// backupObjectKey is where one part of one run lands at a destination.
//
// Environment, container and run in the path so an operator browsing the
// destination can tell what an object is without McHarbor, and so two
// containers with the same name on different hosts cannot collide. Names rather
// than ids, because the operator browsing the destination is looking for a
// container they can recognise.
func backupObjectKey(envName, envID, ctName, ctID, runID, part, kind string) string {
	extension := ".tar.gz"
	if part == BackupPartManifest {
		extension = ".json"
	}
	fileName := plugin.SlugifyOr(runID, "run") +
		"-" + plugin.SlugifyOr(part, BackupPartRootfs) +
		"-" + plugin.SlugifyOr(kind, BackupKindFull) + extension
	return strings.Join([]string{
		plugin.SlugifyOr(envName, envID),
		"ct",
		plugin.SlugifyOr(ctName, ctID),
		fileName,
	}, "/")
}

// formatTimePtr renders an optional timestamp for storage, or "" for nil so the
// column takes NULL rather than the zero time.
func formatTimePtr(at *time.Time) string {
	if at == nil || at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

// nameOrID falls back to the id when a container has been deleted since.
//
// A backup outliving its container is exactly the case somebody goes looking
// for it, so the row still has to name something.
func nameOrID(name, id string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return id
}
