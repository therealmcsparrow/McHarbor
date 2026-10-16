// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Preview statuses. 'demolishing' exists so a teardown interrupted by a restart
// is finished on the reaper's next tick rather than leaving an environment
// nobody owns; 'refused' exists so a quota refusal is visible to the reviewer
// instead of vanishing into a log line.
const (
	previewActive      = "active"
	previewDemolishing = "demolishing"
	previewDemolished  = "demolished"
	previewFailed      = "failed"
	previewExpired     = "expired"
	previewRefused     = "refused"
)

// defaultPreviewTTLHours and maxPreviewTTLHours bound how long a preview lives.
// Too long is how a host fills up; too short is how a reviewer loses the
// environment halfway through a review.
const (
	defaultPreviewTTLHours = 72
	maxPreviewTTLHours     = 24 * 7
)

// previewCreationGrace is how long a preview may sit with no environment before
// the reaper treats it as a creation that half-failed. Provisioning is a few
// seconds; anything past this is a crash between the insert and the create.
const previewCreationGrace = 15 * time.Minute

// defaultHostnameTemplate is used when a pipeline configures a base domain but
// no template of its own.
const defaultHostnameTemplate = "pr-{number}.{domain}"

// ErrPreviewQuotaExhausted reports that a pipeline is already running as many
// previews as it is allowed. It is refused at admission rather than queued: a
// preview that succeeds into an exhausted host is worse than one that is refused
// with a reason the reviewer can read on the pull request.
var ErrPreviewQuotaExhausted = errors.New("gitops: preview concurrency limit reached for this pipeline")

// EnvironmentProvisioner is the seam the preview path reaches the environments
// module through. Every signature is primitives because the service registry
// hands back `any` and modules never import one another.
type EnvironmentProvisioner interface {
	CreateEphemeralEnvironment(ctx context.Context, actorUserID, name, previewID, expiresAt string) (string, error)
	DestroyEphemeralEnvironment(ctx context.Context, envID, previewID string) error
	EphemeralPreviewOf(ctx context.Context, envID string) (previewID string, ephemeral bool, err error)
	ReapExpiredEphemeral(ctx context.Context) (int, error)
}

// permissionChecker is the subset of the RBAC service the preview path uses. It
// is an interface so a test can construct the "the owner does not hold
// environments.manage" case without building a whole role graph.
type permissionChecker interface {
	HasPermission(ctx context.Context, userID, envID string, perm rbac.Permission) (bool, error)
}

// SetRBAC wires the permission checker the preview path needs to honour
// environments.manage before it creates anything. A nil service is left unset
// rather than stored, so the check fails closed instead of panicking.
func (s *Service) SetRBAC(r *rbac.Service) {
	if r == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rbacSvc = r
}

// SetPermissionChecker installs an alternative checker. Used by tests; the
// production path goes through SetRBAC.
func (s *Service) SetPermissionChecker(c permissionChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rbacSvc = c
}

// SetAudit wires the audit logger. A teardown the reaper performs on its own
// still has to be attributable, so it is recorded rather than only logged.
func (s *Service) SetAudit(l *audit.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditLog = l
}

// SetEnvironmentResolver installs a lazy lookup of the environments service.
// Resolved per call rather than at mount time so module mount order cannot
// decide whether previews work.
func (s *Service) SetEnvironmentResolver(resolve func() (EnvironmentProvisioner, bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolveEnvironments = resolve
}

// environments resolves the provisioner, or reports that previews cannot be
// provisioned on this install.
func (s *Service) environments() (EnvironmentProvisioner, bool) {
	s.mu.RLock()
	resolve := s.resolveEnvironments
	s.mu.RUnlock()
	if resolve == nil {
		return nil, false
	}
	return resolve()
}

func (s *Service) rbac() permissionChecker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rbacSvc
}

func (s *Service) audit() *audit.Logger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.auditLog
}

