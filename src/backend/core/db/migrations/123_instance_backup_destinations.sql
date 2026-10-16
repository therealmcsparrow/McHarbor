-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Instance backups gain many storage destinations, the way container backups
-- already have them.
--
-- A single location_id could only ever answer "offsite or not". An operator who
-- wants the copy in two places — the NAS on site and an object store off it —
-- had no way to say so, and that is the whole point of holding more than one:
-- if one destination is unreachable when the backup runs, the other still has it.
--
-- The old location_id / location_key / location_status / location_error columns
-- are LEFT IN PLACE and backfilled from, not dropped. A restore of a backup taken
-- before this migration reads those columns, and the reporting plugin's disaster
-- recovery report already selects them.

CREATE TABLE IF NOT EXISTS instance_backup_schedule_storage_locations (
    schedule_id         TEXT NOT NULL REFERENCES instance_backup_schedules(id) ON DELETE CASCADE,
    storage_location_id TEXT NOT NULL REFERENCES storage_locations(id) ON DELETE CASCADE,
    created_at          TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (schedule_id, storage_location_id)
);

CREATE INDEX IF NOT EXISTS idx_instance_backup_schedule_locations_storage
    ON instance_backup_schedule_storage_locations(storage_location_id);

-- One row per destination per export. storage_location_name and location_type are
-- copied rather than joined: a location deleted later must not erase the record of
-- where a surviving backup was sent.
CREATE TABLE IF NOT EXISTS instance_backup_export_destinations (
    id                    TEXT PRIMARY KEY,
    export_id             TEXT NOT NULL REFERENCES instance_backup_exports(id) ON DELETE CASCADE,
    storage_location_id   TEXT REFERENCES storage_locations(id) ON DELETE SET NULL,
    storage_location_name TEXT NOT NULL DEFAULT '',
    location_type         TEXT NOT NULL DEFAULT '',
    status                TEXT NOT NULL CHECK(status IN ('uploading','success','failure')),
    remote_key            TEXT NOT NULL DEFAULT '',
    error                 TEXT NOT NULL DEFAULT '',
    uploaded_at           TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    bytes_uploaded        INTEGER NOT NULL DEFAULT 0,
    bytes_total           INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_instance_backup_export_destinations_export
    ON instance_backup_export_destinations(export_id);
CREATE INDEX IF NOT EXISTS idx_instance_backup_export_destinations_location
    ON instance_backup_export_destinations(storage_location_id);
CREATE INDEX IF NOT EXISTS idx_instance_backup_export_destinations_status
    ON instance_backup_export_destinations(status);

-- Backfill: every schedule that named a location keeps it, as its single member.
INSERT INTO instance_backup_schedule_storage_locations (schedule_id, storage_location_id, created_at)
SELECT s.id, s.location_id, s.created_at
FROM instance_backup_schedules s
WHERE s.location_id IS NOT NULL
  AND s.location_id <> ''
  AND EXISTS (SELECT 1 FROM storage_locations l WHERE l.id = s.location_id)
  AND NOT EXISTS (
      SELECT 1 FROM instance_backup_schedule_storage_locations x
      WHERE x.schedule_id = s.id AND x.storage_location_id = s.location_id
  );

-- Backfill: every export that recorded an upload keeps that record as one
-- destination row. 'uploaded' was the old success value; the new column's CHECK
-- only accepts uploading / success / failure, so it is mapped.
INSERT INTO instance_backup_export_destinations (
    id, export_id, storage_location_id, storage_location_name, location_type,
    status, remote_key, error, uploaded_at, created_at, updated_at, bytes_uploaded, bytes_total
)
SELECT
    e.id || '-legacy',
    e.id,
    e.location_id,
    COALESCE((SELECT l.name FROM storage_locations l WHERE l.id = e.location_id), ''),
    COALESCE((SELECT l.location_type FROM storage_locations l WHERE l.id = e.location_id), ''),
    CASE e.location_status WHEN 'uploaded' THEN 'success' WHEN 'failed' THEN 'failure' ELSE 'success' END,
    COALESCE(e.location_key, ''),
    COALESCE(e.location_error, ''),
    e.completed_at,
    e.created_at,
    e.updated_at,
    CASE WHEN e.location_status = 'uploaded' THEN e.size_bytes ELSE 0 END,
    e.size_bytes
FROM instance_backup_exports e
WHERE e.location_id IS NOT NULL
  AND e.location_id <> ''
  AND COALESCE(e.location_status, '') <> ''
  AND NOT EXISTS (
      SELECT 1 FROM instance_backup_export_destinations d WHERE d.export_id = e.id
  );
