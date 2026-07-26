-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Add repo_id column to gitops_pr_previews (introduced late in 063).
ALTER TABLE gitops_pr_previews ADD COLUMN repo_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_gitops_pr_previews_repo ON gitops_pr_previews(repo_id);