// HandlePullRequestWebhook is the entry point for an inbound forge delivery. It
// verifies the signature over the bytes that were actually sent before anything
// else happens, so an unsigned or wrongly signed request never reaches the
// lifecycle. A nil preview with a nil error means the action was one the forge
// sends and the lifecycle does not care about.
func (s *Service) HandlePullRequestWebhook(ctx context.Context, d WebhookDelivery) (*PRPreview, error) {
	if d.RepoID == "" {
		return nil, errors.New("gitops: repo id is required")
	}
	secret, err := s.webhookSecret(ctx, d.RepoID)
	if err != nil {
		if errors.Is(err, ErrWebhookSecretMissing) {
			slog.Warn("gitops: webhook refused, no secret configured for repository",
				"repo", d.RepoID,
				"fix", "generate a webhook secret for this repository and set it on the forge")
		}
		return nil, err
	}
	if err := d.verifySignature(secret); err != nil {
		slog.Warn("gitops: webhook refused, signature did not verify", "repo", d.RepoID)
		return nil, err
	}

	in, err := normalizePullRequest(d)
	if err != nil {
		return nil, err
	}
	if in.PRNumber == "" {
		return nil, errors.New("gitops: pull request number is required")
	}
	in.RepoID = d.RepoID

	switch in.Action {
	case actionIgnore:
		return nil, nil
	case actionOpened, actionReopened, actionSynchronize:
		return s.openOrUpdatePreview(ctx, in)
	case actionClosed:
		return s.closePreview(ctx, in, "pr-closed")
	case actionMerged:
		return s.closePreview(ctx, in, "pr-merged")
	}
	return nil, nil
}

