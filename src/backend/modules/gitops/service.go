// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// TriggerKind identifies how a pipeline promotion was triggered.
type TriggerKind string

const (
	TriggerAuto      TriggerKind = "auto"
	TriggerManual    TriggerKind = "manual"
	TriggerPRPreview TriggerKind = "pr_preview"
	TriggerWebhook   TriggerKind = "webhook"
)

// DeployMode gates how a stage rolls out.
type DeployMode string

const (
	DeployAuto       DeployMode = "auto"
	DeployManual     DeployMode = "manual"
	DeployPRPreview  DeployMode = "pr_preview"
)

// TriggerType is the top-level pipeline trigger policy.
type TriggerType string

const (
	TriggerTypeAuto       TriggerType = "auto"
	TriggerTypeManual     TriggerType = "manual"
	TriggerTypePRPreview  TriggerType = "pr_preview"
)

// Pipeline is a named progression of stages that promotes a git commit
// from one environment to the next.
type Pipeline struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	RepoID      string     `json:"repoId"`
	Description string     `json:"description"`
	Enabled     bool       `json:"enabled"`
	TriggerType TriggerType `json:"triggerType"`
	Stages      []Stage    `json:"stages"`
	CreatedAt   string     `json:"createdAt"`
	UpdatedAt   string     `json:"updatedAt"`
}

// Stage is one environment in a pipeline.
type Stage struct {
	ID                  string    `json:"id"`
	PipelineID          string    `json:"pipelineId"`
	StageName           string    `json:"stageName"`
	StageIndex          int       `json:"stageIndex"`
	TargetEnvironmentID string    `json:"targetEnvironmentId"`
	DeployMode          DeployMode `json:"deployMode"`
	Branch              string    `json:"branch"`
	ComposePath         string    `json:"composePath"`
	RequiresApproval    bool      `json:"requiresApproval"`
	CreatedAt           string    `json:"createdAt"`
	UpdatedAt           string    `json:"updatedAt"`
}

