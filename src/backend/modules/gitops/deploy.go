// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// Promotion statuses a run can end in. 'skipped' is used when a preview was torn
// down before its queued deploy started, so nothing was attempted.
const (
	promotionPending   = "pending"
	promotionSucceeded = "succeeded"
	promotionFailed    = "failed"
	promotionSkipped   = "skipped"
)

// promotionStaleAfter is how long a promotion may stay 'pending' before the
// reaper records it as interrupted. It comfortably exceeds the fetch bound plus
// the stacks module's ten-minute compose bound, so a live run is never swept.
const promotionStaleAfter = 30 * time.Minute

// ErrPipelineNotFound is a promotion refusal the HTTP layer maps to a 404; its
// text is unchanged from the string the handler used to compare against.
var ErrPipelineNotFound = errors.New("pipeline not found")

// ErrStageNotFound is the other promotion refusal mapped to a 404, for a stage
// id that is not part of the named pipeline.
var ErrStageNotFound = errors.New("stage not found in pipeline")

// StackDeployer is the seam a promotion deploys through, satisfied by the stacks
// module's Service and reached through the service registry. Every signature is
// primitives because the registry hands back `any` and modules never import one another.
type StackDeployer interface {
	DeployOwnedStack(ctx context.Context, envID, name, owner, compose string) (string, error)
	RemoveOwnedStack(ctx context.Context, envID, name, owner string, removeVolumes bool) error
}

