// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package gitops runs promotion pipelines that carry a git commit through a chain of environments.
package gitops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// TriggerKind identifies how a pipeline promotion was triggered.
type TriggerKind string

const (
	// TriggerAuto marks a promotion the poller started on a new commit.
	TriggerAuto TriggerKind = "auto"
	// TriggerManual marks a promotion an operator started by hand.
	TriggerManual TriggerKind = "manual"
	// TriggerPRPreview marks a promotion that deploys a pull-request preview.
	TriggerPRPreview TriggerKind = "pr_preview"
	// TriggerWebhook marks a promotion a forge webhook started.
	TriggerWebhook TriggerKind = "webhook"
)

// DeployMode gates how a stage rolls out.
type DeployMode string

const (
	// DeployAuto rolls a stage out without waiting for an operator.
	DeployAuto DeployMode = "auto"
	// DeployManual holds a stage until an operator approves it.
	DeployManual DeployMode = "manual"
	// DeployPRPreview rolls a stage out only for pull-request previews.
	DeployPRPreview DeployMode = "pr_preview"
)

// TriggerType is the top-level pipeline trigger policy.
type TriggerType string

const (
	// TriggerTypeAuto lets the poller promote a pipeline on every new commit.
	TriggerTypeAuto TriggerType = "auto"
	// TriggerTypeManual restricts a pipeline to operator-started promotions.
	TriggerTypeManual TriggerType = "manual"
	// TriggerTypePRPreview limits a pipeline to pull-request previews.
	TriggerTypePRPreview TriggerType = "pr_preview"
)

// Pipeline is a named progression of stages that promotes a git commit
// from one environment to the next.
type Pipeline struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	RepoID      string      `json:"repoId"`
	Description string      `json:"description"`
	Enabled     bool        `json:"enabled"`
	TriggerType TriggerType `json:"triggerType"`
	Stages      []Stage     `json:"stages"`
	CreatedAt   string      `json:"createdAt"`
	UpdatedAt   string      `json:"updatedAt"`

	// Preview policy. The concurrency cap and the TTL live here rather than on
	// the environment so one repository cannot exhaust the host and starve
	// another, and so both survive a restart.
	PreviewTTLHours         int    `json:"previewTtlHours"`
	PreviewMaxConcurrent    int    `json:"previewMaxConcurrent"`
	PreviewHostnameTemplate string `json:"previewHostnameTemplate"`
	PreviewBaseDomain       string `json:"previewBaseDomain"`
	PreviewOwnerUserID      string `json:"previewOwnerUserId"`
	PreviewCommentEnabled   bool   `json:"previewCommentEnabled"`
}

// Stage is one environment in a pipeline.
type Stage struct {
	ID                  string     `json:"id"`
	PipelineID          string     `json:"pipelineId"`
	StageName           string     `json:"stageName"`
	StageIndex          int        `json:"stageIndex"`
	TargetEnvironmentID string     `json:"targetEnvironmentId"`
	DeployMode          DeployMode `json:"deployMode"`
	Branch              string     `json:"branch"`
	ComposePath         string     `json:"composePath"`
	RequiresApproval    bool       `json:"requiresApproval"`
	CreatedAt           string     `json:"createdAt"`
	UpdatedAt           string     `json:"updatedAt"`
}

