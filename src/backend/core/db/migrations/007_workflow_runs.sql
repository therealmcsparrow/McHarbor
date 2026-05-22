-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS workflow_runs (
    id          TEXT PRIMARY KEY,
    workflow_id TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'running',
    trigger     TEXT NOT NULL DEFAULT 'manual',
    duration_ms INTEGER DEFAULT 0,
    node_count  INTEGER DEFAULT 0,
    error       TEXT DEFAULT '',
    started_at  TEXT NOT NULL,
    finished_at TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_workflow_runs_workflow_id ON workflow_runs(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_started_at ON workflow_runs(started_at);
