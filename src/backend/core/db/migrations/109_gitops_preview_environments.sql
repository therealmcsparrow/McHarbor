-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Ephemeral preview environments (07d).
--
-- Three things this migration establishes, in order of how badly their absence
-- would hurt:
--
-- 1. Provenance on environments. Without it a preview environment is
--    indistinguishable from production, and a reaper working from
--    gitops_pr_previews.preview_environment_id alone would eventually delete
--    an environment an operator merely pointed a stage at. The reaper refuses
--    to touch any row where ephemeral = 0.
-- 2. A persisted expiry on the preview. The GitOps scheduler keeps no state
--    across a restart, so the reaper can only be correct if expiry is a column.
-- 3. Quotas on the pipeline, not the environment, so one repository cannot
--    exhaust the host and starve another.

-- ─── Provenance and expiry on environments ───────────────────────────
ALTER TABLE environments ADD COLUMN ephemeral INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN preview_id TEXT DEFAULT '';
ALTER TABLE environments ADD COLUMN expires_at TEXT;

CREATE INDEX IF NOT EXISTS idx_environments_ephemeral ON environments(ephemeral);
CREATE INDEX IF NOT EXISTS idx_environments_preview ON environments(preview_id);

-- ─── Preview policy on the pipeline ──────────────────────────────────
ALTER TABLE gitops_pipelines ADD COLUMN preview_ttl_hours INTEGER NOT NULL DEFAULT 72;
ALTER TABLE gitops_pipelines ADD COLUMN preview_max_concurrent INTEGER NOT NULL DEFAULT 5;
ALTER TABLE gitops_pipelines ADD COLUMN preview_hostname_template TEXT DEFAULT '';
ALTER TABLE gitops_pipelines ADD COLUMN preview_base_domain TEXT DEFAULT '';
ALTER TABLE gitops_pipelines ADD COLUMN preview_owner_user_id TEXT DEFAULT '';
ALTER TABLE gitops_pipelines ADD COLUMN preview_comment_enabled INTEGER NOT NULL DEFAULT 1;

-- ─── gitops_pr_previews rebuild ──────────────────────────────────────
-- The status CHECK from 063 allowed only ('active','demolished','failed') and
-- SQLite cannot alter a CHECK, so the table is recreated once here with the two
-- states the lifecycle needs: 'demolishing' (teardown started, finish it on the
-- next tick after a restart) and 'expired' (the TTL ran out rather than the PR
-- closing). Adding them separately would mean doing this twice.

ALTER TABLE gitops_pr_previews RENAME TO gitops_pr_previews_old;

CREATE TABLE gitops_pr_previews (
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
        CHECK(status IN ('active','demolishing','demolished','failed','expired','refused')),
    url TEXT DEFAULT '',
    expires_at TEXT,
    demolish_started_at TEXT,
    torn_down_by TEXT DEFAULT '',
    last_error TEXT DEFAULT '',
    comment_error TEXT DEFAULT '',
    opened_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    closed_at TEXT,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

INSERT INTO gitops_pr_previews (
    id, pipeline_id, repo_id, pr_number, pr_title, source_branch, target_branch,
    commit_sha, preview_environment_id, status, opened_at, closed_at, created_at, updated_at
)
SELECT id, pipeline_id, repo_id, pr_number, pr_title, source_branch, target_branch,
       commit_sha, preview_environment_id, status, opened_at, closed_at, created_at, updated_at
FROM gitops_pr_previews_old;

DROP TABLE gitops_pr_previews_old;

CREATE INDEX IF NOT EXISTS idx_gitops_pr_previews_pipeline ON gitops_pr_previews(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_gitops_pr_previews_status ON gitops_pr_previews(status);
CREATE INDEX IF NOT EXISTS idx_gitops_pr_previews_repo ON gitops_pr_previews(repo_id);
CREATE INDEX IF NOT EXISTS idx_gitops_pr_previews_expires ON gitops_pr_previews(expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gitops_pr_previews_repo_pr
    ON gitops_pr_previews(repo_id, pr_number);
