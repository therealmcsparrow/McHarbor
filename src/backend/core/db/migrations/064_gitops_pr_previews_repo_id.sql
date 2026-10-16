-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Originally intended to add repo_id to gitops_pr_previews, but that
-- column is already created inside 063_gitops_pipelines.sql. This
-- migration is a no-op kept only so installs that already recorded
-- it in schema_migrations don't get re-run with a different content
-- hash and skip the no-op silently. Fresh installs will run this
-- safely; upgrading installs will already have the column from 063.
SELECT 1;