// openOrUpdatePreview creates a preview for a newly opened pull request,
// resurrects one whose pull request was reopened, or redeploys one whose head
// commit moved.
func (s *Service) openOrUpdatePreview(ctx context.Context, in PullRequestInput) (*PRPreview, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	existing, err := s.previewByPR(ctx, in.RepoID, in.PRNumber)
	if err != nil {
		return nil, err
	}

	pipeline, err := s.findPipelineForPRPreview(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	if pipeline == nil {
		return nil, errors.New("gitops: no PR-preview pipeline configured for this repo")
	}

	if existing == nil {
		return s.createPreview(ctx, pipeline, in)
	}

	// A terminal preview whose pull request came back to life starts over. The
	// previous behaviour left the row 'demolished' forever, so reopening a pull
	// request produced nothing at all.
	if existing.Status != previewActive && existing.Status != previewDemolishing {
		return s.resurrectPreview(ctx, pipeline, existing, in)
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE gitops_pr_previews SET commit_sha = ?, pr_title = ?, updated_at = ? WHERE id = ?`,
		in.CommitSHA, in.PRTitle, now, existing.ID,
	); err != nil {
		return nil, fmt.Errorf("gitops: updating PR preview: %w", err)
	}
	existing.CommitSHA = in.CommitSHA
	existing.PRTitle = in.PRTitle

	// A new commit redeploys. Recording the SHA and stopping — which is what
	// this branch used to do — left a preview serving the code from the moment
	// the pull request was opened while claiming to show the branch.
	s.deployPreview(ctx, pipeline, existing, "PR preview redeployed on a new commit", "")
	return existing, nil
}

// createPreview admits a new preview against the pipeline's concurrency cap and
// then provisions it. The row is written before provisioning so a crash in the
// middle is visible to the reaper rather than invisible to everyone.
func (s *Service) createPreview(ctx context.Context, pipeline *Pipeline, in PullRequestInput) (*PRPreview, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	preview := &PRPreview{
		ID:           xid.New().String(),
		PipelineID:   pipeline.ID,
		RepoID:       in.RepoID,
		PRNumber:     in.PRNumber,
		PRTitle:      in.PRTitle,
		SourceBranch: in.SourceBranch,
		TargetBranch: in.TargetBranch,
		CommitSHA:    in.CommitSHA,
		Status:       previewActive,
		OpenedAt:     now,
		ExpiresAt:    previewExpiry(pipeline, time.Now().UTC()),
	}

	if err := s.admitPreview(ctx, pipeline); err != nil {
		preview.Status = previewRefused
		preview.LastError = err.Error()
		preview.ExpiresAt = ""
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO gitops_pr_previews (id, pipeline_id, repo_id, pr_number, pr_title,
		        source_branch, target_branch, commit_sha, status, expires_at, last_error,
		        opened_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		preview.ID, preview.PipelineID, preview.RepoID, preview.PRNumber, preview.PRTitle,
		preview.SourceBranch, preview.TargetBranch, preview.CommitSHA, preview.Status,
		nullableString(preview.ExpiresAt), preview.LastError, preview.OpenedAt, now, now,
	); err != nil {
		return nil, fmt.Errorf("gitops: creating PR preview: %w", err)
	}

	if preview.Status == previewRefused {
		s.commentOnPR(ctx, pipeline, preview,
			fmt.Sprintf("McHarbor refused a preview environment for this pull request: %s", preview.LastError))
		return preview, nil
	}

	s.provisionPreview(ctx, pipeline, preview)
	return preview, nil
}

// resurrectPreview returns a terminal preview to service on a reopened pull
// request, with a fresh expiry and a fresh environment.
func (s *Service) resurrectPreview(ctx context.Context, pipeline *Pipeline, preview *PRPreview, in PullRequestInput) (*PRPreview, error) {
	if err := s.admitPreview(ctx, pipeline); err != nil {
		if _, uErr := s.db.ExecContext(ctx,
			`UPDATE gitops_pr_previews SET status = ?, last_error = ?, updated_at = ? WHERE id = ?`,
			previewRefused, err.Error(), time.Now().UTC().Format(time.RFC3339), preview.ID,
		); uErr != nil {
			return nil, fmt.Errorf("gitops: recording preview refusal: %w", uErr)
		}
		preview.Status = previewRefused
		preview.LastError = err.Error()
		return preview, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	preview.Status = previewActive
	preview.CommitSHA = in.CommitSHA
	preview.PRTitle = in.PRTitle
	preview.ExpiresAt = previewExpiry(pipeline, time.Now().UTC())
	preview.PreviewEnvironmentID = ""
	preview.URL = ""
	preview.LastError = ""
	preview.ClosedAt = ""
	preview.TornDownBy = ""
	if _, err := s.db.ExecContext(ctx,
		`UPDATE gitops_pr_previews
		 SET status = ?, commit_sha = ?, pr_title = ?, expires_at = ?, preview_environment_id = NULL,
		     url = '', last_error = '', closed_at = NULL, demolish_started_at = NULL,
		     torn_down_by = '', updated_at = ?
		 WHERE id = ?`,
		preview.Status, preview.CommitSHA, preview.PRTitle, preview.ExpiresAt, now, preview.ID,
	); err != nil {
		return nil, fmt.Errorf("gitops: resurrecting PR preview: %w", err)
	}
	s.provisionPreview(ctx, pipeline, preview)
	return preview, nil
}

// admitPreview enforces the pipeline's concurrency cap. The cap lives on the
// pipeline, not on the environment, so a busy repository cannot starve another
// one sharing the same host.
func (s *Service) admitPreview(ctx context.Context, pipeline *Pipeline) error {
	limit := pipeline.PreviewMaxConcurrent
	if limit <= 0 {
		return nil
	}
	var live int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM gitops_pr_previews
		 WHERE pipeline_id = ? AND status IN (?, ?)`,
		pipeline.ID, previewActive, previewDemolishing,
	).Scan(&live); err != nil {
		return fmt.Errorf("gitops: counting live previews: %w", err)
	}
	if live >= limit {
		return fmt.Errorf("%w (%d of %d in use)", ErrPreviewQuotaExhausted, live, limit)
	}
	return nil
}

// provisionPreview creates the preview's own environment, resolves its URL and
// deploys into it. Every failure is recorded on the row and leaves the preview
// 'failed', because a half-failed preview nobody can see is a preview nobody
// ever cleans up.
func (s *Service) provisionPreview(ctx context.Context, pipeline *Pipeline, preview *PRPreview) {
	owner := strings.TrimSpace(pipeline.PreviewOwnerUserID)
	if owner == "" {
		s.failPreview(ctx, preview, "no preview owner is configured on this pipeline, so environments.manage cannot be checked against a real principal")
		return
	}
	checker := s.rbac()
	if checker == nil {
		s.failPreview(ctx, preview, "permission checking is unavailable, refusing to create an environment")
		return
	}
	allowed, err := checker.HasPermission(ctx, owner, "", rbac.PermEnvironmentsManage)
	if err != nil {
		s.failPreview(ctx, preview, fmt.Sprintf("checking environments.manage for the pipeline owner failed: %v", err))
		return
	}
	if !allowed {
		s.failPreview(ctx, preview, "the pipeline owner does not hold environments.manage, so no preview environment was created")
		return
	}

	prov, ok := s.environments()
	if !ok {
		s.failPreview(ctx, preview, "the environments service is not available, so no preview environment was created")
		return
	}

	name := previewEnvironmentName(pipeline, preview)
	envID, err := prov.CreateEphemeralEnvironment(ctx, owner, name, preview.ID, preview.ExpiresAt)
	if err != nil {
		s.failPreview(ctx, preview, fmt.Sprintf("creating the preview environment failed: %v", err))
		return
	}
	preview.PreviewEnvironmentID = envID
	preview.URL = previewURL(pipeline, preview)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE gitops_pr_previews SET preview_environment_id = ?, url = ?, last_error = '', updated_at = ? WHERE id = ?`,
		envID, preview.URL, now, preview.ID,
	); err != nil {
		slog.Warn("gitops: recording preview environment failed", "preview", preview.ID, "error", err)
	}

	s.recordAudit(ctx, owner, audit.Entry{
		Action:        "gitops.preview.created",
		Severity:      "info",
		EntityType:    "gitops_pr_preview",
		EntityID:      preview.ID,
		EntityName:    name,
		EnvironmentID: envID,
		Details:       fmt.Sprintf("PR #%s preview environment created, expires %s", preview.PRNumber, preview.ExpiresAt),
	})

	message := fmt.Sprintf("McHarbor preview environment is up and expires at %s.", preview.ExpiresAt)
	if preview.URL != "" {
		message = fmt.Sprintf("McHarbor preview environment: %s\nIt expires at %s.", preview.URL, preview.ExpiresAt)
	}
	s.deployPreview(ctx, pipeline, preview, "PR preview opened", message)
}

// previewStage is the pipeline stage a preview deploys: its branch policy and
// compose path. A pipeline with none creates the environment and deploys nothing.
func previewStage(pipeline *Pipeline) *Stage {
	for i := range pipeline.Stages {
		if pipeline.Stages[i].DeployMode == DeployPRPreview {
			return &pipeline.Stages[i]
		}
	}
	return nil
}

// deployPreview promotes the pipeline's pr_preview stage for this commit into the
// preview's own environment. The promotion is recorded here and deployed on the
// background runner, so the forge's delivery timeout is not the deploy's deadline.
// A failed deploy marks the preview 'failed' with the promotion's message; the
// pull-request comment is posted only once the stack is actually up.
func (s *Service) deployPreview(ctx context.Context, pipeline *Pipeline, preview *PRPreview, note, comment string) {
	stage := previewStage(pipeline)
	if stage == nil {
		if comment != "" {
			s.commentOnPR(ctx, pipeline, preview, comment)
		}
		return
	}

	// The stack name is stamped before the deploy starts, so a crash in the
	// middle still leaves teardown something to remove.
	preview.StackName = previewStackName(pipeline, preview)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE gitops_pr_previews SET stack_name = ?, updated_at = ? WHERE id = ?`,
		preview.StackName, time.Now().UTC().Format(time.RFC3339), preview.ID,
	); err != nil {
		s.failPreview(ctx, preview, fmt.Sprintf("recording the preview's stack failed: %v", err))
		return
	}

	run, err := s.beginPromotion(ctx, pipeline.ID, PromoteInput{
		StageID:     stage.ID,
		CommitSHA:   preview.CommitSHA,
		Note:        note,
		TriggeredBy: "webhook",
		PRNumber:    preview.PRNumber,
	}, preview)
	if err != nil {
		s.failPreview(ctx, preview, fmt.Sprintf("deploying the preview failed: %v", err))
		return
	}

	snapshot := *preview
	s.goBackground(ctx, func(bg context.Context) {
		s.runPromotion(bg, run)
		switch run.promotion.Status {
		case promotionFailed:
			s.failPreview(bg, &snapshot, "deploying the preview failed: "+run.promotion.LastError)
		case promotionSucceeded:
			if comment != "" {
				s.commentOnPR(bg, pipeline, &snapshot, comment)
			}
		}
	})
}

