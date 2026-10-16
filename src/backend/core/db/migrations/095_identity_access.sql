-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE scim_sync_log ADD COLUMN users_seen INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scim_sync_log ADD COLUMN users_linked INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scim_sync_log ADD COLUMN users_unmatched INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scim_sync_log ADD COLUMN users_disabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scim_sync_log ADD COLUMN users_skipped INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scim_sync_log ADD COLUMN groups_seen INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scim_sync_log ADD COLUMN dry_run INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scim_sync_log ADD COLUMN run_trigger TEXT NOT NULL DEFAULT 'schedule';
ALTER TABLE scim_sync_log ADD COLUMN actor_id TEXT;
ALTER TABLE scim_sync_log ADD COLUMN item_errors TEXT NOT NULL DEFAULT '';

ALTER TABLE scim_endpoints ADD COLUMN token_rotated_at TEXT;

CREATE INDEX IF NOT EXISTS idx_scim_sync_log_dry_run ON scim_sync_log(endpoint_id, dry_run, started_at);

CREATE TABLE IF NOT EXISTS identity_group_claims (
    id           TEXT PRIMARY KEY,
    provider_id  TEXT NOT NULL,
    user_id      TEXT NOT NULL,
    groups_json  TEXT NOT NULL DEFAULT '[]',
    seen_at      TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE (provider_id, user_id),
    FOREIGN KEY (provider_id) REFERENCES identity_providers(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_identity_group_claims_provider ON identity_group_claims(provider_id);
CREATE INDEX IF NOT EXISTS idx_identity_group_claims_user ON identity_group_claims(user_id);
