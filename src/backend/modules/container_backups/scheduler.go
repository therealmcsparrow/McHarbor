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
	service  *Service
	logger   *slog.Logger
	running  sync.Map
	coordinator schedulerCoordinator
	nodeID   string
	hasCoord bool
}

// schedulerCoordinator is the subset of *coordinator.Coordinator
// the scheduler depends on. Defining it as an interface keeps
// the module importable in tests without dragging in the
// coordinator package's pgx/Postgres dependencies.
type schedulerCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
}

// leaderFor returns true when this node should run the plan scan
// this tick. On SQLite (no coordinator wired in) it always
// returns true so single-node behavior is identical to the
// pre-HA path.
func (s *Scheduler) leaderFor(ctx context.Context) bool {
	if !s.hasCoord {
		return true
	}
	return s.coordinator.LeaderOf(ctx, "container-backup-scheduler")
}

// NewScheduler creates a backup scheduler for the single-node
// (no-coordinator) path. Both nodes running the same image
// would fire backups on every tick — acceptable for SQLite dev
// installs but not for active-active production. Use
// NewSchedulerWithCoordinator in main() when running against
// Postgres.
func NewScheduler(service *Service, logger *slog.Logger) *Scheduler {
	return &Scheduler{service: service, logger: logger}
}

// NewSchedulerWithCoordinator creates a scheduler that defers the
// plan-launch step to whichever node currently holds the
// `container-backup-scheduler` advisory lock. nodeID is shown in
// log lines for debugging.
func NewSchedulerWithCoordinator(
	service *Service,
	logger *slog.Logger,
	coord schedulerCoordinator,
	nodeID string,
) *Scheduler {
	return &Scheduler{
		service:     service,
		logger:      logger,
		coordinator: coord,
		nodeID:      nodeID,
		hasCoord:    coord != nil,
	}
}

// Start runs the scheduler loop until ctx is cancelled. When both
// nodes of an active-active cluster are running, both Start loops
// tick once a minute. The single tick that actually fires backups
// is determined by the shared lock acquired in s.check (so the
// loop body itself stays the same and the single-node code path
// is identical to the pre-HA behavior).
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	// Interpret cron schedules in the server's local timezone so a
	// `0 1 * * *` schedule triggers at 01:00 local time, not 01:00
	// UTC. The container's `TZ` env var drives `time.Local`.
	s.check(ctx, time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.check(ctx, now)
		}
	}
}

func (s *Scheduler) check(ctx context.Context, now time.Time) {
	// In active-active deployments, only the node that currently
	// holds the scheduler advisory lock runs the plan scan. The
	// other node still ticks (so the recovery code below runs on
	// every node) but skips the plan launch path. The recovery
	// path is per-row (UPDATE … WHERE status='running' …) so
	// both nodes can run it concurrently without conflict.
	if !s.leaderFor(ctx) {
		s.logger.Debug("container backup scheduler: not leader this tick; skipping plan launch",
			"node", s.nodeID, "now", now.Format(time.RFC3339))
		// Still run the recovery pass so a failed run on this
		// node gets cleaned up. The next leader's tick will then
		// re-evaluate.
		if err := s.service.RecoverAbandonedRuns(ctx, "", ""); err != nil {
			s.logger.Warn("container backup scheduler: abandoned run recovery failed", "error", err)
		}
		return
	}
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
	// Store timestamps in the same timezone the cron parser uses so
	// `last_run_at` and `next_run_at` round-trip consistently. `time.RFC3339`
	// includes the offset, so the UI shows the local clock time.
	now := time.Now()
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
