-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Per-user time and date format preferences.
--   time_format: '24h' (default) or '12h'
--   date_format: 'ddmmyyyy' (default) or 'mmddyyyy'
-- Both columns are TEXT so we can add new values without a schema
-- change. The frontend formatters read the values verbatim and
-- fall back to the defaults when a row was created before this
-- migration.
ALTER TABLE users ADD COLUMN time_format TEXT NOT NULL DEFAULT '24h';
ALTER TABLE users ADD COLUMN date_format TEXT NOT NULL DEFAULT 'ddmmyyyy';

-- Default application language. Stored on the `settings` table as a
-- single key/value row so the i18n init can read it when the user
-- has no per-user preferred_language set. The settings module owns
-- this row; the auth module is a read-only consumer.
INSERT INTO settings (id, key, value, category, created_at, updated_at)
VALUES (
    lower(hex(randomblob(16))),
    'default_language',
    'en',
    'general',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
)
ON CONFLICT (key) DO NOTHING;
