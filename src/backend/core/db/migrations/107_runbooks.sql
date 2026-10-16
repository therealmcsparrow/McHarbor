-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS runbooks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    attach_kind TEXT NOT NULL DEFAULT 'environment',
    attach_source_type TEXT NOT NULL DEFAULT '',
    attach_condition_key TEXT NOT NULL DEFAULT '',
    attach_alert_id TEXT NOT NULL DEFAULT '',
    attach_subject TEXT NOT NULL DEFAULT '',
    environment_id TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runbooks_condition ON runbooks(attach_source_type, attach_condition_key, enabled);
CREATE INDEX IF NOT EXISTS idx_runbooks_alert ON runbooks(attach_alert_id, enabled);
CREATE INDEX IF NOT EXISTS idx_runbooks_subject ON runbooks(attach_kind, environment_id, attach_subject, enabled);

CREATE TABLE IF NOT EXISTS runbook_steps (
    id TEXT PRIMARY KEY,
    runbook_id TEXT NOT NULL REFERENCES runbooks(id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0,
    kind TEXT NOT NULL DEFAULT 'instruction',
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    diagnostic_key TEXT NOT NULL DEFAULT '',
    action_key TEXT NOT NULL DEFAULT '',
    action_params TEXT NOT NULL DEFAULT '{}',
    required INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runbook_steps_runbook ON runbook_steps(runbook_id, position);

CREATE TABLE IF NOT EXISTS runbook_runs (
    id TEXT PRIMARY KEY,
    runbook_id TEXT NOT NULL REFERENCES runbooks(id) ON DELETE CASCADE,
    runbook_name TEXT NOT NULL DEFAULT '',
    environment_id TEXT NOT NULL DEFAULT '',
    subject_kind TEXT NOT NULL DEFAULT '',
    subject_id TEXT NOT NULL DEFAULT '',
    subject_name TEXT NOT NULL DEFAULT '',
    incident_id TEXT NOT NULL DEFAULT '',
    alert_event_id TEXT NOT NULL DEFAULT '',
    started_by TEXT NOT NULL DEFAULT '',
    started_by_name TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL DEFAULT 'in_progress',
    note TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_runbook_runs_incident ON runbook_runs(incident_id);
CREATE INDEX IF NOT EXISTS idx_runbook_runs_alert_event ON runbook_runs(alert_event_id);
CREATE INDEX IF NOT EXISTS idx_runbook_runs_open ON runbook_runs(runbook_id, outcome, started_at);
CREATE INDEX IF NOT EXISTS idx_runbook_runs_env ON runbook_runs(environment_id, started_at);

CREATE TABLE IF NOT EXISTS runbook_step_results (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES runbook_runs(id) ON DELETE CASCADE,
    step_id TEXT NOT NULL DEFAULT '',
    step_position INTEGER NOT NULL DEFAULT 0,
    step_title TEXT NOT NULL DEFAULT '',
    step_kind TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'pending',
    skip_reason TEXT NOT NULL DEFAULT '',
    output TEXT NOT NULL DEFAULT '',
    finding TEXT NOT NULL DEFAULT '',
    acted_by TEXT NOT NULL DEFAULT '',
    acted_by_name TEXT NOT NULL DEFAULT '',
    at TEXT NOT NULL,
    last_error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_runbook_step_results_run ON runbook_step_results(run_id, step_position, at);
