-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS workflow_trigger_fires (
    id           TEXT PRIMARY KEY,
    fire_id      TEXT NOT NULL UNIQUE,
    origin_node  TEXT NOT NULL DEFAULT '',
    fire_source  TEXT NOT NULL DEFAULT '',
    action_key   TEXT NOT NULL DEFAULT '',
    expires_at   TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workflow_trigger_fires_expires ON workflow_trigger_fires(expires_at);
