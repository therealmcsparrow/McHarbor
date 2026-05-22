-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Add auto_import_groups flag to identity providers
ALTER TABLE identity_providers ADD COLUMN auto_import_groups INTEGER NOT NULL DEFAULT 0;

-- Add source tracking to group_members (manual vs oidc-synced)
ALTER TABLE group_members ADD COLUMN source TEXT NOT NULL DEFAULT 'manual';
