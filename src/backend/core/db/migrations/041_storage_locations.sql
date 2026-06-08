-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS storage_locations (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    location_type     TEXT NOT NULL,
    enabled           INTEGER NOT NULL DEFAULT 1,
    host              TEXT,
    port              INTEGER,
    base_path         TEXT,
    region            TEXT,
    bucket            TEXT,
    endpoint          TEXT,
    tenant_id         TEXT,
    site_url          TEXT,
    drive_id          TEXT,
    share_name        TEXT,
    domain            TEXT,
    username          TEXT,
    password          TEXT,
    access_key_id     TEXT,
    secret_access_key TEXT,
    client_id         TEXT,
    client_secret     TEXT,
    refresh_token     TEXT,
    token             TEXT,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_storage_locations_type ON storage_locations(location_type);
CREATE INDEX IF NOT EXISTS idx_storage_locations_enabled ON storage_locations(enabled);
