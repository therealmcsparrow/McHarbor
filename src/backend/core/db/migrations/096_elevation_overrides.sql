-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS permission_elevation (
    id              TEXT PRIMARY KEY,
    permission_key  TEXT NOT NULL UNIQUE,
    elevatable      INTEGER NOT NULL DEFAULT 0,
    updated_by      TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
