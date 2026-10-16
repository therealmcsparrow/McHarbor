// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// Sweep cadence. Daily rather than hourly: a sweep is many Docker calls per
// environment and the state it reads — daemon flags and container runtime
// settings — changes when somebody deploys, not minute to minute.
const (
	sweepInterval = 24 * time.Hour
	sweepDelay    = 5 * time.Minute
)

// leaseCoordinator is the slice of the advisory-lock coordinator this scheduler
// needs, declared here so the module never imports the coordinator's internals.
type leaseCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
}

// Scheduler runs the pack for every active environment once a day, on whichever
// node holds the compliance-packs lease.
type Scheduler struct {
	runner *Runner
	svc    *Service
	db     *sql.DB
	coord  leaseCoordinator
	logger *slog.Logger
	role   string
}

// NewScheduler returns a Scheduler; a nil coord means single-node, where every
// tick runs rather than waiting for a lease that nobody grants.
func NewScheduler(runner *Runner, svc *Service, database *sql.DB, coord leaseCoordinator, role string, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{runner: runner, svc: svc, db: database, coord: coord, role: role, logger: logger}
}

// Start blocks until ctx is done, sweeping once after a short delay and then on
// the interval. The delay exists so a restart does not put a daemon sweep in
// front of the first page load.
func (s *Scheduler) Start(ctx context.Context) {
	first := time.NewTimer(sweepDelay)
	defer first.Stop()
	select {
	case <-ctx.Done():
		return
	case <-first.C:
		s.tick(ctx)
	}
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	if !s.leader(ctx) {
		s.logger.Debug("compliance packs: not leader this tick; skipping", "node", s.nodeID())
		return
	}
	if err := s.svc.Prune(ctx); err != nil {
		s.logger.Warn("compliance packs: pruning old runs failed", "error", err)
	}
	for _, envID := range s.environments(ctx) {
		for _, f := range Frameworks() {
			if !f.Runnable {
				continue
			}
			if _, err := s.runner.Run(ctx, f.Key, envID, TriggerScheduled, ""); err != nil {
				s.logger.Warn("compliance packs: scheduled run failed",
					"framework", f.Key, "environment", envID, "error", err)
			}
		}
	}
}

func (s *Scheduler) leader(ctx context.Context) bool {
	if s.coord == nil {
		return true
	}
	return s.coord.LeaderOf(ctx, s.role)
}

func (s *Scheduler) nodeID() string {
	if s.coord == nil {
		return ""
	}
	return s.coord.NodeID()
}

// environments returns the active environment IDs to sweep. An environment that
// cannot be listed yields no run rather than a run against the default, which
// would attribute one host's findings to another.
func (s *Scheduler) environments(ctx context.Context) []string {
	if s.db == nil {
		return nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM environments WHERE is_active = 1 ORDER BY is_default DESC, name LIMIT 1000`)
	if err != nil {
		s.logger.Warn("compliance packs: listing environments failed", "error", err)
		return nil
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("compliance packs: closing rows failed", "error", err)
		}
	}()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			s.logger.Warn("compliance packs: scanning environment failed", "error", err)
			return out
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		s.logger.Warn("compliance packs: listing environments failed", "error", err)
	}
	return out
}
