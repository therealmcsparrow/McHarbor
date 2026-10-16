// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

// seedDeployPipeline creates a repository, a target environment and a two-stage
// pipeline whose second stage needs approval — the shape that shows whether an
// approval is requested only after a real deploy.
func seedDeployPipeline(t *testing.T, database *sql.DB, withTarget bool) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, database,
		`INSERT INTO git_repos (id, name, url, branch, auth_type, created_at, updated_at)
		 VALUES ('repo-d', 'repo-d', 'https://github.com/acme/app', 'main', 'none', ?, ?)`, now, now)
	mustExec(t, database,
		`INSERT INTO environments (id, name, connection_type, is_default, is_active, created_at, updated_at)
		 VALUES ('env-staging', 'staging', 'socket', 0, 1, ?, ?)`, now, now)
	mustExec(t, database,
		`INSERT INTO gitops_pipelines (id, name, repo_id, enabled, trigger_type, created_at, updated_at)
		 VALUES ('pipe-d', 'Web App', 'repo-d', 1, 'manual', ?, ?)`, now, now)
	target := any("env-staging")
	if !withTarget {
		target = nil
	}
	mustExec(t, database,
		`INSERT INTO gitops_pipeline_stages (id, pipeline_id, stage_name, stage_index, target_environment_id, deploy_mode, branch, compose_path, requires_approval, created_at, updated_at)
		 VALUES ('stage-1', 'pipe-d', 'staging', 0, ?, 'manual', 'main', 'deploy/compose.yml', 0, ?, ?),
		        ('stage-2', 'pipe-d', 'production', 1, NULL, 'manual', 'main', '', 1, ?, ?)`,
		target, now, now, now, now)
}

