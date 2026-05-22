-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Add environment-level activity settings for events, metrics, highlights, and disk alerts.

ALTER TABLE environments ADD COLUMN track_container_events_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE environments ADD COLUMN collect_container_metrics_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE environments ADD COLUMN highlight_container_changes_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE environments ADD COLUMN docker_disk_usage_notifications_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE environments ADD COLUMN docker_disk_usage_threshold_percent INTEGER NOT NULL DEFAULT 80;
