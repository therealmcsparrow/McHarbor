-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS cluster_instances (
    id              TEXT PRIMARY KEY,
    hostname        TEXT NOT NULL DEFAULT '',
    version         TEXT NOT NULL DEFAULT '',
    build           TEXT NOT NULL DEFAULT '',
    ha_role         TEXT NOT NULL DEFAULT 'peer',
    state           TEXT NOT NULL DEFAULT 'running',
    advertise_url   TEXT NOT NULL DEFAULT '',
    agents          INTEGER NOT NULL DEFAULT 0,
    started_at      TEXT NOT NULL,
    heartbeat_at    TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_cluster_instances_heartbeat ON cluster_instances(heartbeat_at);

CREATE TABLE IF NOT EXISTS cluster_state (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    value       TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
