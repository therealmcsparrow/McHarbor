// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
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
	service     *Service
	interval    time.Duration
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	coordinator schedulerCoordinator
	nodeID      string
	hasCoord    bool
}

// schedulerCoordinator is the subset of the cluster coordinator needed by
// the GitOps poller. SQLite coordinators always report leader=true, preserving
// single-node behavior.
type schedulerCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
}

// NewScheduler constructs a GitOps scheduler.
func NewScheduler(svc *Service, interval time.Duration) *Scheduler {
	if interval == 0 {
		interval = 60 * time.Second
	}
	return &Scheduler{service: svc, interval: interval}
}

// NewSchedulerWithCoordinator constructs a GitOps scheduler that only polls on
// the elected HA leader.
func NewSchedulerWithCoordinator(svc *Service, interval time.Duration, coord schedulerCoordinator, nodeID string) *Scheduler {
	s := NewScheduler(svc, interval)
	s.coordinator = coord
	s.nodeID = nodeID
	s.hasCoord = coord != nil
	return s
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
			if !s.leaderFor(ctx) {
				slog.Debug("gitops: scheduler not leader this tick; skipping", "node", s.nodeID)
				continue
			}
			quiesce.Run(func() { s.tick(ctx) })
		}
	}
}

func (s *Scheduler) leaderFor(ctx context.Context) bool {
	if !s.hasCoord {
		return true
	}
	return s.coordinator.LeaderOf(ctx, "gitops-scheduler")
}

// tick scans all enabled pipelines and triggers promotion if a new
// commit is detected on the watched branch. Stub: in production this
// tick polls every enabled, non-manual pipeline and promotes its first
// stage when the tracked branch has moved since the last observation.
//
// Only the first stage is promoted from here: later stages advance through
// their own approval and promotion flow, and auto-promoting the whole chain
// on a push would deploy straight to production on a commit.
func (s *Scheduler) tick(ctx context.Context) {
	if s.service == nil {
		return
	}
	// Demo mode seeds previews, promotions and approvals as a story to read, so
	// neither the poller nor the reaper may rewrite them, and every remote they
	// would ask is a forge that does not exist.
	if coredemo.Active() {
		return
	}
	pipelines, err := s.service.ListPipelinesCtx(ctx)
	if err != nil {
		slog.Warn("gitops: scheduler list pipelines failed", "error", err)
		return
	}
	// The reaper is a second branch in this tick rather than a new singleton
	// role: this loop is already leader-gated and already wrapped in
	// quiesce.Run, and everything the reaper acts on is a persisted column, so
	// it survives a restart without any scheduler state.
	if err := s.service.ReapPreviews(ctx); err != nil {
		slog.Warn("gitops: reaping previews failed", "error", err)
	}

	for i := range pipelines {
		if !pipelines[i].Enabled || pipelines[i].TriggerType == TriggerTypeManual {
			continue
		}
		// One unreachable remote or one bad credential must not stop the
		// other pipelines from being polled on this tick.
		if err := s.pollPipeline(ctx, &pipelines[i]); err != nil {
			slog.Warn("gitops: polling pipeline failed",
				"pipeline", pipelines[i].ID, "name", pipelines[i].Name, "error", err)
		}
	}
}

// pollPipeline resolves the remote tip for the pipeline's first stage and
// promotes when it differs from the commit last seen.
func (s *Scheduler) pollPipeline(ctx context.Context, pipeline *Pipeline) error {
	stage := firstStage(pipeline)
	if stage == nil {
		return nil
	}

	tip, err := s.service.RemoteTip(ctx, pipeline.RepoID, stage.Branch)
	if err != nil {
		return err
	}

	lastSeen, hadSeen, err := s.service.StageLastSeen(ctx, stage.ID)
	if err != nil {
		return err
	}
	if hadSeen && lastSeen == tip {
		return nil
	}

	// First observation records the tip without deploying it. Enabling a
	// pipeline should not promote whatever happened to be at HEAD at the
	// moment somebody flipped the switch.
	if !hadSeen {
		slog.Info("gitops: recording initial commit for pipeline",
			"pipeline", pipeline.ID, "stage", stage.StageName, "commit", tip)
		return s.service.RecordStageSeen(ctx, stage.ID, tip)
	}

	slog.Info("gitops: promoting on new commit",
		"pipeline", pipeline.ID, "name", pipeline.Name,
		"stage", stage.StageName, "from", lastSeen, "to", tip)

	prom, err := s.service.Promote(ctx, pipeline.ID, PromoteInput{
		StageID:     stage.ID,
		CommitSHA:   tip,
		Note:        "Promoted automatically on a new commit",
		TriggeredBy: "gitops-scheduler",
	})
	if err != nil {
		return fmt.Errorf("promoting %s: %w", pipeline.Name, err)
	}
	if prom.Status == promotionFailed {
		slog.Warn("gitops: automatic promotion failed",
			"pipeline", pipeline.ID, "stage", stage.StageName, "commit", tip, "reason", prom.LastError)
	}

	// Recorded once a promotion exists, whatever its outcome. A failed deploy
	// is in the history with its reason; retrying the same commit every tick
	// would bury it under identical failures. A new commit, or a manual
	// promote, is what tries again. A promotion that could not even be
	// recorded returned above and is retried next tick.
	return s.service.RecordStageSeen(ctx, stage.ID, tip)
}

// firstStage returns the lowest-indexed stage, which is the one a push
// promotes into.
func firstStage(pipeline *Pipeline) *Stage {
	var first *Stage
	for i := range pipeline.Stages {
		if first == nil || pipeline.Stages[i].StageIndex < first.StageIndex {
			first = &pipeline.Stages[i]
		}
	}
	return first
}
