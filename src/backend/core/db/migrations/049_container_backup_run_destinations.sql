-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS container_backup_run_destinations (
    id                    TEXT PRIMARY KEY,
    run_id                TEXT NOT NULL REFERENCES container_backup_runs(id) ON DELETE CASCADE,
    storage_location_id   TEXT REFERENCES storage_locations(id) ON DELETE SET NULL,
    storage_location_name TEXT NOT NULL DEFAULT '',
    location_type         TEXT NOT NULL DEFAULT '',
    status                TEXT NOT NULL CHECK(status IN ('uploading','success','failure')),
    remote_path           TEXT NOT NULL DEFAULT '',
    error                 TEXT NOT NULL DEFAULT '',
    uploaded_at           TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_container_backup_run_destinations_run
    ON container_backup_run_destinations(run_id);

CREATE INDEX IF NOT EXISTS idx_container_backup_run_destinations_storage
    ON container_backup_run_destinations(storage_location_id);