// failPreview records why a preview could not be stood up. The message is one a
// service deliberately persisted for the operator, which is what StatusError
// shows; it is never a raw error handed back to an HTTP client.
//
// Only an active preview is failed: a deploy that finishes after its preview
// began tearing down must not drag the row back out of 'demolishing'.
func (s *Service) failPreview(ctx context.Context, preview *PRPreview, reason string) {
	slog.Warn("gitops: preview failed", "preview", preview.ID, "pr", preview.PRNumber, "reason", reason)
	preview.Status = previewFailed
	preview.LastError = reason
	if _, err := s.db.ExecContext(context.WithoutCancel(ctx),
		`UPDATE gitops_pr_previews SET status = ?, last_error = ?, updated_at = ? WHERE id = ? AND status = ?`,
		previewFailed, reason, time.Now().UTC().Format(time.RFC3339), preview.ID, previewActive,
	); err != nil {
		slog.Error("gitops: recording preview failure failed", "preview", preview.ID, "error", err)
	}
}

// closePreview tears a preview down because its pull request closed or merged.
// The two are distinguished on the row: a forge reports a merge as a close with
// a flag, and losing that distinction loses the only record of how a preview ended.
func (s *Service) closePreview(ctx context.Context, in PullRequestInput, reason string) (*PRPreview, error) {
	preview, err := s.previewByPR(ctx, in.RepoID, in.PRNumber)
	if err != nil {
		return nil, err
	}
	if preview == nil {
		return nil, nil
	}
	if err := s.TeardownPreview(ctx, preview.ID, reason, "", previewDemolished); err != nil {
		return nil, err
	}
	return s.previewByID(ctx, preview.ID)
}

