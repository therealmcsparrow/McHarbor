-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Lifecycle events cover all Docker resource types: containers,
-- images, volumes, networks, and stacks. The previous `container_events`
-- table only tracked container events; the new `lifecycle_events`
-- table is a superset and is the source of truth for the Logging
-- tab in the Docker menu. The old container_events table is kept
-- for historical reads (the existing /activity page queries it)
-- and writes are mirrored here for new events.
--
-- subject_type:
--   'container'  - a single container
--   'image'      - a Docker image
--   'volume'     - a named volume
--   'network'    - a user-defined bridge / overlay network
--   'stack'      - a Compose stack managed by McHarbor
--
-- state (when known; null otherwise):
--   containers:  'running' | 'stopped' | 'paused' | 'exited' | 'created'
--                | 'restarting' | 'removing' | 'dead'
--   images:      'available' | 'in_use' | 'dangling'
--   volumes:     'created' | 'in_use' | 'dangling'
--   networks:    'active' | 'inactive'
--   stacks:      'created' | 'up' | 'partial' | 'stopped' | 'errored'
--
-- severity is the user-facing hint derived from the action. The
-- UI badges events with 'success' (running/created), 'info'
-- (other neutral), or 'error' (failures / destroy). The mapping
-- rule is computed server-side so the UI doesn't have to know
-- which action maps to which severity.
CREATE TABLE IF NOT EXISTS lifecycle_events (
    id              TEXT PRIMARY KEY,
    environment_id  TEXT REFERENCES environments(id) ON DELETE SET NULL,
    subject_type    TEXT NOT NULL CHECK(subject_type IN ('container','image','volume','network','stack')),
    subject_id      TEXT NOT NULL,
    subject_name    TEXT,
    event_type      TEXT NOT NULL,
    action          TEXT NOT NULL,
    state           TEXT,
    severity        TEXT NOT NULL CHECK(severity IN ('info','success','warning','error')),
    metadata        TEXT,
    source          TEXT NOT NULL DEFAULT 'docker' CHECK(source IN ('docker','compose','mcharbor')),
    timestamp       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_lifecycle_events_timestamp
    ON lifecycle_events(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_lifecycle_events_env_time
    ON lifecycle_events(environment_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_lifecycle_events_subject
    ON lifecycle_events(subject_type, subject_id);
CREATE INDEX IF NOT EXISTS idx_lifecycle_events_severity
    ON lifecycle_events(severity, timestamp DESC);

-- Backfill: copy the most recent container_events into
-- lifecycle_events so existing users see their history on day 1.
-- The new table uses a wider subject column; old rows already
-- carry 'container' as the implicit subject. ON CONFLICT keeps
-- the script idempotent across re-runs.
INSERT INTO lifecycle_events (
    id, environment_id, subject_type, subject_id, subject_name,
    event_type, action, state, severity, metadata, source, timestamp,
    created_at, updated_at
)
SELECT
    id, environment_id, 'container', container_id, container_name,
    event_type, action, NULL, 'info', metadata, 'docker', timestamp,
    created_at, updated_at
FROM container_events
WHERE NOT EXISTS (SELECT 1 FROM lifecycle_events WHERE lifecycle_events.id = container_events.id);
