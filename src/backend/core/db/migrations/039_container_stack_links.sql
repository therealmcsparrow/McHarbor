-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS container_stack_links (
    id TEXT PRIMARY KEY,
    environment_id TEXT NOT NULL DEFAULT '',
    container_id TEXT NOT NULL,
    stack_name TEXT NOT NULL,
    service_name TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(environment_id, container_id)
);

CREATE INDEX IF NOT EXISTS idx_container_stack_links_stack ON container_stack_links(environment_id, stack_name);
