-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS workflow_metrics (
    id TEXT PRIMARY KEY,
    workflow_id TEXT REFERENCES workflows(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    metric_type TEXT NOT NULL DEFAULT 'gauge' CHECK(metric_type IN ('gauge', 'counter', 'event')),
    source_property TEXT NOT NULL DEFAULT 'payload',
    unit TEXT NOT NULL DEFAULT '',
    value_json TEXT NOT NULL DEFAULT 'null',
    numeric_value REAL,
    labels TEXT NOT NULL DEFAULT '{}',
    recorded_at TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_workflow_metrics_workflow_metric_time
    ON workflow_metrics(workflow_id, metric_name, recorded_at);