// TeardownPreview destroys a preview's environment and moves the row to a
// terminal state. It is safe to call twice: a preview already torn down, or one
// whose environment is already gone, is success rather than an error, which is
// what lets the reaper finish a teardown a restart interrupted.
//
// It will never delete an environment it did not create. The environment must be
// marked ephemeral and must name this preview, or the row is left alone and the
// refusal is recorded — the current pipeline-stage validation makes the
// "a preview points at production" state easy to arrange by accident.
func (s *Service) TeardownPreview(ctx context.Context, previewID, reason, actorUserID, finalStatus string) error {
	preview, err := s.previewByID(ctx, previewID)
	if err != nil {
		return err
	}
	if preview == nil {
		return nil
	}
	if finalStatus == "" {
		finalStatus = previewDemolished
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if preview.Status != previewDemolishing {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE gitops_pr_previews SET status = ?, demolish_started_at = ?, updated_at = ? WHERE id = ?`,
			previewDemolishing, now, now, previewID,
		); err != nil {
			return fmt.Errorf("gitops: marking preview for teardown: %w", err)
		}
	}

	// The stack comes down before the environment goes. stacks.environment_id
	// cascades, so deleting the environment first would drop the stack's row and
	// leave its containers running on the host with nothing pointing at them.
	if err := s.removePreviewStack(ctx, preview); err != nil {
		return err
	}

	refusal := ""
	if preview.PreviewEnvironmentID != "" {
		prov, ok := s.environments()
		if !ok {
			return errors.New("gitops: the environments service is unavailable, teardown will be retried")
		}
		ownerPreview, ephemeral, err := prov.EphemeralPreviewOf(ctx, preview.PreviewEnvironmentID)
		switch {
		case err != nil:
			return fmt.Errorf("gitops: reading preview environment provenance: %w", err)
		case !ephemeral || ownerPreview != previewID:
			refusal = fmt.Sprintf(
				"environment %s was left in place: it is not marked as a preview of %s, so it was not created by this preview and will not be deleted",
				preview.PreviewEnvironmentID, previewID)
			slog.Error("gitops: refusing to delete a non-preview environment",
				"preview", previewID, "environment", preview.PreviewEnvironmentID,
				"ephemeral", ephemeral, "markedPreview", ownerPreview)
		default:
			if err := prov.DestroyEphemeralEnvironment(ctx, preview.PreviewEnvironmentID, previewID); err != nil {
				return fmt.Errorf("gitops: destroying preview environment: %w", err)
			}
			// The link is cleared as soon as the environment is gone, so a repeat
			// of this call has nothing left to look up. Without it a second pass
			// reads the provenance of a deleted row, cannot tell "already gone"
			// from "not mine", and records a refusal that never happened.
			if _, err := s.db.ExecContext(ctx,
				`UPDATE gitops_pr_previews SET preview_environment_id = NULL, updated_at = ? WHERE id = ?`,
				now, previewID,
			); err != nil {
				return fmt.Errorf("gitops: clearing the preview environment link: %w", err)
			}
			preview.PreviewEnvironmentID = ""
		}
	}

	// A preview reaped because it failed keeps the reason it failed, which is
	// the only thing the operator still needs from the row.
	lastError := refusal
	if lastError == "" && finalStatus == previewFailed {
		lastError = preview.LastError
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE gitops_pr_previews
		 SET status = ?, closed_at = ?, torn_down_by = ?, last_error = ?, updated_at = ?
		 WHERE id = ?`,
		finalStatus, now, reason, lastError, now, previewID,
	); err != nil {
		return fmt.Errorf("gitops: finishing preview teardown: %w", err)
	}

	s.recordAudit(ctx, actorUserID, audit.Entry{
		Action:        "gitops.preview.destroyed",
		Severity:      "warning",
		EntityType:    "gitops_pr_preview",
		EntityID:      previewID,
		EntityName:    "PR #" + preview.PRNumber,
		EnvironmentID: preview.PreviewEnvironmentID,
		Details:       fmt.Sprintf("preview torn down (%s)", reason),
	})
	return nil
}

// removePreviewStack brings a preview's deployed stack down and deletes it. It
// takes the same per-stack lock a deploy holds, so a teardown that arrives while
// the deploy is still running waits for it rather than racing it. A failure is
// recorded on the row and returned: the row stays 'demolishing' and the reaper's
// next tick retries, and the environment is never deleted over running containers.
func (s *Service) removePreviewStack(ctx context.Context, preview *PRPreview) error {
	if preview.StackName == "" {
		return nil
	}
	unlock := s.lockStack(preview.StackName)
	defer unlock()

	deployer, ok := s.stacks()
	if !ok {
		return errors.New("gitops: the stacks service is unavailable, teardown will be retried")
	}
	if err := deployer.RemoveOwnedStack(ctx, preview.PreviewEnvironmentID, preview.StackName,
		previewStackOwner(preview.ID), true); err != nil {
		message := fmt.Sprintf("removing stack %s failed, teardown will be retried: %v", preview.StackName, err)
		if _, uErr := s.db.ExecContext(ctx,
			`UPDATE gitops_pr_previews SET last_error = ?, updated_at = ? WHERE id = ?`,
			message, time.Now().UTC().Format(time.RFC3339), preview.ID,
		); uErr != nil {
			slog.Warn("gitops: recording the stack removal failure failed", "preview", preview.ID, "error", uErr)
		}
		return fmt.Errorf("gitops: removing preview stack %s: %w", preview.StackName, err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE gitops_pr_previews SET stack_name = '', updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), preview.ID,
	); err != nil {
		return fmt.Errorf("gitops: clearing the preview stack link: %w", err)
	}
	preview.StackName = ""
	return nil
}

// recordAudit writes a teardown or creation to the audit trail. The reaper acts
// on its own, so it is recorded under a named system actor rather than nil.
func (s *Service) recordAudit(ctx context.Context, actorUserID string, e audit.Entry) {
	logger := s.audit()
	if logger == nil {
		return
	}
	username := "gitops-reaper"
	if actorUserID != "" {
		username = actorUserID
	}
	if err := logger.RecordContext(context.WithoutCancel(ctx), actorUserID, username, e); err != nil {
		slog.Warn("gitops: recording preview audit entry failed", "action", e.Action, "error", err)
	}
}

// --- Reads ---

const previewColumns = `id, pipeline_id, COALESCE(repo_id, ''), pr_number, COALESCE(pr_title, ''),
	source_branch, target_branch, commit_sha, COALESCE(preview_environment_id, ''), status,
	COALESCE(url, ''), COALESCE(expires_at, ''), COALESCE(torn_down_by, ''),
	COALESCE(last_error, ''), COALESCE(comment_error, ''), opened_at, COALESCE(closed_at, ''),
	COALESCE(stack_name, '')`

func scanPreview(row interface{ Scan(...any) error }) (*PRPreview, error) {
	var p PRPreview
	if err := row.Scan(&p.ID, &p.PipelineID, &p.RepoID, &p.PRNumber, &p.PRTitle,
		&p.SourceBranch, &p.TargetBranch, &p.CommitSHA, &p.PreviewEnvironmentID, &p.Status,
		&p.URL, &p.ExpiresAt, &p.TornDownBy, &p.LastError, &p.CommentError,
		&p.OpenedAt, &p.ClosedAt, &p.StackName); err != nil {
		return nil, err
	}
	return &p, nil
}

// previewByPR finds the one preview for a repository and pull-request number.
// The pair is unique, enforced by an index, so concurrent deliveries cannot
// produce two rows for one pull request.
func (s *Service) previewByPR(ctx context.Context, repoID, prNumber string) (*PRPreview, error) {
	p, err := scanPreview(s.db.QueryRowContext(ctx,
		`SELECT `+previewColumns+` FROM gitops_pr_previews WHERE repo_id = ? AND pr_number = ?`,
		repoID, prNumber))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gitops: looking up PR preview: %w", err)
	}
	return p, nil
}

