-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Communication channels move to a plugin-owned JSON config, and the two
-- tables that approach replaced are dropped.

-- Reshape `communication_channels` for plugin-driven config: drop the
-- per-channel-type columns (webhook_url, server_url, token, topic, etc.)
-- and add a single `config_enc` blob. Plugins declare their own
-- `ConfigSchema`; the host encrypts the whole config map as one
-- AES-256-GCM ciphertext and stores it in `config_enc`.
--
-- Data migration: rows are copied verbatim (config_enc starts NULL).
-- A startup hook in modules/communications backfills `config_enc` from
-- the typed columns so existing rows survive the transition; the hook
-- is idempotent — rows whose `config_enc` is already non-NULL are
-- skipped, and the run count is logged.
--
-- The rebuild uses the table-rebuild recipe (rename + recreate + copy
-- + drop) because SQLite can't `DROP COLUMN` on older versions and
-- removing 12 columns in one ALTER isn't portable to all drivers.

CREATE TABLE IF NOT EXISTS communication_channels_new (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    channel_type TEXT NOT NULL,
    method       TEXT NOT NULL DEFAULT '',
    is_default   INTEGER NOT NULL DEFAULT 0,
    enabled      INTEGER NOT NULL DEFAULT 1,
    config_enc   TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    config_secret_id TEXT REFERENCES secrets(id)
);

INSERT INTO communication_channels_new
    (id, name, channel_type, method, is_default, enabled, config_enc, created_at, updated_at)
SELECT id, name, channel_type, method, is_default, enabled, NULL, created_at, updated_at
FROM communication_channels;

DROP TABLE communication_channels;
ALTER TABLE communication_channels_new RENAME TO communication_channels;

CREATE INDEX IF NOT EXISTS idx_comm_channels_type    ON communication_channels(channel_type);
CREATE INDEX IF NOT EXISTS idx_comm_channels_default ON communication_channels(is_default);

-- Only one row may be the default. Partial unique index keeps the
-- "exactly one default" invariant at the DB layer so a race in the
-- service's "clear then set" transaction can never leave two rows
-- marked default. Application code MUST clear the existing default
-- inside the same transaction as the new insert/update.
CREATE UNIQUE INDEX IF NOT EXISTS idx_comm_channels_default_unique
  ON communication_channels(is_default) WHERE is_default = 1;


-- Drop the legacy `notification_channels` table owned by the
-- removed `modules/notifications` module. The live dispatch path
-- has been using `communication_channels` (typed, plugin-driven)
-- since the 2026-08-04 conversion; the legacy JSON-blob table is
-- no longer read by anything. Drop the table outright — no production
-- data was ever written through the live UI for this table.

DROP TABLE IF EXISTS notification_channels;


-- Remove the email-server adapter family's table.
--
-- The family was a half-built extension point: an SDK contract with no
-- implementor (every plugin registers in the notification family
-- instead), a table that no code writes, and a dispatcher method
-- (notify.Dispatcher.SendEmail) that the doc comments describe but
-- nobody ever wrote. The management UI was deleted earlier, and the
-- send-email workflow node's "Configured Email Server" picker now reads
-- communication channels of type smtp rather than this table.
--
-- Dropping rather than keeping: an empty table wired to nothing reads as
-- a working feature to the next person, which is how this one survived
-- as long as it did.
DROP TABLE IF EXISTS email_servers;


-- ─── Allow level = 'error' on in-app notifications ────────────────
--
-- 031 created the table with CHECK(level IN ('info','success','warning')),
-- but three call sites broadcast at 'error':
--
--   modules/audit/hash_verifier.go:130  audit hash-chain verification failed
--   modules/audit/hash_verifier.go:156  audit hash-chain verification failed
--   modules/clock/service.go:190        clock drift beyond the audit threshold
--
-- Every one of those inserts failed with
--   CHECK constraint failed: level IN ('info', 'success', 'warning')
-- and was swallowed into a log line. The operator was never told that
-- their audit hash chain had failed verification — which is precisely
-- the notification the feature exists to deliver.
--
-- The constraint is widened rather than the call sites downgraded to
-- 'warning'. A broken audit hash chain and a clock drifting past the
-- audit threshold are integrity failures, not advisories; relabelling
-- them to fit a constraint would understate them in a
-- compliance-relevant surface. The three call sites already chose the
-- right level.
--
-- SQLite cannot DROP a CHECK constraint, so this follows the recipe
-- established by 057: rename, recreate, copy, drop, rebuild indexes.

-- PRAGMA foreign_keys = OFF; (sqlite only)

ALTER TABLE in_app_notifications RENAME TO in_app_notifications_old;

CREATE TABLE in_app_notifications (
    id TEXT PRIMARY KEY,
    level TEXT NOT NULL DEFAULT 'info' CHECK(level IN ('info', 'success', 'warning', 'error')),
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    action TEXT,
    entity_type TEXT,
    entity_id TEXT,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

INSERT INTO in_app_notifications
    (id, level, title, message, action, entity_type, entity_id, created_at, updated_at)
SELECT
    id, level, title, message, action, entity_type, entity_id, created_at, updated_at
FROM in_app_notifications_old;

DROP TABLE in_app_notifications_old;

CREATE INDEX IF NOT EXISTS idx_in_app_notifications_created_at
    ON in_app_notifications(created_at DESC);

-- PRAGMA foreign_keys = ON; (sqlite only)


-- Repair the foreign key the rebuild above leaves dangling.
--
-- Modern SQLite treats ALTER TABLE ... RENAME as a refactor: it rewrites
-- references to the renamed table everywhere else in the schema. So
-- in_app_notification_reads.notification_id, which pointed at
-- in_app_notifications, is quietly repointed at in_app_notifications_old —
-- and three statements later that table is dropped.
--
-- The consequence is not cosmetic. in_app_notification_reads also carries
-- user_id REFERENCES users(id) ON DELETE CASCADE, so deleting any user makes
-- SQLite act on this table, resolve its broken parent reference, and fail:
--
--     SQL logic error: no such table: main.in_app_notifications_old
--
-- Rebuild the table with the reference it was always meant to have. Column
-- list, defaults and all three indexes are reproduced exactly; only the FK
-- target changes. Any rows present are carried over.

CREATE TABLE in_app_notification_reads_fixed (
    id TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL REFERENCES in_app_notifications(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    read_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    deleted_at TEXT
);

-- Only rows whose notification still exists. A read marker for a notification
-- that is already gone could not satisfy the restored foreign key, and means
-- nothing on its own.
INSERT INTO in_app_notification_reads_fixed
    (id, notification_id, user_id, read_at, created_at, updated_at, deleted_at)
SELECT r.id, r.notification_id, r.user_id, r.read_at, r.created_at, r.updated_at, r.deleted_at
  FROM in_app_notification_reads r
 WHERE EXISTS (SELECT 1 FROM in_app_notifications n WHERE n.id = r.notification_id);

DROP TABLE in_app_notification_reads;

-- Safe to rename: nothing else in the schema references this table, so there
-- are no references for SQLite to rewrite this time.
ALTER TABLE in_app_notification_reads_fixed RENAME TO in_app_notification_reads;

CREATE UNIQUE INDEX IF NOT EXISTS idx_in_app_notification_reads_unique
    ON in_app_notification_reads(notification_id, user_id);
CREATE INDEX IF NOT EXISTS idx_in_app_notification_reads_user
    ON in_app_notification_reads(user_id, read_at DESC);
CREATE INDEX IF NOT EXISTS idx_in_app_notification_reads_user_deleted
    ON in_app_notification_reads(user_id, deleted_at, read_at DESC);
