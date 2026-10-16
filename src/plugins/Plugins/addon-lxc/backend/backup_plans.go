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

// Retention and cadence bounds, matching the machine plans so an operator meets
// one rule rather than two.
const (
	maxRetentionCount = 1000
	maxRetentionDays  = 3650
	maxFullEvery      = 365
	maxDestinations   = 10
)

// ListBackupPlans returns a container's plans, oldest first.
func (s *Service) ListBackupPlans(ctx context.Context, containerID string) ([]BackupPlan, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupPlanColumns+`
		  FROM plugin_addon_lxc_backup_plans
		 WHERE container_id = ? ORDER BY created_at LIMIT ?`, containerID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing backup plans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []BackupPlan{}
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
		  FROM plugin_addon_lxc_backup_plans WHERE id = ?`, id)
	if err != nil {
		return BackupPlan{}, fmt.Errorf("reading the backup plan: %w", err)
	}
	found := rows.Next()
	var plan BackupPlan
	if found {
		plan, err = scanBackupPlan(rows)
	}
	_ = rows.Close()
	if err != nil {
		return BackupPlan{}, err
	}
	if !found {
		return BackupPlan{}, ErrNotFound
	}
	if plan.DestinationIDs, err = s.planDestinations(ctx, plan.ID); err != nil {
		return BackupPlan{}, err
	}
	return plan, nil
}

// CreateBackupPlan records a new plan.
//
// A plan with no destination is refused outright: a backup that goes nowhere is
// not a backup, and discovering that after the first scheduled run is the worst
// possible moment.
func (s *Service) CreateBackupPlan(ctx context.Context, req CreateBackupPlanInput) (BackupPlan, error) {
	ct, err := s.Get(ctx, req.ContainerID)
	if err != nil {
		return BackupPlan{}, err
	}
	if err := validateBackupPlan(req.Name, req.Cron, req.RetentionCount,
		req.RetentionDays, req.FullEvery, req.DestinationIDs); err != nil {
		return BackupPlan{}, err
	}
	if req.Enabled && strings.TrimSpace(req.Cron) == "" {
		return BackupPlan{}, invalid(codeInvalidCron,
			"a plan cannot be switched on without a schedule")
	}

	now := time.Now().UTC()
	plan := BackupPlan{
		ID:             xid.New().String(),
		ContainerID:    ct.ID,
		Name:           strings.TrimSpace(req.Name),
		Cron:           strings.TrimSpace(req.Cron),
		Enabled:        req.Enabled,
		RetentionCount: req.RetentionCount,
		RetentionDays:  req.RetentionDays,
		FullEvery:      req.FullEvery,
		DestinationIDs: req.DestinationIDs,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if plan.Cron != "" {
		if next := nextCronRun(plan.Cron, now); !next.IsZero() {
			stamp := next.UTC()
			plan.NextRunAt = &stamp
		}
	}

	stamp := now.Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_backup_plans
		       (id, container_id, name, cron, enabled, retention_count,
		        retention_days, full_every, next_run_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.ID, plan.ContainerID, plan.Name, nullIfEmpty(plan.Cron),
		boolToInt(plan.Enabled), plan.RetentionCount, plan.RetentionDays,
		plan.FullEvery, nullIfEmpty(formatTimePtr(plan.NextRunAt)),
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
		return BackupPlan{}, invalid(codeInvalidCron,
			"a plan cannot be switched on without a schedule")
	}

	plan.UpdatedAt = time.Now().UTC()
	plan.NextRunAt = nil
	if plan.Cron != "" {
		if next := nextCronRun(plan.Cron, plan.UpdatedAt); !next.IsZero() {
			stamp := next.UTC()
			plan.NextRunAt = &stamp
		}
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_lxc_backup_plans
		   SET name = ?, cron = ?, enabled = ?, retention_count = ?,
		       retention_days = ?, full_every = ?, next_run_at = ?, updated_at = ?
		 WHERE id = ?`,
		plan.Name, nullIfEmpty(plan.Cron), boolToInt(plan.Enabled),
		plan.RetentionCount, plan.RetentionDays, plan.FullEvery,
		nullIfEmpty(formatTimePtr(plan.NextRunAt)),
		plan.UpdatedAt.Format(time.RFC3339), id); err != nil {
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
// Its runs survive with plan_id cleared rather than cascading: the archives they
// wrote are still at their destinations and still restorable, and a history that
// vanished with the schedule would make them unfindable.
func (s *Service) DeleteBackupPlan(ctx context.Context, id string) error {
	affected, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_lxc_backup_plans WHERE id = ?`, id)
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
		SELECT storage_location_id FROM plugin_addon_lxc_backup_plan_destinations
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
//
// Delete then insert rather than an upsert: SQLite's INSERT OR IGNORE has no
// portable translation at runtime for every backend the host supports, and a
// full replace needs no conflict handling to begin with.
func (s *Service) setPlanDestinations(ctx context.Context, planID string, ids []string) error {
	if _, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_lxc_backup_plan_destinations WHERE plan_id = ?`,
		planID); err != nil {
		return fmt.Errorf("replacing backup destinations: %w", err)
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		if _, err := s.db.Exec(ctx, `
			INSERT INTO plugin_addon_lxc_backup_plan_destinations
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
	if len([]rune(name)) > 120 {
		return invalid(codeNameTooLong, "that backup plan name is too long")
	}
	if retentionCount < 0 || retentionCount > maxRetentionCount {
		return invalid(codeInvalidKeep,
			fmt.Sprintf("keep a number of backups between 0 and %d, where 0 means forever", maxRetentionCount))
	}
	if retentionDays < 0 || retentionDays > maxRetentionDays {
		return invalid(codeInvalidKeep,
			fmt.Sprintf("keep backups for between 0 and %d days, where 0 means forever", maxRetentionDays))
	}
	if fullEvery < 0 || fullEvery > maxFullEvery {
		return invalid(codeInvalidKeep,
			fmt.Sprintf("take a full copy every 1 to %d runs, or 0 never to force one", maxFullEvery))
	}
	if cron != "" && !validCronSpec(cron) {
		return invalid(codeInvalidCron, ErrInvalidCron.Error())
	}
	if len(destinations) == 0 {
		return invalid(codeNoDestination,
			"a backup plan needs at least one destination — a backup that goes nowhere is not a backup")
	}
	if len(destinations) > maxDestinations {
		return invalid(codeNoDestination,
			fmt.Sprintf("a backup plan can write to at most %d destinations", maxDestinations))
	}
	return nil
}
