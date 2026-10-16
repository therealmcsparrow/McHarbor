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

// kvmBackupSpec describes this plugin to the host's Backups page.
func kvmBackupSpec() plugin.BackupProviderSpec {
	return plugin.BackupProviderSpec{
		Slug:        "addon-kvm",
		SubjectKind: "vm",
		DisplayName: "Virtual Machines",
		Icon:        "IconDeviceDesktop",
		Capabilities: plugin.BackupProviderCapabilities{
			Restore: true,
			Delete:  true,
			Plans:   true,
		},
	}
}

// backupProvider answers the host's Backups page on this plugin's behalf.
//
// Reads only: starting, restoring and deleting stay on this plugin's own
// routes under its own permission keys, because those are the operations
// where getting authorization wrong actually costs something.
type backupProvider struct {
	plugin *KVMPlugin
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
		  FROM plugin_addon_kvm_backup_runs
		 WHERE `+strings.Join(where, " AND ")+`
		 ORDER BY started_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("virtual machines: reading backup history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var runs []BackupRun
	for rows.Next() {
		run, err := scanBackupRun(rows)
		if err != nil {
			return nil, fmt.Errorf("virtual machines: reading backup history: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("virtual machines: reading backup history: %w", err)
	}

	names := svc.vmNames(ctx, runSubjects(runs))

	out := make([]plugin.BackupRunSummary, 0, len(runs))
	for _, run := range runs {
		summary := plugin.BackupRunSummary{
			ID:            run.ID,
			SubjectID:     run.VMID,
			SubjectName:   nameOrID(names[run.VMID], run.VMID),
			EnvironmentID: run.EnvironmentID,
			Operation:     run.Operation,
			Status:        run.Status,
			Kind:          run.Kind,
			SizeBytes:     run.BytesTotal,
			Error:         run.Error,
			Warning:       runWarning(run),
			StartedAt:     run.StartedAt,
			CompletedAt:   run.CompletedAt,
			DurationMS:    run.DurationMS,
		}
		// Best effort: a run whose destinations cannot be read still belongs
		// on the page, with the column blank rather than the row missing.
		if dests, err := svc.runDestinations(ctx, run.ID); err == nil {
			summary.Destinations = destinationNames(dests)
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

	// Joined to the machine because a plan has no environment of its own —
	// it belongs to one only through what it backs up.
	where := "1=1"
	args := []any{}
	if environmentID != "" {
		where = "v.environment_id = ?"
		args = append(args, environmentID)
	}

	rows, err := svc.db.Query(ctx, `
		SELECT p.id, p.vm_id, COALESCE(v.name, ''), COALESCE(v.environment_id, ''),
		       p.name, COALESCE(p.cron, ''), p.enabled,
		       p.retention_count, p.retention_days,
		       COALESCE(p.last_run_at, ''), COALESCE(p.next_run_at, '')
		  FROM plugin_addon_kvm_backup_plans p
		  LEFT JOIN plugin_addon_kvm_vms v ON v.id = p.vm_id
		 WHERE `+where+`
		 ORDER BY p.created_at LIMIT 1000`, args...)
	if err != nil {
		return nil, fmt.Errorf("virtual machines: reading backup schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []plugin.BackupPlanSummary
	for rows.Next() {
		var (
			id, vmID, vmName, envID string
			name, cron              string
			enabled                 int
			keepCount, keepDays     int
			lastRun, nextRun        string
		)
		if err := rows.Scan(&id, &vmID, &vmName, &envID, &name, &cron, &enabled,
			&keepCount, &keepDays, &lastRun, &nextRun); err != nil {
			return nil, fmt.Errorf("virtual machines: reading backup schedules: %w", err)
		}
		out = append(out, plugin.BackupPlanSummary{
			ID:             id,
			SubjectID:      vmID,
			SubjectName:    nameOrID(vmName, vmID),
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
		return nil, fmt.Errorf("virtual machines: reading backup schedules: %w", err)
	}
	return out, nil
}

// service reports the plugin as not ready rather than panicking on a nil
// receiver, because the page calls every provider whether or not each one
// finished starting.
func (p *backupProvider) service() (*Service, error) {
	if p == nil || p.plugin == nil || p.plugin.svc == nil {
		return nil, fmt.Errorf("virtual machines: the plugin is not ready yet")
	}
	return p.plugin.svc, nil
}

// vmNames resolves machine ids to names in one query.
//
// A run whose machine has since been deleted still appears, with the id
// standing in — a backup outliving its machine is exactly the case somebody
// goes looking for it.
func (s *Service) vmNames(ctx context.Context, ids []string) map[string]string {
	names := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return names
	}

	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, name FROM plugin_addon_kvm_vms
		 WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`)
		 LIMIT 1000`, args...)
	if err != nil {
		return names
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return names
		}
		names[id] = name
	}
	return names
}

// runSubjects collects the distinct machines a page of runs refers to.
func runSubjects(runs []BackupRun) []string {
	seen := make(map[string]struct{}, len(runs))
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		if run.VMID == "" {
			continue
		}
		if _, dup := seen[run.VMID]; dup {
			continue
		}
		seen[run.VMID] = struct{}{}
		out = append(out, run.VMID)
	}
	return out
}

// destinationNames names where a run's copies actually landed.
func destinationNames(dests []BackupRunDestination) []string {
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
		if _, dup := seen[label]; dup {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

// runWarning turns a successful-but-notable run into one sentence.
//
// A downgraded incremental and a crash-consistent copy are both successes an
// operator would want to know about, and neither is an error — which is why
// the contract carries them apart.
func runWarning(run BackupRun) string {
	if run.Operation != BackupOpBackup || run.Status != BackupStatusSuccess {
		return ""
	}
	if run.FellBackToFull {
		if run.FallbackReason != "" {
			return "Taken as a full copy: " + run.FallbackReason
		}
		return "Taken as a full copy rather than an incremental."
	}
	if run.Consistency == ConsistencyCrash {
		return "Crash-consistent: the guest was not quiesced, so this copy looks to it like a power cut."
	}
	return ""
}

// nameOrID falls back to the id when a machine has been deleted since.
func nameOrID(name, id string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return id
}
