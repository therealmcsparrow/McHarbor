-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Settings rows that belong to no single feature table.

-- Remove settings rows with NULL id.
--
-- The self-update checker previously INSERTed into settings without
-- specifying the `id` column, leaving rows where `id IS NULL`. SQLite
-- allows this because `id TEXT PRIMARY KEY` is not rowid-alias and
-- does not implicitly enforce NOT NULL. Such rows break
-- GET /api/settings because the Scan into a string column fails.
--
-- The self-update checker has been fixed to always provide a
-- deterministic id, so the next periodic run re-creates the row.

DELETE FROM settings WHERE id IS NULL OR id = '';


-- Account lockout policy.
--
-- Admins can change these at runtime via Settings → Security. Defaults
-- are deliberately conservative (5 attempts, 15 minutes) and match the
-- defaults in core/auth/lockout.go.

INSERT INTO settings (id, key, value, category)
    VALUES ('set_lockout_max_attempts', 'lockout_max_attempts', '5', 'security')
    ON CONFLICT (id) DO NOTHING;

INSERT INTO settings (id, key, value, category)
    VALUES ('set_lockout_window_seconds', 'lockout_window_seconds', '900', 'security')
    ON CONFLICT (id) DO NOTHING;

INSERT INTO settings (id, key, value, category)
    VALUES ('set_lockout_initial_seconds', 'lockout_initial_seconds', '60', 'security')
    ON CONFLICT (id) DO NOTHING;

INSERT INTO settings (id, key, value, category)
    VALUES ('set_lockout_max_lock_seconds', 'lockout_max_lock_seconds', '3600', 'security')
    ON CONFLICT (id) DO NOTHING;

INSERT INTO settings (id, key, value, category)
    VALUES ('set_password_min_length', 'password_min_length', '12', 'security')
    ON CONFLICT (id) DO NOTHING;

INSERT INTO settings (id, key, value, category)
    VALUES ('set_password_require_upper', 'password_require_upper', 'true', 'security')
    ON CONFLICT (id) DO NOTHING;

INSERT INTO settings (id, key, value, category)
    VALUES ('set_password_require_lower', 'password_require_lower', 'true', 'security')
    ON CONFLICT (id) DO NOTHING;

INSERT INTO settings (id, key, value, category)
    VALUES ('set_password_require_digit', 'password_require_digit', 'true', 'security')
    ON CONFLICT (id) DO NOTHING;

INSERT INTO settings (id, key, value, category)
    VALUES ('set_password_require_symbol', 'password_require_symbol', 'true', 'security')
    ON CONFLICT (id) DO NOTHING;

INSERT INTO settings (id, key, value, category)
    VALUES ('set_password_max_age_days', 'password_max_age_days', '0', 'security')
    ON CONFLICT (id) DO NOTHING;

-- How many previous hashes are checked on password change. The
-- DefaultPasswordPolicy hard-codes 5; this row lets the admin override it
-- from the UI. INSERT OR IGNORE keeps existing tuned values intact.

INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES (lower(hex(randomblob(8))), 'password_history_size', '5', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);


-- Seed security policy defaults.
--
-- The `security.*` keys are read at runtime by the users module. All rows
-- are INSERT OR IGNORE so the migration is idempotent and does not
-- overwrite a value an admin has already tuned.

INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES (lower(hex(randomblob(8))), 'security.password_reset_ttl_minutes', '60', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES (lower(hex(randomblob(8))), 'security.password_reset_min_password_length', '8', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- Password-reset email delivery. Security → Password is the surface that
-- edits these four rows. Settings rows rather than a table: there is exactly
-- one password-reset template per install, it is four scalar values, and the
-- TTL knob above already lives here.
--
-- security.password_reset_email_channel_id
--     A communication_channels.id. Empty means "not configured" — the
--     reset flow then logs and does not send. Not a foreign key:
--     settings.value is a TEXT bag shared by every other key, so the
--     users service re-checks the channel on send.
--
-- security.password_reset_email_from
--     Optional envelope/From override. Empty means "use whatever the
--     chosen channel is already configured with", which is the right
--     default: that address is the one whose SPF/DKIM records match.
--
-- security.password_reset_email_subject / _body
--     The message itself, seeded with a working default so a fresh
--     install can send a usable email the moment a channel is picked.
--     Placeholders are rendered by users.renderResetTemplate:
--     {{username}} {{displayName}} {{email}} {{link}} {{expiresMinutes}}
--     {{expiresAt}} {{appName}}.

INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES (lower(hex(randomblob(8))), 'security.password_reset_email_channel_id', '', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES (lower(hex(randomblob(8))), 'security.password_reset_email_from', '', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES (lower(hex(randomblob(8))), 'security.password_reset_email_subject', 'Reset your {{appName}} password', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
-- The body is one string literal with real newlines in it rather than a
-- char(10) concatenation: every driver the migrator targets accepts a
-- multi-line literal, and it keeps the seeded template readable as the
-- email it becomes.
VALUES (
    lower(hex(randomblob(8))),
    'security.password_reset_email_body',
    'Hello {{displayName}},

We received a request to reset the password for your {{appName}} account ({{username}}).

Choose a new password here:
{{link}}

This link can be used once and expires in {{expiresMinutes}} minutes, at {{expiresAt}}.

If you did not ask for this, you can ignore this message — your password stays unchanged and no one has been given access to your account.',
    'security',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
);


-- Automatic sign-out: the idle window, in seconds. 600 (ten minutes) by
-- default; 0 disables automatic sign-out entirely. Bounds are enforced by
-- the API, not here.

INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES (lower(hex(randomblob(8))), 'security.session_idle_timeout_seconds', '600', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
