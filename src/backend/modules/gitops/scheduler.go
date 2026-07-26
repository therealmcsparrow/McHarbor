// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Scheduler polls GitOps pipelines on a configurable interval and
// triggers Auto-promotions when a new commit is detected on the
// watched branch.
//
// Commit detection is a stub for now: in production this would do a
// `git ls-remote <repo> <branch>` and compare the SHA to the last
// known commit. The integration with the existing git_repos.auto_sync
// polling allows a single cadencer.
type Scheduler struct {
	service *Service
	interval time.Duration
	cancel context.CancelFunc
	wg sync.WaitGroup
}

// NewScheduler constructs a GitOps scheduler.
func NewScheduler(svc *Service, interval time.Duration) *Scheduler {
	if interval == 0 {
		interval = 60 * time.Second
	}
	return &Scheduler{service: svc, interval: interval}
}

// Start launches the polling loop in a goroutine.
func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go s.run(ctx)
}

// Stop signals the polling loop to exit and waits for cleanup.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// run polls every interval until ctx is canceled.
func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	slog.Info("gitops: scheduler started", "interval", s.interval)
	for {
		select {
		case <-ctx.Done():
			slog.Info("gitops: scheduler stopped")
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick scans all enabled pipelines and triggers promotion if a new
// commit is detected on the watched branch. Stub: in production this
// would compare the remote HEAD to the cached commit and, on change,
// trigger Promote().
func (s *Scheduler) tick(ctx context.Context) {
	if s.service == nil {
		return
	}
	pipelines, err := s.service.ListPipelines()
	if err != nil {
		slog.Warn("gitops: scheduler list pipelines failed", "error", err)
		return
	}
	for i := range pipelines {
		if !pipelines[i].Enabled {
			continue
		}
		if pipelines[i].TriggerType == TriggerTypeManual {
			continue
		}
		// Stub: log only — actual commit detection would do a git
		// ls-remote and call s.service.Promote(...) on change.
		slog.Debug("gitops: scheduler tick", "pipeline", pipelines[i].ID, "name", pipelines[i].Name)
	}
	_ = ctx
}