// Approval is a pending sign-off request between stages.
type Approval struct {
	ID          string `json:"id"`
	PipelineID  string `json:"pipelineId"`
	StageID     string `json:"stageId"`
	CommitSHA   string `json:"commitSha"`
	RequestedBy string `json:"requestedBy"`
	RequestedAt string `json:"requestedAt"`
	ResolvedBy  string `json:"resolvedBy,omitempty"`
	ResolvedAt  string `json:"resolvedAt,omitempty"`
	Status      string `json:"status"` // pending|approved|rejected|expired
	Note        string `json:"note"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// Promotion is an audit record of a stage deployment.
type Promotion struct {
	ID           string      `json:"id"`
	PipelineID   string      `json:"pipelineId"`
	StageID      string      `json:"stageId"`
	DeploymentID string      `json:"deploymentId"`
	CommitSHA    string      `json:"commitSha"`
	FromStageID  string      `json:"fromStageId"`
	Status       string      `json:"status"` // pending|succeeded|failed|rolled_back|skipped
	TriggerKind  TriggerKind `json:"triggerKind"`
	PRNumber     string      `json:"prNumber"`
	TriggeredBy  string      `json:"triggeredBy"`
	Note         string      `json:"note"`
	StartedAt    string      `json:"startedAt"`
	FinishedAt   string      `json:"finishedAt"`
	CreatedAt    string      `json:"createdAt"`
	UpdatedAt    string      `json:"updatedAt"`

	// Where the promotion deployed and, when it failed, the sentence the
	// operator reads behind the error icon — compose's own message where that
	// was the cause.
	EnvironmentID string `json:"environmentId"`
	StackName     string `json:"stackName"`
	LastError     string `json:"lastError"`
}

// PRPreview is an ephemeral preview environment spawned by a PR.
type PRPreview struct {
	ID                   string `json:"id"`
	PipelineID           string `json:"pipelineId"`
	RepoID               string `json:"repoId"`
	PRNumber             string `json:"prNumber"`
	PRTitle              string `json:"prTitle"`
	SourceBranch         string `json:"sourceBranch"`
	TargetBranch         string `json:"targetBranch"`
	CommitSHA            string `json:"commitSha"`
	PreviewEnvironmentID string `json:"previewEnvironmentId"`
	StackName            string `json:"stackName"`
	Status               string `json:"status"` // active|demolishing|demolished|failed|expired|refused
	URL                  string `json:"url"`
	ExpiresAt            string `json:"expiresAt"`
	TornDownBy           string `json:"tornDownBy"`
	LastError            string `json:"lastError"`
	CommentError         string `json:"commentError"`
	OpenedAt             string `json:"openedAt"`
	ClosedAt             string `json:"closedAt"`
}

// CreatePipelineInput is the request body for creating a pipeline.
type CreatePipelineInput struct {
	Name        string              `json:"name"`
	RepoID      string              `json:"repoId"`
	Description string              `json:"description"`
	TriggerType TriggerType         `json:"triggerType"`
	Stages      []StageInput        `json:"stages"`
	Preview     *PreviewPolicyInput `json:"preview"`
}

// PreviewPolicyInput carries a pipeline's ephemeral-preview policy. Omitting it
// leaves the stored policy untouched on an update and takes the defaults on a
// create.
type PreviewPolicyInput struct {
	TTLHours         *int    `json:"ttlHours"`
	MaxConcurrent    *int    `json:"maxConcurrent"`
	HostnameTemplate *string `json:"hostnameTemplate"`
	BaseDomain       *string `json:"baseDomain"`
	OwnerUserID      *string `json:"ownerUserId"`
	CommentEnabled   *bool   `json:"commentEnabled"`
}

// UpdatePipelineInput is the request body for updating a pipeline.
type UpdatePipelineInput struct {
	Name        *string             `json:"name"`
	Description *string             `json:"description"`
	Enabled     *bool               `json:"enabled"`
	TriggerType *TriggerType        `json:"triggerType"`
	Stages      *[]StageInput       `json:"stages"`
	Preview     *PreviewPolicyInput `json:"preview"`
}

// StageInput describes a stage when creating or updating a pipeline.
type StageInput struct {
	StageName           string     `json:"stageName"`
	StageIndex          int        `json:"stageIndex"`
	TargetEnvironmentID string     `json:"targetEnvironmentId"`
	DeployMode          DeployMode `json:"deployMode"`
	Branch              string     `json:"branch"`
	ComposePath         string     `json:"composePath"`
	RequiresApproval    bool       `json:"requiresApproval"`
}

// PromoteInput is the request body for promoting a deployment.
type PromoteInput struct {
	StageID     string `json:"stageId"`
	CommitSHA   string `json:"commitSha"`
	Note        string `json:"note"`
	TriggeredBy string `json:"triggeredBy"`
	PRNumber    string `json:"prNumber"`
}

// ApprovalInput is the request body for resolving an approval. It carries no
// resolver: that is always the authenticated caller.
type ApprovalInput struct {
	Note string `json:"note"`
}

// PullRequestInput is the inbound webhook payload for PR previews.
type PullRequestInput struct {
	Action       string `json:"action"` // opened|closed|reopened|synchronize
	PRNumber     string `json:"prNumber"`
	PRTitle      string `json:"prTitle"`
	SourceBranch string `json:"sourceBranch"`
	TargetBranch string `json:"targetBranch"`
	CommitSHA    string `json:"commitSha"`
	RepoID       string `json:"repoId"`
}

// Service implements the GitOps pipeline engine.
type Service struct {
	db        *sql.DB
	enc       *encryption.Service
	scheduler *Scheduler
	mu        sync.RWMutex

	// Wired after construction so the preview path can honour
	// environments.manage, attribute a teardown, and reach the environments
	// service without importing it. All three are nil-safe.
	rbacSvc             permissionChecker
	auditLog            *audit.Logger
	resolveEnvironments func() (EnvironmentProvisioner, bool)

	// The deploy path: where checkouts live, how many fetches may run at once,
	// the stacks seam, and the runner that owns every background deploy.
	dataDir       string
	fetchSlots    chan struct{}
	fetcher       composeFetcher
	resolveStacks func() (StackDeployer, bool)
	bgCtx         context.Context //nolint:containedctx // lifecycle root for background deploys, cancelled through bgCancel
	bgCancel      context.CancelFunc
	bg            sync.WaitGroup
	stackLocks    sync.Map

	// The forge PR-state probe's HTTP client, replaceable so a test can point
	// every forge at one local server.
	forgeClient *http.Client
}

// NewService constructs a gitops service. The encryption service is needed
// to read repository credentials when the poller lists remote refs; pass nil
// and polling falls back to unauthenticated remotes only.
func NewService(database *sql.DB, enc *encryption.Service) *Service {
	bgCtx, bgCancel := newBackgroundRoot()
	return &Service{
		db:          database,
		enc:         enc,
		fetchSlots:  make(chan struct{}, maxConcurrentFetches),
		bgCtx:       bgCtx,
		bgCancel:    bgCancel,
		forgeClient: &http.Client{Timeout: probeTimeout},
	}
}

// SetScheduler wires the background scheduler (called from main.go).
func (s *Service) SetScheduler(sched *Scheduler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scheduler = sched
}

// --- Pipeline CRUD ---

// pipelineColumns is the shared projection for a pipeline row, so the two
// readers cannot drift apart when a column is added.
const pipelineColumns = `id, name, repo_id, description, enabled, trigger_type,
	COALESCE(preview_ttl_hours, 72), COALESCE(preview_max_concurrent, 5),
	COALESCE(preview_hostname_template, ''), COALESCE(preview_base_domain, ''),
	COALESCE(preview_owner_user_id, ''), COALESCE(preview_comment_enabled, 1),
	created_at, updated_at`

// scanPipeline reads one pipeline row in the pipelineColumns order.
func scanPipeline(row interface{ Scan(...any) error }) (Pipeline, error) {
	var p Pipeline
	var desc sql.NullString
	var enabled sql.NullBool
	var triggerType sql.NullString
	if err := row.Scan(&p.ID, &p.Name, &p.RepoID, &desc, &enabled, &triggerType,
		&p.PreviewTTLHours, &p.PreviewMaxConcurrent, &p.PreviewHostnameTemplate,
		&p.PreviewBaseDomain, &p.PreviewOwnerUserID, &p.PreviewCommentEnabled,
		&p.CreatedAt, &p.UpdatedAt); err != nil {
		return Pipeline{}, err
	}
	p.Description = desc.String
	p.Enabled = enabled.Bool
	p.TriggerType = TriggerType(orEmpty(triggerType.String, string(TriggerTypeAuto)))
	return p, nil
}

// ListPipelinesCtx returns all pipelines, bounded by ctx.
func (s *Service) ListPipelinesCtx(ctx context.Context) ([]Pipeline, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+pipelineColumns+` FROM gitops_pipelines ORDER BY name ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing gitops pipelines: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("gitops: closing rows failed", "error", cerr)
		}
	}()

	var pipelines []Pipeline
	for rows.Next() {
		p, err := scanPipeline(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning gitops pipeline: %w", err)
		}
		pipelines = append(pipelines, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading rows: %w", err)
	}

	for i := range pipelines {
		stages, err := s.listStagesForPipeline(ctx, pipelines[i].ID)
		if err != nil {
			return nil, err
		}
		pipelines[i].Stages = stages
	}
	if pipelines == nil {
		pipelines = []Pipeline{}
	}
	return pipelines, nil
}

