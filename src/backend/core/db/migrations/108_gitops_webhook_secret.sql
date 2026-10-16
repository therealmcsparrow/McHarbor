-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Step 0 of 07d: the GitOps pull-request webhook is mounted public and had no
-- usable authentication. A per-repository secret is the gate: it scopes and
-- rotates per repository, and a repository with no secret has its webhook
-- refused. The value is stored encrypted by encryption.Service, never in
-- plaintext, and is never returned by any API after generation.

ALTER TABLE git_repos ADD COLUMN webhook_secret TEXT DEFAULT '';

-- Concurrent deliveries for one pull request could previously create duplicate
-- preview rows, because the lookup relied on LIMIT 1 rather than a constraint.
-- Collapse any duplicates that already exist, keeping the oldest row, then make
-- the pair unique.
DELETE FROM gitops_pr_previews
WHERE id NOT IN (
    SELECT keep.id FROM (
        SELECT MIN(id) AS id
        FROM gitops_pr_previews
        GROUP BY repo_id, pr_number
    ) AS keep
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gitops_pr_previews_repo_pr
    ON gitops_pr_previews(repo_id, pr_number);
