-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- GitOps pipelines: a named pipeline that maps a source repo + branch to a series of
-- target environments (dev → staging → prod). Each pipeline has a deploy mode
-- (auto-on-push, manual-approval, or PR-preview) per stage.

CREATE TABLE IF NOT EXISTS gitops_pipelines (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    repo_id TEXT NOT NULL REFERENCES git_repos(id) ON DELETE CASCADE,
    description TEXT DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    trigger_type TEXT NOT NULL DEFAULT 'auto'
        CHECK(trigger_type IN ('auto','manual','pr_preview')),
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_gitops_pipelines_repo ON gitops_pipelines(repo_id);

-- Pipeline stages: the ordered progression of environments for a pipeline.
-- stage_index 0 = the first stage (typically dev), increasing order.
-- deploy_mode: 'auto' (deploy on push), 'manual' (deploy only on click),
-- 'pr_preview' (one ephemeral env per PR, torn down on PR close).
CREATE TABLE IF NOT EXISTS gitops_pipeline_stages (
    id TEXT PRIMARY KEY,
    pipeline_id TEXT NOT NULL REFERENCES gitops_pipelines(id) ON DELETE CASCADE,
    stage_name TEXT NOT NULL,
    stage_index INTEGER NOT NULL,
    target_environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    deploy_mode TEXT NOT NULL DEFAULT 'auto'
        CHECK(deploy_mode IN ('auto','manual','pr_preview')),
    branch TEXT NOT NULL DEFAULT 'main',
    compose_path TEXT DEFAULT '',
    requires_approval INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_gitops_stages_pipeline ON gitops_pipeline_stages(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_gitops_stages_env ON gitops_pipeline_stages(target_environment_id);

-- A pending approval when a stage is gated. Created when a stage deploy
-- finishes successfully and requires_approval is true on the next stage.
CREATE TABLE IF NOT EXISTS gitops_approvals (
    id TEXT PRIMARY KEY,
    pipeline_id TEXT NOT NULL REFERENCES gitops_pipelines(id) ON DELETE CASCADE,
    stage_id TEXT NOT NULL REFERENCES gitops_pipeline_stages(id) ON DELETE CASCADE,
    commit_sha TEXT NOT NULL,
    requested_by TEXT,
    requested_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    resolved_by TEXT,
    resolved_at TEXT,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK(status IN ('pending','approved','rejected','expired')),
    note TEXT DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_gitops_approvals_pipeline ON gitops_approvals(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_gitops_approvals_status ON gitops_approvals(status);

-- Audit trail: every state change in a pipeline. Distinct from git_deployments
-- which tracks a single (repo, commit) deployment. Pipeline promotions
-- link deployments to stages.
CREATE TABLE IF NOT EXISTS gitops_promotions (
    id TEXT PRIMARY KEY,
    pipeline_id TEXT NOT NULL REFERENCES gitops_pipelines(id) ON DELETE CASCADE,
    stage_id TEXT NOT NULL REFERENCES gitops_pipeline_stages(id) ON DELETE CASCADE,
    deployment_id TEXT REFERENCES git_deployments(id) ON DELETE SET NULL,
    commit_sha TEXT NOT NULL,
    from_stage_id TEXT REFERENCES gitops_pipeline_stages(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK(status IN ('pending','succeeded','failed','rolled_back','skipped')),
    trigger_kind TEXT NOT NULL DEFAULT 'auto'
        CHECK(trigger_kind IN ('auto','manual','pr_preview','webhook')),
    pr_number TEXT,
    triggered_by TEXT,
    note TEXT DEFAULT '',
    started_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    finished_at TEXT,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_gitops_promotions_pipeline ON gitops_promotions(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_gitops_promotions_stage ON gitops_promotions(stage_id);
CREATE INDEX IF NOT EXISTS idx_gitops_promotions_status ON gitops_promotions(status);

-- PR preview environments: ephemeral target environments spawned when a PR
-- is opened. torn down when the PR is closed/merged.
CREATE TABLE IF NOT EXISTS gitops_pr_previews (
    id TEXT PRIMARY KEY,
    pipeline_id TEXT NOT NULL REFERENCES gitops_pipelines(id) ON DELETE CASCADE,
    repo_id TEXT NOT NULL DEFAULT '',
    pr_number TEXT NOT NULL,
    pr_title TEXT DEFAULT '',
    source_branch TEXT NOT NULL,
    target_branch TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    preview_environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'active'
        CHECK(status IN ('active','demolished','failed')),
    opened_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    closed_at TEXT,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_gitops_pr_previews_pipeline ON gitops_pr_previews(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_gitops_pr_previews_status ON gitops_pr_previews(status);
CREATE INDEX IF NOT EXISTS idx_gitops_pr_previews_repo ON gitops_pr_previews(repo_id);

-- Last-known commit seen by a pipeline. Used by the poller to detect new
-- commits and trigger auto-promotion.
ALTER TABLE git_repos ADD COLUMN last_known_commit TEXT DEFAULT '';
