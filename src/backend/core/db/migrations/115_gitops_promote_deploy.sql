-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE gitops_promotions ADD COLUMN environment_id TEXT DEFAULT '';
ALTER TABLE gitops_promotions ADD COLUMN stack_name TEXT DEFAULT '';
ALTER TABLE gitops_promotions ADD COLUMN last_error TEXT DEFAULT '';

ALTER TABLE gitops_pr_previews ADD COLUMN stack_name TEXT DEFAULT '';
ALTER TABLE gitops_pr_previews ADD COLUMN last_probed_at TEXT;

ALTER TABLE stacks ADD COLUMN managed_by TEXT DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_stacks_managed_by ON stacks(managed_by);
CREATE INDEX IF NOT EXISTS idx_gitops_pr_previews_probed ON gitops_pr_previews(last_probed_at);
