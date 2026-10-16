// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"sync"
	"time"
)

// backupTickInterval is how often due plans are looked for.
//
// A minute, because cron's finest granularity is a minute: checking more often
// costs queries and finds nothing new.
const backupTickInterval = time.Minute

// backupScheduler fires due backup plans.
//
// Separate from the snapshot scheduler that already exists rather than folded
// into it: the two have different retention rules and different failure
// modes, and a shared loop would mean one stalling the other.
type backupScheduler struct {
	svc    *Service
	logger pluginLogger

	stop chan struct{}
	once sync.Once

	// running guards against a plan being started twice — once by the tick
	// that found it due and once by a tick that arrived while it still ran.
	running sync.Map
}

// newBackupScheduler builds the loop.
func newBackupScheduler(svc *Service, logger pluginLogger) *backupScheduler {
	return &backupScheduler{svc: svc, logger: logger, stop: make(chan struct{})}
}

// Start runs the loop until Stop.
func (b *backupScheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(backupTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-b.stop:
			return
		case <-ticker.C:
			b.check(ctx)
		}
	}
}

// Stop ends the loop. Safe to call more than once.
func (b *backupScheduler) Stop() {
	b.once.Do(func() { close(b.stop) })
}

// check starts every plan whose schedule matches this minute.
func (b *backupScheduler) check(ctx context.Context) {
	plans, err := b.svc.duePlans(ctx, time.Now().Truncate(time.Minute))
	if err != nil {
		b.logger.Warn("kvm: looking for due backup plans", "error", err)
		return
	}

	for _, plan := range plans {
		if _, busy := b.running.LoadOrStore(plan.ID, true); busy {
			// A previous run is still going. Skipping is right: two backups
			// of one machine at once would fight over the same bitmap.
			continue
		}
		go b.runPlan(ctx, plan)
	}
}

// runPlan fires one plan and records when it ran.
func (b *backupScheduler) runPlan(ctx context.Context, plan BackupPlan) {
	defer b.running.Delete(plan.ID)

	if _, err := b.svc.StartBackupRun(ctx, plan.VMID, plan.ID, RunBackupInput{}); err != nil {
		b.logger.Warn("kvm: a scheduled backup did not start",
			"plan", plan.ID, "error", err)
		return
	}
	// Stamped on a fresh context: the run outlives this call, and a cancelled
	// parent must not leave the plan looking as though it never fired.
	stampCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	b.svc.stampPlanRun(stampCtx, plan)
}

// duePlans returns the enabled plans whose cron matches this minute.
func (s *Service) duePlans(ctx context.Context, at time.Time) ([]BackupPlan, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupPlanColumns+`
		  FROM plugin_addon_kvm_backup_plans
		 WHERE enabled = 1 AND COALESCE(cron, '') <> '' LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var due []BackupPlan
	for rows.Next() {
		plan, err := scanBackupPlan(rows)
		if err != nil {
			return nil, err
		}
		if cronMatches(plan.Cron, at) {
			due = append(due, plan)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range due {
		if due[i].DestinationIDs, err = s.planDestinations(ctx, due[i].ID); err != nil {
			return nil, err
		}
	}
	return due, nil
}

// stampPlanRun records when a plan last fired and when it is next due.
func (s *Service) stampPlanRun(ctx context.Context, plan BackupPlan) {
	now := time.Now().UTC()
	next := nextCronRun(plan.Cron, now)
	nextStamp := ""
	if !next.IsZero() {
		nextStamp = next.UTC().Format(time.RFC3339)
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_backup_plans
		   SET last_run_at = ?, next_run_at = ?, updated_at = ?
		 WHERE id = ?`,
		now.Format(time.RFC3339), nullIfEmpty(nextStamp),
		now.Format(time.RFC3339), plan.ID); err != nil {
		s.logger.Warn("kvm: stamping a backup plan", "plan", plan.ID, "error", err)
	}
}
