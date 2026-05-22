-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS workflow_link_messages (
    id          TEXT PRIMARY KEY,
    workflow_id TEXT NOT NULL,
    node_id     TEXT NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    msg         TEXT DEFAULT '{}',
    updated_at  TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_wlm_workflow_node ON workflow_link_messages(workflow_id, node_id);