func mustExec(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

func promotionRow(t *testing.T, database *sql.DB, id string) (status, lastError, stack, env string) {
	t.Helper()
	if err := database.QueryRow(
		`SELECT status, COALESCE(last_error, ''), COALESCE(stack_name, ''), COALESCE(environment_id, '') FROM gitops_promotions WHERE id = ?`, id,
	).Scan(&status, &lastError, &stack, &env); err != nil {
		t.Fatalf("read promotion: %v", err)
	}
	return
}

const deploySHA = "0123456789abcdef0123456789abcdef01234567"

// TestPromoteRecordsSucceededOnlyWhenTheDeploySucceeds is the stub's replacement:
// the row says 'succeeded' because the stack came up, the compose file read at the
// promoted commit is what was deployed, and the next stage's approval follows.
func TestPromoteRecordsSucceededOnlyWhenTheDeploySucceeds(t *testing.T) {
	database := newTestDB(t)
	seedDeployPipeline(t, database, true)
	svc, _ := newPreviewService(t, database, allowAll{})
	var asked composeRequest
	svc.setComposeFetcher(func(_ context.Context, req composeRequest) (string, error) {
		asked = req
		return "services: {web: {image: nginx}}", nil
	})

	prom, err := svc.Promote(context.Background(), "pipe-d", PromoteInput{StageID: "stage-1", CommitSHA: deploySHA, TriggeredBy: "user-1"})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	status, lastError, stack, env := promotionRow(t, database, prom.ID)
	if status != promotionSucceeded || lastError != "" {
		t.Fatalf("status=%q last_error=%q, want succeeded with no error", status, lastError)
	}
	if env != "env-staging" || stack != "gitops-web-app-staging" {
		t.Fatalf("recorded env=%q stack=%q", env, stack)
	}
	if asked.Commit != deploySHA || asked.ComposePath != "deploy/compose.yml" || asked.RepoID != "repo-d" {
		t.Fatalf("fetch asked for %+v", asked)
	}
	deploys, _, _ := deployerOf(t, svc).snapshot()
	if len(deploys) != 1 || deploys[0].envID != "env-staging" || deploys[0].compose != "services: {web: {image: nginx}}" ||
		deploys[0].owner != "gitops:pipe-d:staging" {
		t.Fatalf("deploys = %+v", deploys)
	}
	if n := countRows(t, database, `SELECT COUNT(*) FROM gitops_approvals WHERE stage_id = 'stage-2' AND status = 'pending'`); n != 1 {
		t.Fatalf("approvals for the next stage = %d, want 1", n)
	}
}

// TestPromoteRecordsFailedWithTheDeployMessage: a compose failure is recorded as
// 'failed' with compose's own sentence, and no approval is requested for a
// deploy that never happened.
func TestPromoteRecordsFailedWithTheDeployMessage(t *testing.T) {
	database := newTestDB(t)
	seedDeployPipeline(t, database, true)
	svc, _ := newPreviewService(t, database, allowAll{})
	deployerOf(t, svc).deployErr = errors.New("docker compose up failed: pull access denied for acme/nope")

	prom, err := svc.Promote(context.Background(), "pipe-d", PromoteInput{StageID: "stage-1", CommitSHA: deploySHA})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	status, lastError, _, _ := promotionRow(t, database, prom.ID)
	if status != promotionFailed {
		t.Fatalf("status = %q, want failed", status)
	}
	if !strings.Contains(lastError, "pull access denied for acme/nope") {
		t.Fatalf("last_error does not carry the compose message: %q", lastError)
	}
	if n := countRows(t, database, `SELECT COUNT(*) FROM gitops_approvals`); n != 0 {
		t.Fatalf("an approval was requested for a failed deploy (%d rows)", n)
	}
}

// TestPromoteRecordsAFetchFailure: a repository that cannot be fetched fails the
// promotion with the fetch's reason and never reaches the stacks seam.
func TestPromoteRecordsAFetchFailure(t *testing.T) {
	database := newTestDB(t)
	seedDeployPipeline(t, database, true)
	svc, _ := newPreviewService(t, database, allowAll{})
	svc.setComposeFetcher(func(context.Context, composeRequest) (string, error) {
		return "", errors.New("compose file deploy/compose.yml was not found at commit 01234567")
	})

	prom, err := svc.Promote(context.Background(), "pipe-d", PromoteInput{StageID: "stage-1", CommitSHA: deploySHA})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	status, lastError, _, _ := promotionRow(t, database, prom.ID)
	if status != promotionFailed || !strings.Contains(lastError, "was not found at commit") {
		t.Fatalf("status=%q last_error=%q", status, lastError)
	}
	if deploys, _, _ := deployerOf(t, svc).snapshot(); len(deploys) != 0 {
		t.Fatalf("the stacks seam was called after a failed fetch: %+v", deploys)
	}
}

// TestPromoteWithoutATargetFails: a stage with nowhere to deploy is recorded as a
// failure the operator can read, not as a success that deployed nothing.
func TestPromoteWithoutATargetFails(t *testing.T) {
	database := newTestDB(t)
	seedDeployPipeline(t, database, false)
	svc, _ := newPreviewService(t, database, allowAll{})

	prom, err := svc.Promote(context.Background(), "pipe-d", PromoteInput{StageID: "stage-1", CommitSHA: deploySHA})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	status, lastError, _, _ := promotionRow(t, database, prom.ID)
	if status != promotionFailed || !strings.Contains(lastError, "no target environment") {
		t.Fatalf("status=%q last_error=%q", status, lastError)
	}
}

// TestPromoteRefusesUnknownPipelineAndStage keeps the two 404s the handler maps.
func TestPromoteRefusesUnknownPipelineAndStage(t *testing.T) {
	database := newTestDB(t)
	seedDeployPipeline(t, database, true)
	svc, _ := newPreviewService(t, database, allowAll{})

	if _, err := svc.StartPromotion(context.Background(), "nope", PromoteInput{StageID: "stage-1", CommitSHA: deploySHA}); !errors.Is(err, ErrPipelineNotFound) {
		t.Fatalf("unknown pipeline: err = %v", err)
	}
	if _, err := svc.StartPromotion(context.Background(), "pipe-d", PromoteInput{StageID: "nope", CommitSHA: deploySHA}); !errors.Is(err, ErrStageNotFound) {
		t.Fatalf("unknown stage: err = %v", err)
	}
}

// TestStartPromotionReturnsPendingAndRecordsTheOutcome: the HTTP form answers at
// once and the background runner records what really happened.
func TestStartPromotionReturnsPendingAndRecordsTheOutcome(t *testing.T) {
	database := newTestDB(t)
	seedDeployPipeline(t, database, true)
	svc, _ := newPreviewService(t, database, allowAll{})
	release := make(chan struct{})
	svc.setComposeFetcher(func(context.Context, composeRequest) (string, error) {
		<-release
		return "services: {}", nil
	})

	prom, err := svc.StartPromotion(context.Background(), "pipe-d", PromoteInput{StageID: "stage-1", CommitSHA: deploySHA})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if prom.Status != promotionPending {
		t.Fatalf("returned status = %q, want pending", prom.Status)
	}
	close(release)
	svc.waitBackground()
	if status, _, _, _ := promotionRow(t, database, prom.ID); status != promotionSucceeded {
		t.Fatalf("recorded status = %q, want succeeded", status)
	}
}

// TestInterruptedPromotionIsRecordedFailed: a promotion whose runner died with the
// process does not read 'pending' forever.
func TestInterruptedPromotionIsRecordedFailed(t *testing.T) {
	database := newTestDB(t)
	seedDeployPipeline(t, database, true)
	svc, _ := newPreviewService(t, database, allowAll{})
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	fresh := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, database,
		`INSERT INTO gitops_promotions (id, pipeline_id, stage_id, commit_sha, status, trigger_kind, started_at, created_at, updated_at)
		 VALUES ('stale', 'pipe-d', 'stage-1', 'x', 'pending', 'manual', ?, ?, ?),
		        ('live', 'pipe-d', 'stage-1', 'y', 'pending', 'manual', ?, ?, ?)`,
		old, old, old, fresh, fresh, fresh)

	if err := svc.ReapPreviews(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if status, lastError, _, _ := promotionRow(t, database, "stale"); status != promotionFailed || !strings.Contains(lastError, "interrupted") {
		t.Fatalf("stale: status=%q last_error=%q", status, lastError)
	}
	if status, _, _, _ := promotionRow(t, database, "live"); status != promotionPending {
		t.Fatalf("a running promotion was swept: status=%q", status)
	}
}

// TestPreviewDeploysIntoItsOwnEnvironment: the preview's stack lands in the
// preview's environment, owned by the preview, at the pull request's head.
func TestPreviewDeploysIntoItsOwnEnvironment(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, _ := newPreviewService(t, database, allowAll{})

	preview := openPreview(t, svc, "repo-1", "21", "feature/x")
	svc.waitBackground()

	deploys, _, _ := deployerOf(t, svc).snapshot()
	if len(deploys) != 1 {
		t.Fatalf("deploys = %+v, want one", deploys)
	}
	d := deploys[0]
	if d.envID != preview.PreviewEnvironmentID || d.owner != previewStackOwner(preview.ID) || d.name != "preview-pipe-1-pr-21" {
		t.Fatalf("deploy = %+v, preview env %q", d, preview.PreviewEnvironmentID)
	}
	if status, lastError, _ := previewStatus(t, database, preview.ID); status != previewActive {
		t.Fatalf("status=%q last_error=%q", status, lastError)
	}
}

// TestFailedPreviewDeployFailsThePreview: the promotion's message reaches the
// preview row, where StatusError shows it.
func TestFailedPreviewDeployFailsThePreview(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, _ := newPreviewService(t, database, allowAll{})
	deployerOf(t, svc).deployErr = errors.New("docker compose up failed: port 80 is already allocated")

	preview := openPreview(t, svc, "repo-1", "22", "feature/y")
	svc.waitBackground()

	status, lastError, _ := previewStatus(t, database, preview.ID)
	if status != previewFailed || !strings.Contains(lastError, "port 80 is already allocated") {
		t.Fatalf("status=%q last_error=%q", status, lastError)
	}
}

// TestTeardownRemovesTheStackBeforeTheEnvironment: stacks.environment_id cascades,
// so deleting the environment first would orphan running containers.
func TestTeardownRemovesTheStackBeforeTheEnvironment(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, _ := newPreviewService(t, database, allowAll{})

	preview := openPreview(t, svc, "repo-1", "23", "feature/z")
	svc.waitBackground()
	if err := svc.TeardownPreview(context.Background(), preview.ID, "pr-closed", "", previewDemolished); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	_, removes, envPresent := deployerOf(t, svc).snapshot()
	if len(removes) != 1 || removes[0] != "preview-pipe-1-pr-23" {
		t.Fatalf("removes = %v", removes)
	}
	if !envPresent[0] {
		t.Fatal("the environment was already gone when the stack was removed")
	}
	if n := countRows(t, database, `SELECT COUNT(*) FROM environments WHERE id = ?`, preview.PreviewEnvironmentID); n != 0 {
		t.Fatal("the environment survived a successful teardown")
	}
}

// TestFailedStackRemovalKeepsTheEnvironment: the environment is never deleted over
// running containers; the row stays 'demolishing' for the next tick, with why.
func TestFailedStackRemovalKeepsTheEnvironment(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, _ := newPreviewService(t, database, allowAll{})

	preview := openPreview(t, svc, "repo-1", "24", "feature/w")
	svc.waitBackground()
	deployerOf(t, svc).removeErr = errors.New("docker compose down failed: daemon unreachable")

	if err := svc.TeardownPreview(context.Background(), preview.ID, "pr-closed", "", previewDemolished); err == nil {
		t.Fatal("teardown reported success although the stack is still running")
	}
	if n := countRows(t, database, `SELECT COUNT(*) FROM environments WHERE id = ?`, preview.PreviewEnvironmentID); n != 1 {
		t.Fatal("the environment was deleted while its stack could not be removed")
	}
	status, lastError, _ := previewStatus(t, database, preview.ID)
	if status != previewDemolishing || !strings.Contains(lastError, "daemon unreachable") {
		t.Fatalf("status=%q last_error=%q", status, lastError)
	}
}
