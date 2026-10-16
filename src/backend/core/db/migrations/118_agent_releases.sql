-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS agent_releases (
    id                TEXT PRIMARY KEY,
    version           TEXT NOT NULL,
    build             TEXT NOT NULL DEFAULT '',
    source_kind       TEXT NOT NULL DEFAULT '',
    source_url        TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'pulling',
    signature_status  TEXT NOT NULL DEFAULT '',
    signature_key     TEXT NOT NULL DEFAULT '',
    assets            TEXT NOT NULL DEFAULT '[]',
    size_bytes        INTEGER NOT NULL DEFAULT 0,
    image_ref         TEXT NOT NULL DEFAULT '',
    image_error       TEXT NOT NULL DEFAULT '',
    last_error        TEXT NOT NULL DEFAULT '',
    pulled_at         TEXT NOT NULL DEFAULT '',
    pulled_by         TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_releases_version ON agent_releases(version);
CREATE INDEX IF NOT EXISTS idx_agent_releases_status ON agent_releases(status);
