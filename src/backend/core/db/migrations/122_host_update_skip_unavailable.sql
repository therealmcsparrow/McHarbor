-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE environments ADD COLUMN host_update_skip_unavailable INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS host_update_self (
    id                            TEXT PRIMARY KEY,
    host_update_check_enabled     INTEGER NOT NULL DEFAULT 0,
    host_update_check_time        TEXT NOT NULL DEFAULT '03:00',
    host_update_check_days        TEXT NOT NULL DEFAULT 'mon,tue,wed,thu,fri,sat,sun',
    host_update_last_checked_at   TEXT,
    host_update_available         INTEGER NOT NULL DEFAULT 0,
    host_update_count             INTEGER NOT NULL DEFAULT 0,
    host_update_manager           TEXT,
    host_update_packages          TEXT,
    host_update_with_new_packages INTEGER NOT NULL DEFAULT 0,
    host_update_skip_unavailable  INTEGER NOT NULL DEFAULT 0,
    created_at                    TEXT NOT NULL,
    updated_at                    TEXT NOT NULL
);

INSERT OR IGNORE INTO host_update_self (id, created_at, updated_at)
VALUES ('self', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
