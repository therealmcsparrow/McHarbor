-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS compliance_pack_runs (
    id             TEXT PRIMARY KEY,
    framework      TEXT NOT NULL,
    environment_id TEXT NOT NULL DEFAULT '',
    trigger_kind   TEXT NOT NULL DEFAULT 'manual',
    started_by     TEXT NOT NULL DEFAULT '',
    started_at     TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    finished_at    TEXT,
    status         TEXT NOT NULL DEFAULT 'running',
    checks_pass    INTEGER NOT NULL DEFAULT 0,
    checks_warn    INTEGER NOT NULL DEFAULT 0,
    checks_fail    INTEGER NOT NULL DEFAULT 0,
    checks_info    INTEGER NOT NULL DEFAULT 0,
    checks_unknown INTEGER NOT NULL DEFAULT 0,
    last_error     TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at     TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_compliance_pack_runs_framework
    ON compliance_pack_runs(framework, environment_id, started_at);
CREATE INDEX IF NOT EXISTS idx_compliance_pack_runs_started
    ON compliance_pack_runs(started_at);

CREATE TABLE IF NOT EXISTS compliance_pack_results (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL,
    control_id TEXT NOT NULL,
    control_group TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL,
    facts      TEXT NOT NULL DEFAULT '[]',
    reasons    TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE(run_id, control_id)
);
CREATE INDEX IF NOT EXISTS idx_compliance_pack_results_run
    ON compliance_pack_results(run_id, control_id);
CREATE INDEX IF NOT EXISTS idx_compliance_pack_results_control
    ON compliance_pack_results(control_id, status);
