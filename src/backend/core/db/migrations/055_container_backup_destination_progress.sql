-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Per-destination upload progress for backup runs. Updated during upload
-- so the UI can show "uploading to OneDrive (450 MB / 1.2 GB)" rather than
-- the previous opaque "uploading" stage.
ALTER TABLE container_backup_run_destinations
    ADD COLUMN bytes_uploaded INTEGER NOT NULL DEFAULT 0;
ALTER TABLE container_backup_run_destinations
    ADD COLUMN bytes_total    INTEGER NOT NULL DEFAULT 0;