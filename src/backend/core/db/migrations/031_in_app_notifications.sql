-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS in_app_notifications (
    id TEXT PRIMARY KEY,
    level TEXT NOT NULL DEFAULT 'info' CHECK(level IN ('info', 'success', 'warning')),
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    action TEXT,
    entity_type TEXT,
    entity_id TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_in_app_notifications_created_at
    ON in_app_notifications(created_at DESC);

CREATE TABLE IF NOT EXISTS in_app_notification_reads (
    id TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL REFERENCES in_app_notifications(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    read_at TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_in_app_notification_reads_unique
    ON in_app_notification_reads(notification_id, user_id);

CREATE INDEX IF NOT EXISTS idx_in_app_notification_reads_user
    ON in_app_notification_reads(user_id, read_at DESC);
