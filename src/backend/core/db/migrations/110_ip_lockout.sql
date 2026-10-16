-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES (lower(hex(randomblob(8))), 'security.lockout_ip_max_attempts', '30', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES (lower(hex(randomblob(8))), 'security.lockout_ip_window_seconds', '900', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES (lower(hex(randomblob(8))), 'security.lockout_ip_lock_seconds', '900', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

ALTER TABLE login_attempts ADD COLUMN ip_source TEXT DEFAULT '';

CREATE TABLE IF NOT EXISTS login_ip_locks (
    id           TEXT PRIMARY KEY,
    ip_address   TEXT NOT NULL,
    locked_until TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_login_ip_locks_ip ON login_ip_locks(ip_address);
CREATE INDEX IF NOT EXISTS idx_login_ip_locks_until ON login_ip_locks(locked_until);
