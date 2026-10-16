-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS incidents (
    id                 TEXT PRIMARY KEY,
    env_id             TEXT,
    subject_type       TEXT NOT NULL DEFAULT 'container',
    subject_name       TEXT NOT NULL,
    stack_name         TEXT NOT NULL DEFAULT '',
    signal_kind        TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'open',
    signal_count       INTEGER NOT NULL DEFAULT 1,
    first_signal_at    TEXT NOT NULL,
    last_signal_at     TEXT NOT NULL,
    alert_event_id     TEXT,
    evidence           TEXT NOT NULL DEFAULT '',
    evidence_at        TEXT,
    narrative          TEXT NOT NULL DEFAULT '',
    narrative_prompt   TEXT NOT NULL DEFAULT '',
    narrative_meta     TEXT NOT NULL DEFAULT '',
    narrative_status   TEXT NOT NULL DEFAULT 'none',
    narrative_error    TEXT NOT NULL DEFAULT '',
    narrated_at        TEXT,
    narrated_count     INTEGER NOT NULL DEFAULT 0,
    timeline           TEXT NOT NULL DEFAULT '',
    summary            TEXT NOT NULL DEFAULT '',
    resolved_at        TEXT,
    created_at         TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at         TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_incidents_env_status ON incidents(env_id, status);
CREATE INDEX IF NOT EXISTS idx_incidents_subject ON incidents(env_id, subject_name, status);
CREATE INDEX IF NOT EXISTS idx_incidents_alert ON incidents(alert_event_id);
CREATE INDEX IF NOT EXISTS idx_incidents_last_signal ON incidents(status, last_signal_at);

CREATE TABLE IF NOT EXISTS incident_settings (
    id              TEXT PRIMARY KEY,
    env_id          TEXT NOT NULL,
    narrate_opt_in  INTEGER NOT NULL DEFAULT 0,
    proactive       INTEGER NOT NULL DEFAULT 0,
    hourly_cap      INTEGER NOT NULL DEFAULT 10,
    opted_in_by     TEXT NOT NULL DEFAULT '',
    opted_in_at     TEXT,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_incident_settings_env ON incident_settings(env_id);

CREATE TABLE IF NOT EXISTS behaviour_findings (
    id                TEXT PRIMARY KEY,
    env_id            TEXT,
    subject_type      TEXT NOT NULL,
    subject_name      TEXT NOT NULL,
    kind              TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'open',
    metrics           TEXT NOT NULL DEFAULT '',
    first_seen_at     TEXT NOT NULL,
    last_seen_at      TEXT NOT NULL,
    explanation       TEXT NOT NULL DEFAULT '',
    explanation_meta  TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at        TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_behaviour_findings_env ON behaviour_findings(env_id, status);
CREATE INDEX IF NOT EXISTS idx_behaviour_findings_subject ON behaviour_findings(env_id, subject_name, kind);
