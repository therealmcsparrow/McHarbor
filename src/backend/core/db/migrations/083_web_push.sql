-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS push_subscriptions (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint_hash    TEXT NOT NULL,
    endpoint         TEXT NOT NULL,
    p256dh           TEXT NOT NULL,
    auth_secret      TEXT NOT NULL,
    label            TEXT NOT NULL DEFAULT '',
    user_agent       TEXT NOT NULL DEFAULT '',
    last_success_at  TEXT,
    first_failure_at TEXT,
    next_attempt_at  TEXT,
    failure_count    INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE(user_id, endpoint_hash)
);
CREATE INDEX IF NOT EXISTS idx_push_subscriptions_user ON push_subscriptions(user_id);