// SetStackResolver installs a lazy lookup of the stacks service. Resolved per
// call, like the environments seam, so module mount order cannot decide whether
// a promotion deploys.
func (s *Service) SetStackResolver(resolve func() (StackDeployer, bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolveStacks = resolve
}

func (s *Service) stacks() (StackDeployer, bool) {
	s.mu.RLock()
	resolve := s.resolveStacks
	s.mu.RUnlock()
	if resolve == nil {
		return nil, false
	}
	return resolve()
}

// setComposeFetcher replaces the git fetch. Tests use it to exercise the
// promotion bookkeeping without a network.
func (s *Service) setComposeFetcher(f composeFetcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetcher = f
}

func (s *Service) composeFetcher() composeFetcher {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.fetcher != nil {
		return s.fetcher
	}
	return s.fetchCompose
}

// SetBackgroundContext parents the service's background runner on the
// application's long-lived context, so shutting the application down cancels
// deploys that are still running.
func (s *Service) SetBackgroundContext(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	old := s.bgCancel
	s.bgCtx, s.bgCancel = ctx, cancel
	s.mu.Unlock()
	if old != nil {
		old()
	}
}

// Close cancels every background deploy and waits for them to record their
// outcome. A deploy cut short records 'failed', never a silent 'pending'.
func (s *Service) Close() {
	s.mu.RLock()
	cancel := s.bgCancel
	s.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
	s.bg.Wait()
}

// goBackground runs fn on the service's runner, detached from whichever request
// started it: a closed browser tab or a forge that stops waiting must not abort
// half a deploy. fn still stops when the service closes.
func (s *Service) goBackground(ctx context.Context, fn func(ctx context.Context)) {
	s.mu.RLock()
	root := s.bgCtx
	s.mu.RUnlock()
	bg, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := func() bool { return false }
	if root != nil {
		stop = context.AfterFunc(root, cancel) //nolint:contextcheck // root is the service lifecycle context, deliberately not derived from the request
	}
	s.bg.Add(1)
	go func() { // goroutine lifecycle: tracked by s.bg, ends with fn or service Close
		defer s.bg.Done()
		defer cancel()
		defer stop()
		fn(bg)
	}()
}

// waitBackground blocks until every background run has finished. Tests use it
// to observe an outcome deterministically.
func (s *Service) waitBackground() { s.bg.Wait() }

// lockStack serialises every deploy and teardown of one stack, so a preview
// teardown waits for an in-flight deploy instead of racing it.
func (s *Service) lockStack(name string) func() {
	v, _ := s.stackLocks.LoadOrStore(name, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

// deployTarget is where one promotion lands and who owns what it creates.
type deployTarget struct {
	EnvironmentID string
	StackName     string
	Owner         string
	Branch        string
	ComposePath   string
	PreviewID     string
}

// promotionRun is a recorded promotion that still has to be deployed.
type promotionRun struct {
	promotion *Promotion
	pipeline  *Pipeline
	stage     *Stage
	target    deployTarget
}

// stageStackName is the stack a stage's promotions deploy to. It is derived from
// names rather than ids because stage ids are regenerated on every pipeline edit.
func stageStackName(pipeline *Pipeline, stage *Stage) string {
	return boundedLabel("gitops-" + sanitizeHostLabel(pipeline.Name) + "-" + sanitizeHostLabel(stage.StageName))
}

// stageStackOwner is the provenance a stage's stack carries, keyed by the
// pipeline id so two pipelines with the same name can never adopt each other's stack.
func stageStackOwner(pipeline *Pipeline, stage *Stage) string {
	return "gitops:" + pipeline.ID + ":" + sanitizeHostLabel(stage.StageName)
}

// previewStackName is the stack a preview deploys to inside its own environment.
func previewStackName(pipeline *Pipeline, preview *PRPreview) string {
	return boundedLabel("preview-" + sanitizeHostLabel(pipeline.Name) + "-pr-" + sanitizeHostLabel(preview.PRNumber))
}

// previewStackOwner is the provenance a preview's stack carries.
func previewStackOwner(previewID string) string {
	return "gitops-preview:" + previewID
}

func boundedLabel(v string) string {
	v = sanitizeHostLabel(v)
	if v == "" {
		return "gitops"
	}
	return v
}

// Promote records a promotion and deploys it before returning. It is the form
// the scheduler uses, which is already off the request path. The promotion comes
// back 'succeeded' only when the stack came up, and 'failed' with the reason otherwise.
func (s *Service) Promote(ctx context.Context, pipelineID string, in PromoteInput) (*Promotion, error) {
	run, err := s.beginPromotion(ctx, pipelineID, in, nil)
	if err != nil {
		return nil, err
	}
	s.runPromotion(ctx, run)
	result := *run.promotion
	return &result, nil
}

// StartPromotion records a promotion and deploys it on the background runner,
// returning the 'pending' row at once. HTTP callers use it because a compose pull
// can outlast the server's write timeout by minutes.
func (s *Service) StartPromotion(ctx context.Context, pipelineID string, in PromoteInput) (*Promotion, error) {
	run, err := s.beginPromotion(ctx, pipelineID, in, nil)
	if err != nil {
		return nil, err
	}
	snapshot := *run.promotion
	if run.promotion.Status == promotionPending {
		s.goBackground(ctx, func(bg context.Context) { s.runPromotion(bg, run) })
	}
	return &snapshot, nil
}

// beginPromotion validates the request, resolves where it lands, and records the
// promotion. A refusal that is the pipeline's own configuration — no target, a
// branch whose tip cannot be read — is recorded as a failed promotion rather than
// returned, so the operator sees it in the history where they look.
func (s *Service) beginPromotion(ctx context.Context, pipelineID string, in PromoteInput, preview *PRPreview) (*promotionRun, error) {
	pipeline, err := s.PipelineByIDCtx(ctx, pipelineID)
	if err != nil {
		return nil, err
	}
	if pipeline == nil {
		return nil, ErrPipelineNotFound
	}
	stage, err := s.findStage(pipeline, in.StageID)
	if err != nil {
		return nil, err
	}

	target := deployTarget{
		EnvironmentID: stage.TargetEnvironmentID,
		StackName:     stageStackName(pipeline, stage),
		Owner:         stageStackOwner(pipeline, stage),
		Branch:        orEmpty(stage.Branch, "main"),
		ComposePath:   stage.ComposePath,
	}
	if preview != nil {
		target.EnvironmentID = preview.PreviewEnvironmentID
		target.StackName = previewStackName(pipeline, preview)
		target.Owner = previewStackOwner(preview.ID)
		target.Branch = orEmpty(preview.SourceBranch, target.Branch)
		target.PreviewID = preview.ID
	}

	var fromStage *Stage
	for i := range pipeline.Stages {
		if pipeline.Stages[i].StageIndex == stage.StageIndex-1 {
			fromStage = &pipeline.Stages[i]
			break
		}
	}

	preflight := ""
	commit := strings.TrimSpace(in.CommitSHA)
	if commit == "" {
		tip, tipErr := s.RemoteTip(ctx, pipeline.RepoID, target.Branch)
		if tipErr != nil {
			preflight = fmt.Sprintf("resolving the tip of branch %s failed: %v", target.Branch, tipErr)
		}
		commit = tip
	}
	if preflight == "" && target.EnvironmentID == "" {
		preflight = fmt.Sprintf("stage %s has no target environment, so there is nowhere to deploy", stage.StageName)
	}
	if preflight == "" {
		if _, pathErr := cleanComposePath(target.ComposePath); pathErr != nil {
			preflight = pathErr.Error()
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	prom := &Promotion{
		ID:            xid.New().String(),
		PipelineID:    pipelineID,
		StageID:       stage.ID,
		CommitSHA:     commit,
		TriggerKind:   inferredTriggerKind(in.TriggeredBy, in.PRNumber),
		PRNumber:      in.PRNumber,
		TriggeredBy:   in.TriggeredBy,
		Note:          in.Note,
		EnvironmentID: target.EnvironmentID,
		StackName:     target.StackName,
		StartedAt:     now,
		Status:        promotionPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if fromStage != nil {
		prom.FromStageID = fromStage.ID
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO gitops_promotions (id, pipeline_id, stage_id, deployment_id, commit_sha, from_stage_id, status,
		        trigger_kind, pr_number, triggered_by, note, environment_id, stack_name, last_error,
		        started_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?)`,
		prom.ID, prom.PipelineID, prom.StageID, nil, prom.CommitSHA,
		nullableString(prom.FromStageID), prom.Status, string(prom.TriggerKind),
		nullableString(prom.PRNumber), nullableString(prom.TriggeredBy), prom.Note,
		prom.EnvironmentID, prom.StackName, prom.StartedAt, now, now,
	); err != nil {
		return nil, fmt.Errorf("creating promotion: %w", err)
	}

	run := &promotionRun{promotion: prom, pipeline: pipeline, stage: stage, target: target}
	if preflight != "" {
		s.finishPromotion(ctx, run, promotionFailed, preflight)
	}
	return run, nil
}

// runPromotion fetches the compose file at the promoted commit and deploys it
// through the stacks seam, then records what actually happened.
func (s *Service) runPromotion(ctx context.Context, run *promotionRun) {
	if run.promotion.Status != promotionPending {
		return
	}
	unlock := s.lockStack(run.target.StackName)
	defer unlock()

	if run.target.PreviewID != "" {
		current, err := s.previewByID(ctx, run.target.PreviewID)
		if err != nil {
			s.finishPromotion(ctx, run, promotionFailed, fmt.Sprintf("reading the preview failed: %v", err))
			return
		}
		if current == nil || current.Status != previewActive {
			s.finishPromotion(ctx, run, promotionSkipped, "the preview was torn down before its deploy started")
			return
		}
	}

	deployer, ok := s.stacks()
	if !ok {
		s.finishPromotion(ctx, run, promotionFailed, "the stacks service is not available, so nothing was deployed")
		return
	}

	compose, err := s.composeFetcher()(ctx, composeRequest{
		RepoID:      run.pipeline.RepoID,
		Branch:      run.target.Branch,
		Commit:      run.promotion.CommitSHA,
		ComposePath: run.target.ComposePath,
		WorkID:      run.promotion.ID,
	})
	if err != nil {
		s.finishPromotion(ctx, run, promotionFailed, fmt.Sprintf("fetching the compose file failed: %v", err))
		return
	}

	if _, err := deployer.DeployOwnedStack(ctx, run.target.EnvironmentID, run.target.StackName, run.target.Owner, compose); err != nil {
		var ref *selfguard.Refusal
		if errors.As(err, &ref) {
			s.finishPromotion(ctx, run, promotionFailed, fmt.Sprintf("deploying stack %s was refused [%s]: %s (%s)",
				run.target.StackName, ref.Code, i18n.T(i18n.FromContext(ctx), ref.Code), strings.Join(ref.Reasons, ", ")))
			return
		}
		s.finishPromotion(ctx, run, promotionFailed, fmt.Sprintf("deploying stack %s failed: %v", run.target.StackName, err))
		return
	}
	s.finishPromotion(ctx, run, promotionSucceeded, "")

	if next := nextStage(run.pipeline, run.stage); next != nil && next.RequiresApproval {
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := s.db.ExecContext(context.WithoutCancel(ctx),
			`INSERT INTO gitops_approvals (id, pipeline_id, stage_id, commit_sha, requested_by, status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)`,
			xid.New().String(), run.pipeline.ID, next.ID, run.promotion.CommitSHA, run.promotion.TriggeredBy, now, now,
		); err != nil {
			slog.Error("gitops: creating the next stage's approval failed", "promotion", run.promotion.ID, "error", err)
		}
	}
}

// finishPromotion records a terminal outcome. It writes on a context detached
// from cancellation, so a deploy cut short by shutdown still records 'failed'
// instead of leaving 'pending' for the reaper to guess at.
func (s *Service) finishPromotion(ctx context.Context, run *promotionRun, status, message string) {
	now := time.Now().UTC().Format(time.RFC3339)
	run.promotion.Status = status
	run.promotion.LastError = message
	run.promotion.FinishedAt = now
	run.promotion.UpdatedAt = now
	if status == promotionFailed {
		slog.Warn("gitops: promotion failed", "promotion", run.promotion.ID, "pipeline", run.pipeline.ID,
			"stage", run.stage.StageName, "reason", message)
	}
	if _, err := s.db.ExecContext(context.WithoutCancel(ctx),
		`UPDATE gitops_promotions SET status = ?, last_error = ?, finished_at = ?, updated_at = ? WHERE id = ?`,
		status, message, now, now, run.promotion.ID,
	); err != nil {
		slog.Error("gitops: recording the promotion outcome failed", "promotion", run.promotion.ID, "error", err)
	}
}

// failInterruptedPromotions records every promotion left 'pending' past
// promotionStaleAfter as failed. It is the restart-mid-deploy case: the runner
// that owned the row is gone, and a row pending forever reads as still running.
func (s *Service) failInterruptedPromotions(ctx context.Context, now time.Time) error {
	cutoff := now.Add(-promotionStaleAfter).UTC().Format(time.RFC3339)
	stamp := now.UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE gitops_promotions
		 SET status = ?, last_error = ?, finished_at = ?, updated_at = ?
		 WHERE status = ? AND started_at <= ?`,
		promotionFailed, "interrupted: McHarbor stopped before the deploy finished", stamp, stamp,
		promotionPending, cutoff,
	); err != nil {
		return fmt.Errorf("gitops: failing interrupted promotions: %w", err)
	}
	return nil
}

// ListPipelines is ListPipelinesCtx without a caller context, kept for callers
// outside this package that have none to pass.
func (s *Service) ListPipelines() ([]Pipeline, error) {
	return s.ListPipelinesCtx(context.Background())
}

// PipelineByID is PipelineByIDCtx without a caller context, kept for callers
// outside this package that have none to pass.
func (s *Service) PipelineByID(id string) (*Pipeline, error) {
	return s.PipelineByIDCtx(context.Background(), id)
}

// newBackgroundRoot returns the standalone lifecycle context NewService starts
// with, before main.go parents it on the application context.
func newBackgroundRoot() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
