// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// lxcBackupSpec describes this plugin to the host's Backups page.
func lxcBackupSpec() plugin.BackupProviderSpec {
	return plugin.BackupProviderSpec{
		Slug:        "addon-lxc",
		SubjectKind: "ct",
		DisplayName: "System Containers",
		Icon:        "IconBox",
		Capabilities: plugin.BackupProviderCapabilities{
			Restore: true,
			Delete:  true,
			Plans:   true,
		},
	}
}

// backupProvider answers the host's Backups page on this plugin's behalf.
//
// Reads only: starting, restoring and deleting stay on this plugin's own routes
// under ct.backup and ct.restore, because those are the operations where getting
// authorization wrong actually costs something.
type backupProvider struct {
	plugin *LXCPlugin
}

var _ plugin.BackupProviderClient = (*backupProvider)(nil)

// Runs returns this plugin's backups and restores, newest first.
func (p *backupProvider) Runs(ctx context.Context, query plugin.BackupRunQuery) ([]plugin.BackupRunSummary, error) {
	svc, err := p.service()
	if err != nil {
		return nil, err
	}

	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	where := []string{"1=1"}
	args := []any{}
	if query.EnvironmentID != "" {
		where = append(where, "environment_id = ?")
		args = append(args, query.EnvironmentID)
	}
	if !query.Since.IsZero() {
		where = append(where, "started_at >= ?")
		args = append(args, query.Since.UTC().Format(time.RFC3339))
	}
	args = append(args, limit)

	rows, err := svc.db.Query(ctx, `SELECT `+backupRunColumns+`
		  FROM plugin_addon_lxc_backup_runs
		 WHERE `+strings.Join(where, " AND ")+`
		 ORDER BY started_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("system containers: reading backup history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var runs []BackupRun
	for rows.Next() {
		run, err := scanBackupRun(rows)
		if err != nil {
			return nil, fmt.Errorf("system containers: reading backup history: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("system containers: reading backup history: %w", err)
	}

	out := make([]plugin.BackupRunSummary, 0, len(runs))
	for _, run := range runs {
		summary := plugin.BackupRunSummary{
			ID:            run.ID,
			SubjectID:     run.ContainerID,
			SubjectName:   nameOrID(run.ContainerName, run.ContainerID),
			EnvironmentID: run.EnvironmentID,
			Operation:     run.Operation,
			Status:        run.Status,
			Kind:          run.Kind,
			SizeBytes:     run.BytesTotal,
			Error:         run.Error,
			Warning:       backupRunWarning(run),
			StartedAt:     run.StartedAt,
			CompletedAt:   run.CompletedAt,
			DurationMS:    run.DurationMS,
		}
		// Best effort: a run whose destinations cannot be read still belongs on
		// the page, with the column blank rather than the row missing.
		if dests, err := svc.runDestinations(ctx, run.ID); err == nil {
			summary.Destinations = backupDestinationNames(dests)
		}
		out = append(out, summary)
	}
	return out, nil
}

// Plans returns this plugin's backup schedules for one environment.
func (p *backupProvider) Plans(ctx context.Context, environmentID string) ([]plugin.BackupPlanSummary, error) {
	svc, err := p.service()
	if err != nil {
		return nil, err
	}

	// Joined to the container because a plan has no environment of its own — it
	// belongs to one only through what it backs up.
	where := "1=1"
	args := []any{}
	if environmentID != "" {
		where = "c.environment_id = ?"
		args = append(args, environmentID)
	}
	args = append(args, maxRows)

	rows, err := svc.db.Query(ctx, `
		SELECT p.id, p.container_id, COALESCE(c.name, ''), COALESCE(c.environment_id, ''),
		       p.name, COALESCE(p.cron, ''), p.enabled,
		       p.retention_count, p.retention_days,
		       COALESCE(p.last_run_at, ''), COALESCE(p.next_run_at, '')
		  FROM plugin_addon_lxc_backup_plans p
		  LEFT JOIN plugin_addon_lxc_containers c ON c.id = p.container_id
		 WHERE `+where+`
		 ORDER BY p.created_at LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("system containers: reading backup schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []plugin.BackupPlanSummary
	for rows.Next() {
		var (
			id, ctID, ctName, envID string
			name, cron              string
			enabled                 int
			keepCount, keepDays     int
			lastRun, nextRun        string
		)
		if err := rows.Scan(&id, &ctID, &ctName, &envID, &name, &cron, &enabled,
			&keepCount, &keepDays, &lastRun, &nextRun); err != nil {
			return nil, fmt.Errorf("system containers: reading backup schedules: %w", err)
		}
		out = append(out, plugin.BackupPlanSummary{
			ID:             id,
			SubjectID:      ctID,
			SubjectName:    nameOrID(ctName, ctID),
			EnvironmentID:  envID,
			Name:           name,
			Cron:           cron,
			Enabled:        enabled == 1,
			RetentionCount: keepCount,
			RetentionDays:  keepDays,
			LastRunAt:      parseTimePtr(lastRun),
			NextRunAt:      parseTimePtr(nextRun),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("system containers: reading backup schedules: %w", err)
	}
	return out, nil
}

// service reports the plugin as not ready rather than panicking on a nil
// receiver, because the page calls every provider whether or not each one
// finished starting.
func (p *backupProvider) service() (*Service, error) {
	if p == nil || p.plugin == nil || p.plugin.svc == nil {
		return nil, fmt.Errorf("system containers: the plugin is not ready yet")
	}
	return p.plugin.svc, nil
}

// backupDestinationNames names where a run's copies actually landed.
func backupDestinationNames(dests []BackupRunDestination) []string {
	seen := make(map[string]struct{}, len(dests))
	var out []string
	for _, dest := range dests {
		if dest.Status != DestinationSuccess {
			continue
		}
		label := dest.LocationName
		if strings.TrimSpace(label) == "" {
			label = dest.LocationID
		}
		if _, duplicate := seen[label]; duplicate {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

// backupRunWarning turns a successful-but-notable run into one sentence.
//
// A downgraded incremental is a success an operator would want to know about and
// is not an error, which is why the contract carries the two apart. A frozen copy
// is worth saying too: the container stopped executing for the length of it.
func backupRunWarning(run BackupRun) string {
	if run.Operation != BackupOpBackup || run.Status != BackupStatusSuccess {
		return ""
	}
	if run.FellBackToFull {
		if run.FallbackReason != "" {
			return "Taken as a full copy: " + run.FallbackReason
		}
		return "Taken as a full copy rather than an incremental."
	}
	if run.Consistency == BackupConsistencyFrozen {
		return "Taken with the container frozen: it executed nothing for the length of the copy."
	}
	if run.Consistency == BackupConsistencyLiveCopy {
		return "Copied while the container kept running: not point-in-time, files were read at different moments."
	}
	return ""
}
