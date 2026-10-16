-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS container_debug_sessions (
    id TEXT PRIMARY KEY,
    environment_id TEXT NOT NULL DEFAULT '',
    container_id TEXT NOT NULL,
    container_name TEXT NOT NULL DEFAULT '',
    user_id TEXT NOT NULL DEFAULT '',
    api_key_id TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL,
    arch TEXT NOT NULL,
    busybox_digest TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    uploaded INTEGER NOT NULL DEFAULT 1,
    expires_at TEXT NOT NULL,
    removed_at TEXT,
    removal_trigger TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_container_debug_sessions_container ON container_debug_sessions(environment_id, container_id, removed_at);
CREATE INDEX IF NOT EXISTS idx_container_debug_sessions_open ON container_debug_sessions(removed_at, expires_at);
CREATE INDEX IF NOT EXISTS idx_container_debug_sessions_user ON container_debug_sessions(user_id);