// PipelineByIDCtx returns a single pipeline with its stages, bounded by ctx.
func (s *Service) PipelineByIDCtx(ctx context.Context, id string) (*Pipeline, error) {
	p, err := scanPipeline(s.db.QueryRowContext(ctx,
		`SELECT `+pipelineColumns+` FROM gitops_pipelines WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying gitops pipeline: %w", err)
	}
	stages, err := s.listStagesForPipeline(ctx, id)
	if err != nil {
		return nil, err
	}
	p.Stages = stages
	return &p, nil
}

// CreatePipeline inserts a new pipeline with its stages.
func (s *Service) CreatePipeline(ctx context.Context, userID string, in CreatePipelineInput) (*Pipeline, error) {
	if in.Name == "" {
		return nil, errors.New("pipeline name is required")
	}
	if in.RepoID == "" {
		return nil, errors.New("repo id is required")
	}
	if len(in.Stages) == 0 {
		return nil, errors.New("at least one stage is required")
	}
	if err := s.validateStages(ctx, userID, in.Stages); err != nil {
		return nil, err
	}

	if exists, err := s.repoExists(ctx, in.RepoID); err != nil {
		return nil, err
	} else if !exists {
		return nil, errors.New("repo does not exist")
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	triggerType := in.TriggerType
	if triggerType == "" {
		triggerType = TriggerTypeAuto
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			slog.Warn("gitops: rolling back transaction failed", "error", rerr)
		}
	}()

	policy := defaultPreviewPolicy()
	applyPreviewPolicy(&policy, in.Preview)

	_, err = tx.ExecContext(ctx,
		`INSERT INTO gitops_pipelines (id, name, repo_id, description, enabled, trigger_type,
		        preview_ttl_hours, preview_max_concurrent, preview_hostname_template,
		        preview_base_domain, preview_owner_user_id, preview_comment_enabled,
		        created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.Name, in.RepoID, in.Description, true, string(triggerType),
		policy.PreviewTTLHours, policy.PreviewMaxConcurrent, policy.PreviewHostnameTemplate,
		policy.PreviewBaseDomain, policy.PreviewOwnerUserID, policy.PreviewCommentEnabled,
		now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting gitops pipeline: %w", err)
	}

	for _, stage := range in.Stages {
		stageID := xid.New().String()
		_, err := tx.ExecContext(ctx,
			`INSERT INTO gitops_pipeline_stages (id, pipeline_id, stage_name, stage_index, target_environment_id, deploy_mode, branch, compose_path, requires_approval, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			stageID, id, stage.StageName, stage.StageIndex,
			nullableString(stage.TargetEnvironmentID),
			nullableString(string(stage.DeployMode)),
			nullableString(orEmpty(stage.Branch, "main")),
			stage.ComposePath, stage.RequiresApproval, now, now,
		)
		if err != nil {
			return nil, fmt.Errorf("inserting gitops stage: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing pipeline: %w", err)
	}
	return s.PipelineByIDCtx(ctx, id)
}

// UpdatePipeline replaces a pipeline's metadata and stages.
func (s *Service) UpdatePipeline(ctx context.Context, userID, id string, in UpdatePipelineInput) (*Pipeline, error) {
	existing, err := s.PipelineByIDCtx(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)

	name := existing.Name
	if in.Name != nil {
		name = *in.Name
	}
	desc := existing.Description
	if in.Description != nil {
		desc = *in.Description
	}
	enabled := existing.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	triggerType := existing.TriggerType
	if in.TriggerType != nil {
		triggerType = *in.TriggerType
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			slog.Warn("gitops: rolling back transaction failed", "error", rerr)
		}
	}()

	policy := Pipeline{
		PreviewTTLHours:         existing.PreviewTTLHours,
		PreviewMaxConcurrent:    existing.PreviewMaxConcurrent,
		PreviewHostnameTemplate: existing.PreviewHostnameTemplate,
		PreviewBaseDomain:       existing.PreviewBaseDomain,
		PreviewOwnerUserID:      existing.PreviewOwnerUserID,
		PreviewCommentEnabled:   existing.PreviewCommentEnabled,
	}
	applyPreviewPolicy(&policy, in.Preview)

	_, err = tx.ExecContext(ctx,
		`UPDATE gitops_pipelines SET name = ?, description = ?, enabled = ?, trigger_type = ?,
		        preview_ttl_hours = ?, preview_max_concurrent = ?, preview_hostname_template = ?,
		        preview_base_domain = ?, preview_owner_user_id = ?, preview_comment_enabled = ?,
		        updated_at = ? WHERE id = ?`,
		name, desc, enabled, string(triggerType),
		policy.PreviewTTLHours, policy.PreviewMaxConcurrent, policy.PreviewHostnameTemplate,
		policy.PreviewBaseDomain, policy.PreviewOwnerUserID, policy.PreviewCommentEnabled,
		now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("updating gitops pipeline: %w", err)
	}

	if in.Stages != nil {
		if err := s.validateStages(ctx, userID, *in.Stages); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM gitops_pipeline_stages WHERE pipeline_id = ?`, id); err != nil {
			return nil, fmt.Errorf("clearing old stages: %w", err)
		}
		for _, stage := range *in.Stages {
			stageID := xid.New().String()
			_, err := tx.ExecContext(ctx,
				`INSERT INTO gitops_pipeline_stages (id, pipeline_id, stage_name, stage_index, target_environment_id, deploy_mode, branch, compose_path, requires_approval, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				stageID, id, stage.StageName, stage.StageIndex,
				nullableString(stage.TargetEnvironmentID),
				nullableString(string(stage.DeployMode)),
				nullableString(orEmpty(stage.Branch, "main")),
				stage.ComposePath, stage.RequiresApproval, now, now,
			)
			if err != nil {
				return nil, fmt.Errorf("inserting stage: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing update: %w", err)
	}
	return s.PipelineByIDCtx(ctx, id)
}

// DeletePipeline removes a pipeline and cascades its stages.
func (s *Service) DeletePipeline(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM gitops_pipelines WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting gitops pipeline: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// --- Promotions ---

// Promote, StartPromotion and the deploy they drive live in deploy.go.

// ListPromotions returns recent promotions for a pipeline.
func (s *Service) ListPromotions(ctx context.Context, pipelineID string, page, perPage int) ([]Promotion, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM gitops_promotions WHERE pipeline_id = ?`, pipelineID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting promotions: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, pipeline_id, stage_id, COALESCE(deployment_id, ''), commit_sha,
		        COALESCE(from_stage_id, ''), status, trigger_kind,
		        COALESCE(pr_number, ''), COALESCE(triggered_by, ''), COALESCE(note, ''),
		        started_at, COALESCE(finished_at, ''), created_at, updated_at,
		        COALESCE(environment_id, ''), COALESCE(stack_name, ''), COALESCE(last_error, '')
		 FROM gitops_promotions WHERE pipeline_id = ?
		 ORDER BY started_at DESC LIMIT ? OFFSET ?`,
		pipelineID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying promotions: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("gitops: closing rows failed", "error", cerr)
		}
	}()

	var items []Promotion
	for rows.Next() {
		var p Promotion
		var triggerKind string
		if err := rows.Scan(&p.ID, &p.PipelineID, &p.StageID, &p.DeploymentID, &p.CommitSHA,
			&p.FromStageID, &p.Status, &triggerKind, &p.PRNumber, &p.TriggeredBy, &p.Note,
			&p.StartedAt, &p.FinishedAt, &p.CreatedAt, &p.UpdatedAt,
			&p.EnvironmentID, &p.StackName, &p.LastError); err != nil {
			return nil, 0, fmt.Errorf("scanning promotion: %w", err)
		}
		p.TriggerKind = TriggerKind(triggerKind)
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("reading rows: %w", err)
	}
	if items == nil {
		items = []Promotion{}
	}
	return items, total, nil
}

// --- Approvals ---

// ListApprovals returns pending approval requests.
func (s *Service) ListApprovals(ctx context.Context, status string) ([]Approval, error) {
	if status == "" {
		status = "pending"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, pipeline_id, stage_id, commit_sha, COALESCE(requested_by, ''),
		        requested_at, COALESCE(resolved_by, ''), COALESCE(resolved_at, ''),
		        status, COALESCE(note, ''), created_at, updated_at
		 FROM gitops_approvals WHERE status = ? ORDER BY requested_at DESC LIMIT 100`,
		status,
	)
	if err != nil {
		return nil, fmt.Errorf("listing approvals: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("gitops: closing rows failed", "error", cerr)
		}
	}()

	var items []Approval
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.PipelineID, &a.StageID, &a.CommitSHA, &a.RequestedBy,
			&a.RequestedAt, &a.ResolvedBy, &a.ResolvedAt, &a.Status, &a.Note,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning approval: %w", err)
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading rows: %w", err)
	}
	if items == nil {
		items = []Approval{}
	}
	return items, nil
}

// ResolveApproval sets an approval to approved or rejected.
func (s *Service) ResolveApproval(ctx context.Context, approvalID, action, resolvedBy, note string) (*Approval, error) {
	if action != "approved" && action != "rejected" {
		return nil, errors.New("action must be 'approved' or 'rejected'")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		`UPDATE gitops_approvals SET status = ?, resolved_by = ?, resolved_at = ?, note = ?, updated_at = ?
		 WHERE id = ? AND status = 'pending'`,
		action, resolvedBy, now, note, now, approvalID,
	)
	if err != nil {
		return nil, fmt.Errorf("resolving approval: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return nil, errors.New("approval not found or already resolved")
	}
	var a Approval
	err = s.db.QueryRowContext(ctx,
		`SELECT id, pipeline_id, stage_id, commit_sha, COALESCE(requested_by, ''),
		        requested_at, COALESCE(resolved_by, ''), COALESCE(resolved_at, ''),
		        status, COALESCE(note, ''), created_at, updated_at
		 FROM gitops_approvals WHERE id = ?`, approvalID,
	).Scan(&a.ID, &a.PipelineID, &a.StageID, &a.CommitSHA, &a.RequestedBy,
		&a.RequestedAt, &a.ResolvedBy, &a.ResolvedAt, &a.Status, &a.Note,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("reading approval: %w", err)
	}

	// If approved, auto-promote to the approved stage. The deploy runs on the
	// background runner because this is reached from an HTTP handler.
	if action == "approved" {
		if _, err := s.StartPromotion(ctx, a.PipelineID, PromoteInput{
			StageID:     a.StageID,
			CommitSHA:   a.CommitSHA,
			Note:        "auto-promoted after approval",
			TriggeredBy: resolvedBy,
		}); err != nil {
			slog.Warn("gitops: auto-promote after approval failed", "approval", a.ID, "error", err)
		}
	}
	return &a, nil
}

// --- PR previews ---

// ListPRPreviews returns recent PR previews, newest first.
func (s *Service) ListPRPreviews(ctx context.Context, pipelineID string) ([]PRPreview, error) {
	q := `SELECT ` + previewColumns + ` FROM gitops_pr_previews`
	args := []any{}
	if pipelineID != "" {
		q += ` WHERE pipeline_id = ?`
		args = append(args, pipelineID)
	}
	q += ` ORDER BY opened_at DESC LIMIT 100`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying PR previews: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("gitops: closing rows failed", "error", cerr)
		}
	}()

	items := []PRPreview{}
	for rows.Next() {
		p, err := scanPreview(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning PR preview: %w", err)
		}
		items = append(items, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading PR previews: %w", err)
	}
	return items, nil
}

// --- Internal helpers ---

func (s *Service) listStagesForPipeline(ctx context.Context, pipelineID string) ([]Stage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, pipeline_id, stage_name, stage_index, COALESCE(target_environment_id, ''),
		        COALESCE(deploy_mode, 'auto'), COALESCE(branch, 'main'), COALESCE(compose_path, ''),
		        requires_approval, created_at, updated_at
		 FROM gitops_pipeline_stages WHERE pipeline_id = ? ORDER BY stage_index ASC`,
		pipelineID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing stages: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("gitops: closing rows failed", "error", cerr)
		}
	}()

	var stages []Stage
	for rows.Next() {
		var s Stage
		var deployMode string
		if err := rows.Scan(&s.ID, &s.PipelineID, &s.StageName, &s.StageIndex, &s.TargetEnvironmentID,
			&deployMode, &s.Branch, &s.ComposePath, &s.RequiresApproval, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning stage: %w", err)
		}
		s.DeployMode = DeployMode(deployMode)
		stages = append(stages, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading rows: %w", err)
	}
	if stages == nil {
		stages = []Stage{}
	}
	return stages, nil
}

func (s *Service) repoExists(ctx context.Context, id string) (bool, error) {
	var exists string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM git_repos WHERE id = ? LIMIT 1`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking repo: %w", err)
	}
	return true, nil
}

func (s *Service) findStage(pipeline *Pipeline, stageID string) (*Stage, error) {
	for i := range pipeline.Stages {
		if pipeline.Stages[i].ID == stageID {
			return &pipeline.Stages[i], nil
		}
	}
	return nil, ErrStageNotFound
}

func (s *Service) findPipelineForPRPreview(ctx context.Context, repoID string) (*Pipeline, error) {
	pipelines, err := s.ListPipelinesCtx(ctx)
	if err != nil {
		return nil, err
	}
	for i := range pipelines {
		if pipelines[i].RepoID != repoID {
			continue
		}
		if pipelines[i].TriggerType != TriggerTypePRPreview {
			continue
		}
		if !pipelines[i].Enabled {
			continue
		}
		return &pipelines[i], nil
	}
	return nil, nil
}

func nextStage(pipeline *Pipeline, current *Stage) *Stage {
	for i := range pipeline.Stages {
		if pipeline.Stages[i].StageIndex == current.StageIndex+1 {
			return &pipeline.Stages[i]
		}
	}
	return nil
}

// validateStages checks a pipeline's stages before they are stored. Beyond names
// and index uniqueness it now validates the target environment: the previous
// version waived the requirement entirely for a pr_preview stage and never
// checked that any referenced environment existed, so a holder of gitops.manage
// alone could point a stage at any environment id in the install.
//
// A pr_preview stage may still leave the target empty — its environment is
// created per pull request — but a target it does name is validated like any other.
func (s *Service) validateStages(ctx context.Context, userID string, stages []StageInput) error {
	seen := make(map[int]bool)
	for i, stage := range stages {
		if stage.StageName == "" {
			return fmt.Errorf("stage %d: name is required", i)
		}
		if seen[stage.StageIndex] {
			return fmt.Errorf("stage %d: duplicate index %d", i, stage.StageIndex)
		}
		seen[stage.StageIndex] = true

		if _, err := cleanComposePath(stage.ComposePath); err != nil {
			return fmt.Errorf("stage %d (%s): %w", stage.StageIndex, stage.StageName, err)
		}

		if stage.TargetEnvironmentID == "" {
			if stage.DeployMode == DeployPRPreview {
				continue
			}
			return fmt.Errorf("stage %d (%s): target environment is required", stage.StageIndex, stage.StageName)
		}
		if err := s.validateTargetEnvironment(ctx, userID, stage); err != nil {
			return err
		}
	}
	return nil
}

// validateTargetEnvironment refuses a stage whose environment does not exist, or
// one the caller cannot see. Route-level gitops.manage says nothing about which
// environments a principal may reach, so the check is made per environment here.
func (s *Service) validateTargetEnvironment(ctx context.Context, userID string, stage StageInput) error {
	var exists string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM environments WHERE id = ?`, stage.TargetEnvironmentID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("stage %d (%s): target environment does not exist", stage.StageIndex, stage.StageName)
	}
	if err != nil {
		return fmt.Errorf("stage %d (%s): checking target environment: %w", stage.StageIndex, stage.StageName, err)
	}

	checker := s.rbac()
	if checker == nil || userID == "" {
		return nil
	}
	allowed, err := checker.HasPermission(ctx, userID, stage.TargetEnvironmentID, rbac.PermEnvironmentsView)
	if err != nil {
		return fmt.Errorf("stage %d (%s): checking access to the target environment: %w", stage.StageIndex, stage.StageName, err)
	}
	if !allowed {
		return fmt.Errorf("stage %d (%s): you do not have environments.view for the target environment", stage.StageIndex, stage.StageName)
	}
	return nil
}

// defaultPreviewPolicy is what a pipeline gets when it says nothing about
// previews: three days, five at a time, no DNS, comments on.
func defaultPreviewPolicy() Pipeline {
	return Pipeline{
		PreviewTTLHours:       defaultPreviewTTLHours,
		PreviewMaxConcurrent:  5,
		PreviewCommentEnabled: true,
	}
}

// applyPreviewPolicy overlays the fields the caller actually sent, clamping the
// TTL so no pipeline can hold a host for longer than the cap allows.
func applyPreviewPolicy(target *Pipeline, in *PreviewPolicyInput) {
	if in == nil {
		return
	}
	if in.TTLHours != nil {
		hours := *in.TTLHours
		if hours <= 0 {
			hours = defaultPreviewTTLHours
		}
		if hours > maxPreviewTTLHours {
			hours = maxPreviewTTLHours
		}
		target.PreviewTTLHours = hours
	}
	if in.MaxConcurrent != nil && *in.MaxConcurrent >= 0 {
		target.PreviewMaxConcurrent = *in.MaxConcurrent
	}
	if in.HostnameTemplate != nil {
		target.PreviewHostnameTemplate = *in.HostnameTemplate
	}
	if in.BaseDomain != nil {
		target.PreviewBaseDomain = *in.BaseDomain
	}
	if in.OwnerUserID != nil {
		target.PreviewOwnerUserID = *in.OwnerUserID
	}
	if in.CommentEnabled != nil {
		target.PreviewCommentEnabled = *in.CommentEnabled
	}
}

func orEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func inferredTriggerKind(triggeredBy, prNumber string) TriggerKind {
	if prNumber != "" {
		return TriggerPRPreview
	}
	if triggeredBy == "webhook" {
		return TriggerWebhook
	}
	if triggeredBy != "" {
		return TriggerManual
	}
	return TriggerAuto
}

// Compile-time guard: Service must satisfy the interface used by main.go.
var _ = (*Service)(nil)
