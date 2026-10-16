// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"
)

// Retention and cadence bounds, matching the container plans so an operator
// meets one rule rather than two.
const (
	maxRetentionCount = 1000
	maxRetentionDays  = 3650
	maxFullEvery      = 365
)

// backupPlanColumns is the select list every plan read shares.
const backupPlanColumns = `
	id, vm_id, name, COALESCE(cron, ''), enabled,
	retention_count, retention_days, full_every, quiesce,
	COALESCE(last_run_at, ''), COALESCE(next_run_at, ''),
	created_at, updated_at`

// ListBackupPlans returns a machine's plans, oldest first.
func (s *Service) ListBackupPlans(ctx context.Context, vmID string) ([]BackupPlan, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupPlanColumns+`
		  FROM plugin_addon_kvm_backup_plans
		 WHERE vm_id = ? ORDER BY created_at LIMIT 1000`, vmID)
	if err != nil {
		return nil, fmt.Errorf("listing backup plans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []BackupPlan
	for rows.Next() {
		plan, err := scanBackupPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, plan)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing backup plans: %w", err)
	}
	for i := range out {
		if out[i].DestinationIDs, err = s.planDestinations(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// BackupPlanByID reads one plan with its destinations.
func (s *Service) BackupPlanByID(ctx context.Context, id string) (BackupPlan, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupPlanColumns+`
		  FROM plugin_addon_kvm_backup_plans WHERE id = ?`, id)
	if err != nil {
		return BackupPlan{}, fmt.Errorf("reading backup plan: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return BackupPlan{}, err
		}
		return BackupPlan{}, ErrNotFound
	}
	plan, err := scanBackupPlan(rows)
	if err != nil {
		return BackupPlan{}, err
	}
	if plan.DestinationIDs, err = s.planDestinations(ctx, plan.ID); err != nil {
		return BackupPlan{}, err
	}
	return plan, nil
}

// CreateBackupPlan records a new plan.
//
// A plan with no destination is refused outright: a backup that goes nowhere
// is not a backup, and discovering that after the first scheduled run is the
// worst possible moment.
func (s *Service) CreateBackupPlan(ctx context.Context, req CreateBackupPlanInput) (BackupPlan, error) {
	vm, err := s.Get(ctx, req.VMID)
	if err != nil {
		return BackupPlan{}, err
	}
	if err := validateBackupPlan(req.Name, req.Cron, req.RetentionCount,
		req.RetentionDays, req.FullEvery, req.DestinationIDs); err != nil {
		return BackupPlan{}, err
	}

	now := time.Now().UTC()
	plan := BackupPlan{
		ID:             xid.New().String(),
		VMID:           vm.ID,
		Name:           strings.TrimSpace(req.Name),
		Cron:           strings.TrimSpace(req.Cron),
		Enabled:        req.Enabled,
		RetentionCount: req.RetentionCount,
		RetentionDays:  req.RetentionDays,
		FullEvery:      req.FullEvery,
		Quiesce:        req.Quiesce,
		DestinationIDs: req.DestinationIDs,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	stamp := now.Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_backup_plans
		       (id, vm_id, name, cron, enabled, retention_count, retention_days,
		        full_every, quiesce, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.ID, plan.VMID, plan.Name, nullIfEmpty(plan.Cron), boolToInt(plan.Enabled),
		plan.RetentionCount, plan.RetentionDays, plan.FullEvery, boolToInt(plan.Quiesce),
		stamp, stamp); err != nil {
		return BackupPlan{}, fmt.Errorf("recording the backup plan: %w", err)
	}
	if err := s.setPlanDestinations(ctx, plan.ID, plan.DestinationIDs); err != nil {
		return BackupPlan{}, err
	}
	return plan, nil
}

// UpdateBackupPlan patches a plan.
func (s *Service) UpdateBackupPlan(ctx context.Context, id string, req UpdateBackupPlanInput) (BackupPlan, error) {
	plan, err := s.BackupPlanByID(ctx, id)
	if err != nil {
		return BackupPlan{}, err
	}

	if req.Name != nil {
		plan.Name = strings.TrimSpace(*req.Name)
	}
	if req.Cron != nil {
		plan.Cron = strings.TrimSpace(*req.Cron)
	}
	if req.Enabled != nil {
		plan.Enabled = *req.Enabled
	}
	if req.RetentionCount != nil {
		plan.RetentionCount = *req.RetentionCount
	}
	if req.RetentionDays != nil {
		plan.RetentionDays = *req.RetentionDays
	}
	if req.FullEvery != nil {
		plan.FullEvery = *req.FullEvery
	}
	if req.Quiesce != nil {
		plan.Quiesce = *req.Quiesce
	}
	if req.DestinationIDs != nil {
		plan.DestinationIDs = *req.DestinationIDs
	}

	// Validated on the merged plan rather than the patch: turning a plan on
	// while its cron is empty is only visible once the two are seen together.
	if err := validateBackupPlan(plan.Name, plan.Cron, plan.RetentionCount,
		plan.RetentionDays, plan.FullEvery, plan.DestinationIDs); err != nil {
		return BackupPlan{}, err
	}
	if plan.Enabled && plan.Cron == "" {
		return BackupPlan{}, fmt.Errorf("%w: a plan cannot be switched on without a schedule",
			ErrBackupPlanNeedsSchedule)
	}

	plan.UpdatedAt = time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_backup_plans
		   SET name = ?, cron = ?, enabled = ?, retention_count = ?,
		       retention_days = ?, full_every = ?, quiesce = ?, updated_at = ?
		 WHERE id = ?`,
		plan.Name, nullIfEmpty(plan.Cron), boolToInt(plan.Enabled),
		plan.RetentionCount, plan.RetentionDays, plan.FullEvery,
		boolToInt(plan.Quiesce), plan.UpdatedAt.Format(time.RFC3339), id); err != nil {
		return BackupPlan{}, fmt.Errorf("updating the backup plan: %w", err)
	}
	if req.DestinationIDs != nil {
		if err := s.setPlanDestinations(ctx, plan.ID, plan.DestinationIDs); err != nil {
			return BackupPlan{}, err
		}
	}
	return plan, nil
}

// DeleteBackupPlan removes a plan.
//
// Its runs survive with plan_id cleared rather than cascading: the archives
// they wrote are still at their destinations and still restorable, and a
// history that vanished with the schedule would make them unfindable.
func (s *Service) DeleteBackupPlan(ctx context.Context, id string) error {
	affected, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_kvm_backup_plans WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting the backup plan: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// planDestinations reads a plan's destination ids.
func (s *Service) planDestinations(ctx context.Context, planID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT storage_location_id FROM plugin_addon_kvm_backup_plan_destinations
		 WHERE plan_id = ? ORDER BY created_at LIMIT 100`, planID)
	if err != nil {
		return nil, fmt.Errorf("reading backup destinations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("reading backup destinations: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// setPlanDestinations replaces a plan's destination set.
func (s *Service) setPlanDestinations(ctx context.Context, planID string, ids []string) error {
	if _, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_kvm_backup_plan_destinations WHERE plan_id = ?`,
		planID); err != nil {
		return fmt.Errorf("replacing backup destinations: %w", err)
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := s.db.Exec(ctx, `
			INSERT OR IGNORE INTO plugin_addon_kvm_backup_plan_destinations
			       (plan_id, storage_location_id, created_at)
			VALUES (?, ?, ?)`, planID, id, stamp); err != nil {
			return fmt.Errorf("recording a backup destination: %w", err)
		}
	}
	return nil
}

// validateBackupPlan bounds everything an operator can type.
func validateBackupPlan(name, cron string, retentionCount, retentionDays, fullEvery int, destinations []string) error {
	if strings.TrimSpace(name) == "" {
		return invalid(codeNameRequired, "a backup plan needs a name")
	}
	if len(name) > 120 {
		return invalid(codeNameTooLong, "that backup plan name is too long")
	}
	if retentionCount < 0 || retentionCount > maxRetentionCount {
		return fmt.Errorf("%w: keep a number of backups between 0 and %d, where 0 means forever",
			ErrBackupKeepRange, maxRetentionCount)
	}
	if retentionDays < 0 || retentionDays > maxRetentionDays {
		return fmt.Errorf("%w: keep backups for between 0 and %d days, where 0 means forever",
			ErrBackupKeepRange, maxRetentionDays)
	}
	if fullEvery < 0 || fullEvery > maxFullEvery {
		return fmt.Errorf("%w: take a full copy every 1 to %d runs, or 0 never to force one",
			ErrBackupKeepRange, maxFullEvery)
	}
	if cron != "" && !validCronSpec(cron) {
		return invalid(codeInvalidCron, ErrInvalidCron.Error())
	}
	if len(destinations) == 0 {
		return fmt.Errorf("%w: a backup plan needs at least one destination — a backup that goes nowhere is not a backup",
			ErrBackupNoDestination)
	}
	if len(destinations) > 10 {
		return fmt.Errorf("%w: a backup plan can write to at most ten destinations",
			ErrBackupTooManyDestinations)
	}
	return nil
}
