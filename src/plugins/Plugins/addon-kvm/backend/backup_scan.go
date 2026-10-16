// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// backupRunColumns is the select list every run read shares.
const backupRunColumns = `
	id, COALESCE(plan_id, ''), vm_id, environment_id, operation,
	COALESCE(source_run_id, ''), status, kind, COALESCE(chain_id, ''),
	fell_back_to_full, COALESCE(fallback_reason, ''), consistency,
	bytes_total, included_disks, COALESCE(error, ''),
	progress_stage, progress_message, COALESCE(progress_updated_at, ''),
	started_at, COALESCE(completed_at, ''), duration_ms`

func scanBackupPlan(rows plugin.Rows) (BackupPlan, error) {
	var plan BackupPlan
	var enabled, quiesce int
	var lastRun, nextRun, created, updated string

	if err := rows.Scan(&plan.ID, &plan.VMID, &plan.Name, &plan.Cron, &enabled,
		&plan.RetentionCount, &plan.RetentionDays, &plan.FullEvery, &quiesce,
		&lastRun, &nextRun, &created, &updated); err != nil {
		return BackupPlan{}, err
	}

	plan.Enabled = enabled == 1
	plan.Quiesce = quiesce == 1
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
	var includedDisks, progressUpdated, started, completed string

	if err := rows.Scan(&run.ID, &run.PlanID, &run.VMID, &run.EnvironmentID,
		&run.Operation, &run.SourceRunID, &run.Status, &run.Kind, &run.ChainID,
		&fellBack, &run.FallbackReason, &run.Consistency,
		&run.BytesTotal, &includedDisks, &run.Error,
		&run.ProgressStage, &run.ProgressMessage, &progressUpdated,
		&started, &completed, &run.DurationMS); err != nil {
		return BackupRun{}, err
	}

	run.FellBackToFull = fellBack == 1
	run.ProgressUpdatedAt = progressUpdated
	run.StartedAt = parseTime(started)
	run.CompletedAt = parseTimePtr(completed)
	run.Destinations = []BackupRunDestination{}
	if includedDisks != "" {
		// A blob an older build wrote in a shape this one does not know is
		// dropped rather than rendered: the run's own disks are recoverable
		// from its destination rows, and a half-parsed list would make the
		// restore dialog offer disks the run never covered.
		var disks []string
		if err := json.Unmarshal([]byte(includedDisks), &disks); err == nil {
			run.IncludedDisks = disks
		}
	}
	return run, nil
}

func scanBackupChain(rows plugin.Rows) (BackupChain, error) {
	var chain BackupChain
	var invalidated int
	var lastFull, lastRun string

	if err := rows.Scan(&chain.ID, &chain.VMID, &chain.DiskID, &chain.PlanID,
		&chain.BitmapName, &chain.Granularity, &chain.LastRunID,
		&lastFull, &lastRun, &invalidated, &chain.InvalidWhy); err != nil {
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
		&dest.DiskID, &dest.Status, &dest.RemotePath, &dest.KeyID,
		&dest.BytesUploaded, &dest.Error, &uploaded); err != nil {
		return BackupRunDestination{}, err
	}
	dest.UploadedAt = parseTimePtr(uploaded)
	return dest, nil
}

// encodeDiskList stores a run's disk list as JSON.
func encodeDiskList(disks []string) string {
	if len(disks) == 0 {
		return ""
	}
	encoded, err := json.Marshal(disks)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// backupObjectKey is where one disk's artifact lands at a destination.
//
// Environment, machine and run in the path so an operator browsing the
// destination can tell what an object is without McHarbor, and so two
// machines with the same name on different hosts cannot collide.
func backupObjectKey(envName, envID, vmName, vmID, runID, diskID, kind string) string {
	// The layout the container backups use, so one destination holds both
	// kinds in the same shape: <environment>/<subject kind>/<subject>/<file>.
	// Names rather than ids, because the operator browsing the destination is
	// looking for a machine they can recognise.
	//
	// One file per disk per run, so the run id alone cannot name it the way a
	// container's single tar can.
	fileName := plugin.SlugifyOr(runID, "run") +
		"-" + plugin.SlugifyOr(diskID, "disk") +
		"-" + plugin.SlugifyOr(kind, "full") + ".qcow2"
	return strings.Join([]string{
		plugin.SlugifyOr(envName, envID),
		"vm",
		plugin.SlugifyOr(vmName, vmID),
		fileName,
	}, "/")
}
