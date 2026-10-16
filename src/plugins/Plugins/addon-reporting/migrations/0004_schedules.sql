-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Scheduled reports.
--
-- A schedule runs on a cron, produces the document, delivers it to one or
-- more targets, and keeps only its most recent run. Scheduled runs never
-- enter the report library: the library is the record of reports a person
-- asked for, and filling it with a nightly job would bury them.

CREATE TABLE IF NOT EXISTS plugin_reporting_schedules (
    id               TEXT    PRIMARY KEY,
    name             TEXT    NOT NULL,
    report_type      TEXT    NOT NULL,

    -- The generation request, stored field by field rather than as an
    -- opaque blob so a schedule can be listed, filtered and shown in a
    -- table without decoding JSON per row.
    subject_kind     TEXT    NOT NULL DEFAULT '',
    subject_id       TEXT    NOT NULL DEFAULT '',
    subject_name     TEXT    NOT NULL DEFAULT '',
    environment_id   TEXT    NOT NULL DEFAULT '',
    environment      TEXT    NOT NULL DEFAULT '',
    period_days      INTEGER NOT NULL DEFAULT 30,
    levels           TEXT    NOT NULL DEFAULT '',
    options_json     TEXT    NOT NULL DEFAULT '[]',
    format           TEXT    NOT NULL DEFAULT 'pdf',
    prompt           TEXT    NOT NULL DEFAULT '',
    ai_connection_id TEXT    NOT NULL DEFAULT '',

    cron             TEXT    NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,

    -- Both the id and the name of whoever created the schedule. The id is
    -- what the run actually needs: collection runs as that user through a
    -- host-issued session, so a scheduled report can never contain data
    -- its author could not already see. The name is only for display, and
    -- survives the user being renamed.
    created_by_id    TEXT    NOT NULL DEFAULT '',
    created_by       TEXT    NOT NULL DEFAULT '',

    last_run_at      TEXT    NOT NULL DEFAULT '',
    next_run_at      TEXT    NOT NULL DEFAULT '',
    created_at       TEXT    NOT NULL,
    updated_at       TEXT    NOT NULL
);

-- The scheduler's only query: enabled schedules whose next run has passed.
CREATE INDEX IF NOT EXISTS idx_plugin_reporting_schedules_due
    ON plugin_reporting_schedules(enabled, next_run_at);

CREATE TABLE IF NOT EXISTS plugin_reporting_schedule_targets (
    id          TEXT NOT NULL PRIMARY KEY,
    schedule_id TEXT NOT NULL,
    -- 'comm' (a communication channel) or 'storage' (a storage location).
    kind        TEXT NOT NULL,
    -- The host-side row id. Never a credential: the host resolves and
    -- decrypts the target, the plugin only ever names it.
    ref_id      TEXT NOT NULL,
    created_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_reporting_schedule_targets_schedule
    ON plugin_reporting_schedule_targets(schedule_id);

-- schedule_id is the PRIMARY KEY, not a foreign key column: that is what
-- makes "keep only the last run" structural instead of a cleanup job that
-- can fall behind. A new run replaces the old one in one upsert.
CREATE TABLE IF NOT EXISTS plugin_reporting_schedule_runs (
    schedule_id   TEXT PRIMARY KEY,
    -- 'running' | 'success' | 'partial' | 'failed'
    status        TEXT NOT NULL,
    title         TEXT NOT NULL DEFAULT '',
    -- The rendered document, so the last run stays readable in the UI
    -- without being written into the library.
    document_json TEXT NOT NULL DEFAULT '',
    -- An i18n code, never a raw error string: a raw error can carry a
    -- path, a hostname or a query that has no business being stored.
    error         TEXT NOT NULL DEFAULT '',
    -- Per-target outcome, so a partial delivery says which target failed.
    delivery_json TEXT NOT NULL DEFAULT '[]',
    started_at    TEXT NOT NULL,
    finished_at   TEXT NOT NULL DEFAULT ''
);