// previewByID reads one preview row.
func (s *Service) previewByID(ctx context.Context, id string) (*PRPreview, error) {
	p, err := scanPreview(s.db.QueryRowContext(ctx,
		`SELECT `+previewColumns+` FROM gitops_pr_previews WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gitops: reading PR preview: %w", err)
	}
	return p, nil
}

// PreviewByID returns one preview, or nil when there is none.
func (s *Service) PreviewByID(ctx context.Context, id string) (*PRPreview, error) {
	return s.previewByID(ctx, id)
}

// DeletePreviewRow removes a preview row once its environment is gone. Used by
// the manual destroy path so a demolished row can be cleared from the list.
func (s *Service) DeletePreviewRow(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM gitops_pr_previews WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("gitops: deleting preview row: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// --- Naming and URLs ---

// hostLabelUnsafe matches everything a DNS label and a container name both
// refuse. A branch name reaches a hostname and a Compose project name, so it is
// sanitised before it reaches either.
var hostLabelUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// sanitizeHostLabel reduces a branch or pipeline name to a single safe DNS
// label, truncated to the 63-octet limit.
func sanitizeHostLabel(in string) string {
	out := hostLabelUnsafe.ReplaceAllString(strings.ToLower(strings.TrimSpace(in)), "-")
	out = strings.Trim(out, "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
	}
	return out
}

// previewEnvironmentName is the generated environment's display name. It names
// the pipeline and the pull request so an operator reading the environment list
// can tell at a glance what it is and what created it.
func previewEnvironmentName(pipeline *Pipeline, preview *PRPreview) string {
	branch := sanitizeHostLabel(preview.SourceBranch)
	if branch == "" {
		branch = "pr"
	}
	return fmt.Sprintf("preview/%s/pr-%s-%s", sanitizeHostLabel(pipeline.Name), preview.PRNumber, branch)
}

// previewURL resolves the pipeline's hostname template against this preview.
// McHarbor manages no DNS: the template names a host the operator's own
// wildcard record already resolves, and an unconfigured pipeline simply has no
// URL rather than a broken one.
func previewURL(pipeline *Pipeline, preview *PRPreview) string {
	domain := strings.Trim(strings.TrimSpace(pipeline.PreviewBaseDomain), ".")
	template := strings.TrimSpace(pipeline.PreviewHostnameTemplate)
	if template == "" {
		if domain == "" {
			return ""
		}
		template = defaultHostnameTemplate
	}
	host := template
	host = strings.ReplaceAll(host, "{number}", preview.PRNumber)
	host = strings.ReplaceAll(host, "{branch}", sanitizeHostLabel(preview.SourceBranch))
	host = strings.ReplaceAll(host, "{pipeline}", sanitizeHostLabel(pipeline.Name))
	host = strings.ReplaceAll(host, "{domain}", domain)
	host = strings.Trim(host, ".")
	if host == "" || strings.Contains(host, "{") {
		return ""
	}
	return "https://" + host
}

// previewExpiry stamps the moment a preview stops being wanted, clamped to the
// maximum TTL. It is persisted rather than held in memory because the GitOps
// scheduler keeps no state across a restart.
func previewExpiry(pipeline *Pipeline, from time.Time) string {
	hours := pipeline.PreviewTTLHours
	if hours <= 0 {
		hours = defaultPreviewTTLHours
	}
	if hours > maxPreviewTTLHours {
		hours = maxPreviewTTLHours
	}
	return from.Add(time.Duration(hours) * time.Hour).UTC().Format(time.RFC3339)
}
