-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS device_codes (
    id                  TEXT PRIMARY KEY,
    device_code_hash    TEXT NOT NULL UNIQUE,
    user_code           TEXT NOT NULL UNIQUE,
    status              TEXT NOT NULL DEFAULT 'pending',
    requested_host      TEXT NOT NULL DEFAULT '',
    requested_os_user   TEXT NOT NULL DEFAULT '',
    approved_by_user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    key_name            TEXT NOT NULL DEFAULT '',
    role_ids            TEXT NOT NULL DEFAULT '[]',
    scopes              TEXT NOT NULL DEFAULT '[]',
    environment_ids     TEXT NOT NULL DEFAULT '[]',
    key_expires_at      TEXT,
    api_key_id          TEXT,
    last_polled_at      TEXT,
    expires_at          TEXT NOT NULL,
    created_at          TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at          TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_device_codes_expires ON device_codes(expires_at);
CREATE INDEX IF NOT EXISTS idx_device_codes_approver ON device_codes(approved_by_user_id);
