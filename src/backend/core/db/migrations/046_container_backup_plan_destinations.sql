-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS container_backup_plan_storage_locations (
    plan_id             TEXT NOT NULL REFERENCES container_backup_plans(id) ON DELETE CASCADE,
    storage_location_id TEXT NOT NULL REFERENCES storage_locations(id) ON DELETE CASCADE,
    created_at          TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (plan_id, storage_location_id)
);

CREATE INDEX IF NOT EXISTS idx_container_backup_plan_storage_locations_storage
    ON container_backup_plan_storage_locations(storage_location_id);

INSERT OR IGNORE INTO container_backup_plan_storage_locations (plan_id, storage_location_id, created_at)
SELECT id, storage_location_id, created_at
FROM container_backup_plans
WHERE storage_location_id IS NOT NULL AND TRIM(storage_location_id) <> '';
