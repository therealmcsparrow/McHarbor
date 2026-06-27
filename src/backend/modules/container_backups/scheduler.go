// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"
)

// Scheduler runs enabled container backup plans.
type Scheduler struct {
	service *Service
	logger  *slog.Logger
	running sync.Map
}

// NewScheduler creates a backup scheduler.
func NewScheduler(service *Service, logger *slog.Logger) *Scheduler {
	return &Scheduler{service: service, logger: logger}
}

// Start runs the scheduler loop until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	s.check(ctx, time.Now().UTC())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.check(ctx, now.UTC())
		}
	}
}

func (s *Scheduler) check(ctx context.Context, now time.Time) {
	if err := s.service.RecoverAbandonedRuns(ctx, "", ""); err != nil {
		s.logger.Warn("container backup scheduler: abandoned run recovery failed", "error", err)
	}

	rows, err := s.service.db.QueryContext(ctx, `
		SELECT id, name, environment_id, container_id, container_name, COALESCE(storage_location_id, ''),
		       include_config, include_logs, include_filesystem, include_image, selected_mounts,
		       log_tail_lines,
		       COALESCE(cron, ''), enabled, retention_count, retention_days, COALESCE(last_run_at, ''), COALESCE(next_run_at, ''),
		       created_at, updated_at
		FROM container_backup_plans
		WHERE enabled = 1 AND COALESCE(cron, '') <> ''
		LIMIT 1000`)
	if err != nil {
		s.logger.Error("container backup scheduler: list plans failed", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			s.logger.Error("container backup scheduler: scan plan failed", "error", err)
			continue
		}
		if err := s.service.hydratePlanStorageLocations(ctx, &plan); err != nil {
			s.logger.Error("container backup scheduler: load plan destinations failed", "plan", plan.ID, "error", err)
			continue
		}
		if !cronMatches(plan.Cron, now.Truncate(time.Minute)) {
			continue
		}
		if _, loaded := s.running.LoadOrStore(plan.ID, true); loaded {
			continue
		}
		// Defensive DB check: if a previous scheduler tick somehow left
		// the in-memory map out of sync (e.g. process restart while a run
		// is still active), make sure we don't queue a duplicate run for
		// the same plan. The orphaned-run reaper will eventually clean up
		// truly stuck runs.
		if running, err := s.service.HasRunningRunForPlan(ctx, plan.ID); err != nil {
			s.logger.Warn("container backup scheduler: running-run check failed", "plan", plan.ID, "error", err)
		} else if running {
			s.running.Delete(plan.ID)
			continue
		}
		go s.runPlan(plan.ID)
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("container backup scheduler: iterate plans failed", "error", err)
	}
}

func (s *Scheduler) runPlan(planID string) {
	defer s.running.Delete(planID)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	// StartPlan spawns a background goroutine that performs the actual
	// backup (archive + upload). Previously the scheduler called RunPlan,
	// which executed the backup synchronously and blocked the scheduler
	// goroutine — and therefore the whole check loop — for the entire
	// duration of the backup, including slow storage uploads. By
	// delegating the work to a background goroutine and waiting for the
	// run to reach a terminal state via DB polling, the scheduler stays
	// responsive and the HTTP server is not blocked.
	run, err := s.service.StartPlan(ctx, planID)
	if err != nil {
		s.logger.Error("scheduled container backup failed to start", "plan", planID, "error", err)
		return
	}

	if err := s.service.WaitForRun(ctx, run.ID); err != nil {
		s.logger.Error("scheduled container backup failed", "plan", planID, "error", err)
	}

	// Use a fresh context for the timestamp update so a near-deadline
	// parent context does not fail the bookkeeping write.
	tsCtx, tsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer tsCancel()
	now := time.Now().UTC()
	next := ""
	var cronSpec string
	if err := s.service.db.QueryRowContext(tsCtx, "SELECT COALESCE(cron, '') FROM container_backup_plans WHERE id = ?", planID).Scan(&cronSpec); err == nil && cronSpec != "" {
		next = nextCronRun(cronSpec, now).Format(time.RFC3339)
	} else if err != nil && err != sql.ErrNoRows {
		s.logger.Warn("container backup scheduler: next run lookup failed", "plan", planID, "error", err)
	}
	if _, err := s.service.db.ExecContext(tsCtx, "UPDATE container_backup_plans SET last_run_at = ?, next_run_at = ?, updated_at = ? WHERE id = ?", now.Format(time.RFC3339), nullString(next), now.Format(time.RFC3339), planID); err != nil {
		s.logger.Warn("container backup scheduler: update run timestamps failed", "plan", planID, "error", err)
	}
}
