-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE lifecycle_events_new (
    id              TEXT PRIMARY KEY,
    environment_id  TEXT REFERENCES environments(id) ON DELETE SET NULL,
    subject_type    TEXT NOT NULL,
    subject_id      TEXT NOT NULL,
    subject_name    TEXT,
    event_type      TEXT NOT NULL,
    action          TEXT NOT NULL,
    state           TEXT,
    severity        TEXT NOT NULL CHECK(severity IN ('info','success','warning','error')),
    metadata        TEXT,
    source          TEXT NOT NULL DEFAULT 'docker' CHECK(length(source) > 0),
    timestamp       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

INSERT INTO lifecycle_events_new (
    id, environment_id, subject_type, subject_id, subject_name,
    event_type, action, state, severity, metadata, source, timestamp,
    created_at, updated_at
)
SELECT
    id, environment_id, subject_type, subject_id, subject_name,
    event_type, action, state, severity, metadata, source, timestamp,
    created_at, updated_at
FROM lifecycle_events;

DROP TABLE lifecycle_events;

ALTER TABLE lifecycle_events_new RENAME TO lifecycle_events;

CREATE INDEX IF NOT EXISTS idx_lifecycle_events_timestamp
    ON lifecycle_events(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_lifecycle_events_env_time
    ON lifecycle_events(environment_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_lifecycle_events_subject
    ON lifecycle_events(subject_type, subject_id);
CREATE INDEX IF NOT EXISTS idx_lifecycle_events_severity
    ON lifecycle_events(severity, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_lifecycle_events_env_sev_time
    ON lifecycle_events(environment_id, severity, timestamp DESC);
