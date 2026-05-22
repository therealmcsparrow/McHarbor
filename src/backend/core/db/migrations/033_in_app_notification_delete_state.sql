-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE in_app_notification_reads
    ADD COLUMN deleted_at TEXT;

CREATE INDEX IF NOT EXISTS idx_in_app_notification_reads_user_deleted
    ON in_app_notification_reads(user_id, deleted_at, read_at DESC);
