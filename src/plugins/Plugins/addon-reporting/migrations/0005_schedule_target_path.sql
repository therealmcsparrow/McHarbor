-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Per-target destination folder for storage deliveries.
--
-- The path is relative to the storage location's own configured base path —
-- which is what StorageHandle.Upload documents its key as being relative to —
-- so a schedule can name a folder without ever being able to escape the base
-- an operator set on the location itself.
--
-- Empty means "use the plugin's default", mcharbor-reports/<schedule-slug>,
-- which is what every schedule created before this column behaved as. The
-- column is therefore additive in behaviour as well as in schema.
--
-- Meaningless for a 'comm' target, and dropped server-side for one rather than
-- stored as a field that lies about what the run does.

ALTER TABLE plugin_reporting_schedule_targets ADD COLUMN path TEXT NOT NULL DEFAULT '';
