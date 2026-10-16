-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Reporting plugin schema.
--
-- Every table is prefixed plugin_reporting_ so the host's uninstall gate
-- can drop the plugin's storage in one pass, and so the plugin's fenced
-- DB handle will accept the statements at all.

CREATE TABLE IF NOT EXISTS plugin_reporting_reports (
    id             TEXT PRIMARY KEY,
    report_type    TEXT    NOT NULL,
    title          TEXT    NOT NULL,
    environment_id TEXT    NOT NULL DEFAULT '',
    environment    TEXT    NOT NULL DEFAULT '',
    period_start   TEXT    NOT NULL,
    period_end     TEXT    NOT NULL,
    -- Comma-separated levels present in this report, e.g. "strategic,tactical".
    levels         TEXT    NOT NULL DEFAULT '',
    status         TEXT    NOT NULL DEFAULT 'ready',
    -- -1 means "not scored": a report type without a strategic section,
    -- or one where no domain could be assessed. Distinct from 0, which
    -- would mean a genuinely critical posture.
    overall_score  INTEGER NOT NULL DEFAULT -1,
    overall_band   TEXT    NOT NULL DEFAULT '',
    headline       TEXT    NOT NULL DEFAULT '',
    -- The rendered document, as JSON. Stored rather than recomputed so a
    -- report is a fixed record of what was true when it was generated —
    -- regenerating it later would silently produce different figures and
    -- make the archive worthless as evidence.
    document       TEXT    NOT NULL,
    generated_by   TEXT    NOT NULL DEFAULT '',
    size_bytes     INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT    NOT NULL,
    updated_at     TEXT    NOT NULL
);

-- The library lists newest-first and filters by type; these are the only
-- two access paths.
CREATE INDEX IF NOT EXISTS idx_plugin_reporting_reports_created
    ON plugin_reporting_reports(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_plugin_reporting_reports_type
    ON plugin_reporting_reports(report_type, created_at DESC);

-- Per-source collection outcomes, kept for support: when a report says a
-- source was unavailable, this is the evidence for why.
CREATE TABLE IF NOT EXISTS plugin_reporting_runs (
    id          TEXT    PRIMARY KEY,
    report_id   TEXT    NOT NULL,
    source      TEXT    NOT NULL,
    ok          INTEGER NOT NULL DEFAULT 0,
    http_status INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    note        TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_reporting_runs_report
    ON plugin_reporting_runs(report_id);
