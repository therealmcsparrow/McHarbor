-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS compliance_attestations (
    id          TEXT PRIMARY KEY,
    framework   TEXT NOT NULL,
    run_id      TEXT NOT NULL DEFAULT '',
    scope       TEXT NOT NULL DEFAULT '',
    attested_by TEXT NOT NULL,
    attested_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    note        TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_compliance_attestations_framework
    ON compliance_attestations(framework, attested_at);
CREATE INDEX IF NOT EXISTS idx_compliance_attestations_run
    ON compliance_attestations(run_id);
CREATE INDEX IF NOT EXISTS idx_compliance_attestations_attested_by
    ON compliance_attestations(attested_by);