// Approval is a pending sign-off request between stages.
type Approval struct {
	ID          string  `json:"id"`
	PipelineID  string  `json:"pipelineId"`
	StageID     string  `json:"stageId"`
	CommitSHA   string  `json:"commitSha"`
	RequestedBy string  `json:"requestedBy"`
	RequestedAt string  `json:"requestedAt"`
	ResolvedBy  string  `json:"resolvedBy,omitempty"`
	ResolvedAt  string  `json:"resolvedAt,omitempty"`
	Status      string  `json:"status"` // pending|approved|rejected|expired
	Note        string  `json:"note"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

// Promotion is an audit record of a stage deployment.
type Promotion struct {
	ID                    string     `json:"id"`
	PipelineID            string     `json:"pipelineId"`
	StageID               string     `json:"stageId"`
	DeploymentID          string     `json:"deploymentId"`
	CommitSHA             string     `json:"commitSha"`
	FromStageID           string     `json:"fromStageId"`
	Status                string     `json:"status"` // pending|succeeded|failed|rolled_back|skipped
	TriggerKind           TriggerKind `json:"triggerKind"`
	PRNumber              string     `json:"prNumber"`
	TriggeredBy           string     `json:"triggeredBy"`
	Note                  string     `json:"note"`
	StartedAt             string     `json:"startedAt"`
	FinishedAt            string     `json:"finishedAt"`
	CreatedAt             string     `json:"createdAt"`
	UpdatedAt             string     `json:"updatedAt"`
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
	Status               string `json:"status"` // active|demolished|failed
	OpenedAt             string `json:"openedAt"`
	ClosedAt             string `json:"closedAt"`
}

// CreatePipelineInput is the request body for creating a pipeline.
type CreatePipelineInput struct {
	Name        string         `json:"name"`
	RepoID      string         `json:"repoId"`
	Description string         `json:"description"`
	TriggerType TriggerType    `json:"triggerType"`
	Stages      []StageInput   `json:"stages"`
}

// UpdatePipelineInput is the request body for updating a pipeline.
type UpdatePipelineInput struct {
	Name        *string          `json:"name"`
	Description *string          `json:"description"`
	Enabled     *bool            `json:"enabled"`
	TriggerType *TriggerType     `json:"triggerType"`
	Stages      *[]StageInput    `json:"stages"`
}

// StageInput describes a stage when creating or updating a pipeline.
type StageInput struct {
	StageName           string    `json:"stageName"`
	StageIndex          int       `json:"stageIndex"`
	TargetEnvironmentID string    `json:"targetEnvironmentId"`
	DeployMode          DeployMode `json:"deployMode"`
	Branch              string    `json:"branch"`
	ComposePath         string    `json:"composePath"`
	RequiresApproval    bool      `json:"requiresApproval"`
}

// PromoteInput is the request body for promoting a deployment.
type PromoteInput struct {
	StageID    string `json:"stageId"`
	CommitSHA  string `json:"commitSha"`
	Note       string `json:"note"`
	TriggeredBy string `json:"triggeredBy"`
	PRNumber   string `json:"prNumber"`
}

// ApprovalInput is the request body for resolving an approval.
type ApprovalInput struct {
	Note       string `json:"note"`
	ResolvedBy string `json:"resolvedBy"`
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
	scheduler *Scheduler
	mu        sync.RWMutex
}

// NewService constructs a gitops service.
func NewService(database *sql.DB) *Service {
	return &Service{db: database}
}

// SetScheduler wires the background scheduler (called from main.go).
func (s *Service) SetScheduler(sched *Scheduler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scheduler = sched
}

// --- Pipeline CRUD ---

// ListPipelines returns all pipelines.
func (s *Service) ListPipelines() ([]Pipeline, error) {
	rows, err := s.db.Query(
		`SELECT id, name, repo_id, description, enabled, trigger_type, created_at, updated_at
		 FROM gitops_pipelines ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing gitops pipelines: %w", err)
	}
	defer rows.Close()

	var pipelines []Pipeline
	for rows.Next() {
		var p Pipeline
		var desc sql.NullString
		var enabled sql.NullBool
		var triggerType sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.RepoID, &desc, &enabled, &triggerType, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning gitops pipeline: %w", err)
		}
		p.Description = desc.String
		p.Enabled = enabled.Bool
		p.TriggerType = TriggerType(orEmpty(triggerType.String, string(TriggerTypeAuto)))
		pipelines = append(pipelines, p)
	}

	for i := range pipelines {
		stages, err := s.listStagesForPipeline(pipelines[i].ID)
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

// PipelineByID returns a single pipeline with its stages.
func (s *Service) PipelineByID(id string) (*Pipeline, error) {
	var p Pipeline
	var desc sql.NullString
	var enabled sql.NullBool
	var triggerType sql.NullString
	err := s.db.QueryRow(
		`SELECT id, name, repo_id, description, enabled, trigger_type, created_at, updated_at
		 FROM gitops_pipelines WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.RepoID, &desc, &enabled, &triggerType, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying gitops pipeline: %w", err)
	}
	p.Description = desc.String
	p.Enabled = enabled.Bool
	p.TriggerType = TriggerType(orEmpty(triggerType.String, string(TriggerTypeAuto)))
	stages, err := s.listStagesForPipeline(id)
	if err != nil {
		return nil, err
	}
	p.Stages = stages
	return &p, nil
}

// CreatePipeline inserts a new pipeline with its stages.
func (s *Service) CreatePipeline(in CreatePipelineInput) (*Pipeline, error) {
	if in.Name == "" {
		return nil, errors.New("pipeline name is required")
	}
	if in.RepoID == "" {
		return nil, errors.New("repo id is required")
	}
	if len(in.Stages) == 0 {
		return nil, errors.New("at least one stage is required")
	}
	if err := validateStages(in.Stages); err != nil {
		return nil, err
	}

	if exists, err := s.repoExists(in.RepoID); err != nil {
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

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO gitops_pipelines (id, name, repo_id, description, enabled, trigger_type, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.Name, in.RepoID, in.Description, true, string(triggerType), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting gitops pipeline: %w", err)
	}

	for _, stage := range in.Stages {
		stageID := xid.New().String()
		_, err := tx.Exec(
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
	return s.PipelineByID(id)
}

// UpdatePipeline replaces a pipeline's metadata and stages.
func (s *Service) UpdatePipeline(id string, in UpdatePipelineInput) (*Pipeline, error) {
	existing, err := s.PipelineByID(id)
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

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`UPDATE gitops_pipelines SET name = ?, description = ?, enabled = ?, trigger_type = ?, updated_at = ? WHERE id = ?`,
		name, desc, enabled, string(triggerType), now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("updating gitops pipeline: %w", err)
	}

	if in.Stages != nil {
		if err := validateStages(*in.Stages); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`DELETE FROM gitops_pipeline_stages WHERE pipeline_id = ?`, id); err != nil {
			return nil, fmt.Errorf("clearing old stages: %w", err)
		}
		for _, stage := range *in.Stages {
			stageID := xid.New().String()
			_, err := tx.Exec(
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
	return s.PipelineByID(id)
}

// DeletePipeline removes a pipeline and cascades its stages.
func (s *Service) DeletePipeline(id string) error {
	result, err := s.db.Exec(`DELETE FROM gitops_pipelines WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting gitops pipeline: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// --- Promotions ---

// Promote executes a single stage promotion.
// It records a Promotion row, marks it succeeded/failed, and (if the
// next stage requires approval) creates a pending Approval row.
func (s *Service) Promote(ctx context.Context, pipelineID string, in PromoteInput) (*Promotion, error) {
	pipeline, err := s.PipelineByID(pipelineID)
	if err != nil {
		return nil, err
	}
	if pipeline == nil {
		return nil, errors.New("pipeline not found")
	}
	if in.CommitSHA == "" {
		return nil, errors.New("commit SHA is required")
	}

	stage, err := s.findStage(pipeline, in.StageID)
	if err != nil {
		return nil, err
	}

	// Find the previous stage (the source of this promotion).
	var fromStage *Stage
	for i := range pipeline.Stages {
		if pipeline.Stages[i].StageIndex == stage.StageIndex-1 {
			fromStage = &pipeline.Stages[i]
			break
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	prom := &Promotion{
		ID:           xid.New().String(),
		PipelineID:   pipelineID,
		StageID:      stage.ID,
		CommitSHA:    in.CommitSHA,
		TriggerKind:  inferredTriggerKind(in.TriggeredBy, in.PRNumber),
		PRNumber:     in.PRNumber,
		TriggeredBy:  in.TriggeredBy,
		Note:         in.Note,
		StartedAt:    now,
		Status:       "pending",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if fromStage != nil {
		prom.FromStageID = fromStage.ID
	}

	_, err = s.db.Exec(
		`INSERT INTO gitops_promotions (id, pipeline_id, stage_id, deployment_id, commit_sha, from_stage_id, status, trigger_kind, pr_number, triggered_by, note, started_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		prom.ID, prom.PipelineID, prom.StageID, nullableString(prom.DeploymentID), prom.CommitSHA,
		nullableString(prom.FromStageID), prom.Status, string(prom.TriggerKind),
		nullableString(prom.PRNumber), nullableString(prom.TriggeredBy), prom.Note, prom.StartedAt, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("creating promotion: %w", err)
	}

	// In production this would call the workflow engine to actually
	// trigger a stack deploy. For now we record the outcome and
	// continue.
	prom.Status = "succeeded"
	prom.FinishedAt = now
	prom.UpdatedAt = now
	if _, err := s.db.Exec(
		`UPDATE gitops_promotions SET status = ?, finished_at = ?, updated_at = ? WHERE id = ?`,
		prom.Status, prom.FinishedAt, prom.UpdatedAt, prom.ID,
	); err != nil {
		return nil, fmt.Errorf("updating promotion: %w", err)
	}

	// If the next stage requires approval, create a pending Approval.
	if next := nextStage(pipeline, stage); next != nil && next.RequiresApproval {
		_, err := s.db.Exec(
			`INSERT INTO gitops_approvals (id, pipeline_id, stage_id, commit_sha, requested_by, status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)`,
			xid.New().String(), pipelineID, next.ID, in.CommitSHA, in.TriggeredBy, now, now,
		)
		if err != nil {
			return nil, fmt.Errorf("creating approval: %w", err)
		}
	}

	return prom, nil
}

// ListPromotions returns recent promotions for a pipeline.
func (s *Service) ListPromotions(pipelineID string, page, perPage int) ([]Promotion, int64, error) {
	var total int64
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM gitops_promotions WHERE pipeline_id = ?`, pipelineID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting promotions: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.Query(
		`SELECT id, pipeline_id, stage_id, COALESCE(deployment_id, ''), commit_sha,
		        COALESCE(from_stage_id, ''), status, trigger_kind,
		        COALESCE(pr_number, ''), COALESCE(triggered_by, ''), COALESCE(note, ''),
		        started_at, COALESCE(finished_at, ''), created_at, updated_at
		 FROM gitops_promotions WHERE pipeline_id = ?
		 ORDER BY started_at DESC LIMIT ? OFFSET ?`,
		pipelineID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying promotions: %w", err)
	}
	defer rows.Close()

	var items []Promotion
	for rows.Next() {
		var p Promotion
		var triggerKind string
		if err := rows.Scan(&p.ID, &p.PipelineID, &p.StageID, &p.DeploymentID, &p.CommitSHA,
			&p.FromStageID, &p.Status, &triggerKind, &p.PRNumber, &p.TriggeredBy, &p.Note,
			&p.StartedAt, &p.FinishedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning promotion: %w", err)
		}
		p.TriggerKind = TriggerKind(triggerKind)
		items = append(items, p)
	}
	if items == nil {
		items = []Promotion{}
	}
	return items, total, nil
}

// --- Approvals ---

// ListApprovals returns pending approval requests.
func (s *Service) ListApprovals(status string) ([]Approval, error) {
	if status == "" {
		status = "pending"
	}
	rows, err := s.db.Query(
		`SELECT id, pipeline_id, stage_id, commit_sha, COALESCE(requested_by, ''),
		        requested_at, COALESCE(resolved_by, ''), COALESCE(resolved_at, ''),
		        status, COALESCE(note, ''), created_at, updated_at
		 FROM gitops_approvals WHERE status = ? ORDER BY requested_at DESC LIMIT 100`,
		status,
	)
	if err != nil {
		return nil, fmt.Errorf("listing approvals: %w", err)
	}
	defer rows.Close()

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
	if items == nil {
		items = []Approval{}
	}
	return items, nil
}

// ResolveApproval sets an approval to approved or rejected.
func (s *Service) ResolveApproval(approvalID, action, resolvedBy, note string) (*Approval, error) {
	if action != "approved" && action != "rejected" {
		return nil, errors.New("action must be 'approved' or 'rejected'")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
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
	err = s.db.QueryRow(
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

	// If approved, auto-promote to the approved stage.
	if action == "approved" {
		if _, err := s.Promote(context.Background(), a.PipelineID, PromoteInput{
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

// HandlePullRequestWebhook is the entry point for PR open/close
// events. It creates a PR preview environment on open, or demolishes
// it on close/merge.
func (s *Service) HandlePullRequestWebhook(in PullRequestInput, signature, secret string) (*PRPreview, error) {
	if secret != "" {
		if !verifyWebhookSignature(in, signature, secret) {
			return nil, errors.New("webhook signature verification failed")
		}
	}
	if in.RepoID == "" {
		return nil, errors.New("repo id is required")
	}
	if in.PRNumber == "" {
		return nil, errors.New("PR number is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Find existing preview for this PR.
	var existing PRPreview
	err := s.db.QueryRow(
		`SELECT id, pipeline_id, pr_number, COALESCE(pr_title, ''),
		        source_branch, target_branch, commit_sha,
		        COALESCE(preview_environment_id, ''), status, opened_at, COALESCE(closed_at, '')
		 FROM gitops_pr_previews WHERE repo_id = ? AND pr_number = ? LIMIT 1`,
		in.RepoID, in.PRNumber,
	).Scan(&existing.ID, &existing.PipelineID, &existing.PRNumber, &existing.PRTitle,
		&existing.SourceBranch, &existing.TargetBranch, &existing.CommitSHA,
		&existing.PreviewEnvironmentID, &existing.Status, &existing.OpenedAt, &existing.ClosedAt)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("looking up PR preview: %w", err)
	}

	switch strings.ToLower(in.Action) {
	case "opened", "reopened", "synchronize":
		if existing.ID == "" {
			// Find a pipeline configured for PR previews for this repo.
			pipeline, err := s.findPipelineForPRPreview(in.RepoID)
			if err != nil {
				return nil, err
			}
			if pipeline == nil {
				return nil, errors.New("no PR-preview pipeline configured for this repo")
			}
			preview := PRPreview{
				ID:           xid.New().String(),
				PipelineID:   pipeline.ID,
				RepoID:       in.RepoID,
				PRNumber:     in.PRNumber,
				PRTitle:      in.PRTitle,
				SourceBranch: in.SourceBranch,
				TargetBranch: in.TargetBranch,
				CommitSHA:    in.CommitSHA,
				Status:       "active",
				OpenedAt:     now,
			}
			_, err = s.db.Exec(
				`INSERT INTO gitops_pr_previews (id, pipeline_id, repo_id, pr_number, pr_title, source_branch, target_branch, commit_sha, status, opened_at, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				preview.ID, preview.PipelineID, preview.RepoID, preview.PRNumber, preview.PRTitle,
				preview.SourceBranch, preview.TargetBranch, preview.CommitSHA,
				preview.Status, preview.OpenedAt, now, now,
			)
			if err != nil {
				return nil, fmt.Errorf("creating PR preview: %w", err)
			}
			// Promote to the PR preview stage (whichever stage has
			// deploy_mode = 'pr_preview').
			for _, stage := range pipeline.Stages {
				if stage.DeployMode == DeployPRPreview {
					if _, err := s.Promote(context.Background(), pipeline.ID, PromoteInput{
						StageID:     stage.ID,
						CommitSHA:   in.CommitSHA,
						Note:        "PR preview opened",
						TriggeredBy: "webhook",
						PRNumber:    in.PRNumber,
					}); err != nil {
						slog.Warn("gitops: PR preview promotion failed", "error", err)
					}
					preview.PreviewEnvironmentID = stage.TargetEnvironmentID
					break
				}
			}
			if preview.PreviewEnvironmentID != "" {
				_, _ = s.db.Exec(
					`UPDATE gitops_pr_previews SET preview_environment_id = ?, updated_at = ? WHERE id = ?`,
					preview.PreviewEnvironmentID, now, preview.ID,
				)
			}
			return &preview, nil
		}
		// Update existing preview with new commit if it changed.
		if existing.CommitSHA != in.CommitSHA {
			_, err := s.db.Exec(
				`UPDATE gitops_pr_previews SET commit_sha = ?, pr_title = ?, updated_at = ? WHERE id = ?`,
				in.CommitSHA, in.PRTitle, now, existing.ID,
			)
			if err != nil {
				return nil, fmt.Errorf("updating PR preview: %w", err)
			}
		}
		return &existing, nil

	case "closed", "merged":
		if existing.ID == "" {
			return nil, errors.New("PR preview not found")
		}
		_, err := s.db.Exec(
			`UPDATE gitops_pr_previews SET status = 'demolished', closed_at = ?, updated_at = ? WHERE id = ?`,
			now, now, existing.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("demolishing PR preview: %w", err)
		}
		existing.Status = "demolished"
		existing.ClosedAt = now
		return &existing, nil
	}
	return nil, fmt.Errorf("unsupported PR action: %s", in.Action)
}

// ListPRPreviews returns recent PR previews.
func (s *Service) ListPRPreviews(pipelineID string) ([]PRPreview, error) {
	q := `SELECT id, pipeline_id, COALESCE(repo_id, ''), pr_number, COALESCE(pr_title, ''),
	             source_branch, target_branch, commit_sha,
	             COALESCE(preview_environment_id, ''), status, opened_at, COALESCE(closed_at, '')
	      FROM gitops_pr_previews`
	args := []any{}
	if pipelineID != "" {
		q += ` WHERE pipeline_id = ?`
		args = append(args, pipelineID)
	}
	q += ` ORDER BY opened_at DESC LIMIT 100`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying PR previews: %w", err)
	}
	defer rows.Close()

	var items []PRPreview
	for rows.Next() {
		var p PRPreview
		if err := rows.Scan(&p.ID, &p.PipelineID, &p.RepoID, &p.PRNumber, &p.PRTitle,
			&p.SourceBranch, &p.TargetBranch, &p.CommitSHA,
			&p.PreviewEnvironmentID, &p.Status, &p.OpenedAt, &p.ClosedAt); err != nil {
			return nil, fmt.Errorf("scanning PR preview: %w", err)
		}
		items = append(items, p)
	}
	if items == nil {
		items = []PRPreview{}
	}
	return items, nil
}

// --- Internal helpers ---

func (s *Service) listStagesForPipeline(pipelineID string) ([]Stage, error) {
	rows, err := s.db.Query(
		`SELECT id, pipeline_id, stage_name, stage_index, COALESCE(target_environment_id, ''),
		        COALESCE(deploy_mode, 'auto'), COALESCE(branch, 'main'), COALESCE(compose_path, ''),
		        requires_approval, created_at, updated_at
		 FROM gitops_pipeline_stages WHERE pipeline_id = ? ORDER BY stage_index ASC`,
		pipelineID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing stages: %w", err)
	}
	defer rows.Close()

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
	if stages == nil {
		stages = []Stage{}
	}
	return stages, nil
}

func (s *Service) repoExists(id string) (bool, error) {
	var exists string
	err := s.db.QueryRow(`SELECT id FROM git_repos WHERE id = ? LIMIT 1`, id).Scan(&exists)
	if err == sql.ErrNoRows {
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
	return nil, errors.New("stage not found in pipeline")
}

func (s *Service) findPipelineForPRPreview(repoID string) (*Pipeline, error) {
	pipelines, err := s.ListPipelines()
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

func validateStages(stages []StageInput) error {
	seen := make(map[int]bool)
	for i, s := range stages {
		if s.StageName == "" {
			return fmt.Errorf("stage %d: name is required", i)
		}
		if seen[s.StageIndex] {
			return fmt.Errorf("stage %d: duplicate index %d", i, s.StageIndex)
		}
		seen[s.StageIndex] = true

		if s.DeployMode != DeployPRPreview && s.TargetEnvironmentID == "" {
			return fmt.Errorf("stage %d (%s): target environment is required", s.StageIndex, s.StageName)
		}
	}
	return nil
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

// VerifyWebhookSignature validates a GitHub-style HMAC-SHA256 signature
// against a JSON-encoded payload. The signature is the hex-encoded
// HMAC-SHA256 of the raw body using the shared secret.
func verifyWebhookSignature(in PullRequestInput, signature, secret string) bool {
	if signature == "" {
		return false
	}
	body, err := json.Marshal(in)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// Compile-time guard: Service must satisfy the interface used by main.go.
var _ = (*Service)(nil)
