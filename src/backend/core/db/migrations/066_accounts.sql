-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Multi-factor auth, password policy, and the account and session columns
-- the RBAC v2 work needs on `users` and `sessions`.

-- MFA: TOTP secrets and recovery codes per user.
CREATE TABLE IF NOT EXISTS user_mfa (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    secret_encrypted TEXT NOT NULL DEFAULT '',
    recovery_codes_encrypted TEXT NOT NULL DEFAULT '[]',
    enrolled_at TEXT,
    last_used_at TEXT,
    scratch_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_user_mfa_enabled ON user_mfa(enabled);

-- Short-lived challenges stored server-side for login flows when a user
-- supplies password+TOTP together and we need to verify the TOTP before
-- issuing a session.
CREATE TABLE IF NOT EXISTS mfa_login_challenges (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    expires_at TEXT NOT NULL,
    consumed INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_mfa_login_challenges_user ON mfa_login_challenges(user_id);
CREATE INDEX IF NOT EXISTS idx_mfa_login_challenges_expires ON mfa_login_challenges(expires_at);


-- Password history
--
-- Stores the last N Argon2id hashes for each user so the password
-- policy can reject re-use. Plaintext is never stored.

CREATE TABLE IF NOT EXISTS password_history (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_password_history_user ON password_history(user_id, created_at);


-- Usernames are case-insensitive identifiers. The application
-- normalises every login/register/create/provision path through
-- auth.NormalizeUsername (Go's strings.ToLower — Unicode-aware), so
-- "Linda" and "linda" resolve to the same account.
-- display_name is unaffected and remains what the UI shows.
--
--   1. Lowercases existing usernames with SQL's lower(), which is
--      ASCII-ONLY — it does not fold non-ASCII case pairs (e.g. an
--      uppercase "Ä" is not SQL-lower()'d to "ä"). That covers the
--      ASCII majority of usernames but is NOT the full fix: a
--      username containing a non-ASCII uppercase letter survives
--      this UPDATE unchanged, while auth.NormalizeUsername folds it
--      on every subsequent login, which would make that account
--      permanently unreachable. The remaining Unicode gap is closed
--      by auth.NormalizeUsernamesSweep, a Go-side sweep that runs at
--      startup AFTER migrations apply, on every boot — see
--      core/auth/username_normalize_sweep.go. It is a no-op (nothing
--      logged, nothing written) once every username is already
--      canonical, so running it every boot costs nothing on a clean
--      database.
--
--   2. Recreates idx_users_username as a case-insensitive (COLLATE
--      NOCASE) unique index. This is defence-in-depth: even if some
--      future code path forgets to normalise before an INSERT, the
--      database itself still refuses a case-variant duplicate rather
--      than silently allowing two accounts that collide once folded
--      to the same case. Note NOCASE itself is also ASCII-only in
--      SQLite; the Unicode side of the guarantee comes from the Go
--      sweep always keeping stored usernames pre-folded, not from
--      this index's collation.
--
-- The UPDATE is intentionally NOT wrapped in OR IGNORE / OR REPLACE. If a
-- genuine case-collision existed (e.g. both "Linda" and "linda" as separate
-- rows), this statement fails with a UNIQUE constraint violation and the
-- migration aborts. That is correct behaviour: silently collapsing two
-- distinct accounts into one on a case fold would be a privilege-escalation
-- bug, not a fix. An operator hitting this should reconcile the duplicate
-- accounts by hand before re-running migrations. The Go-side sweep applies
-- the identical fail-loudly rule for the Unicode case it additionally
-- covers.
--
-- Note: this is a one-way transform. The original mixed-case spelling is
-- not recoverable from the database once this runs; only a pre-migration
-- backup preserves it.

UPDATE users SET username = lower(username) WHERE username <> lower(username);

DROP INDEX IF EXISTS idx_users_username;
CREATE UNIQUE INDEX idx_users_username ON users(username COLLATE NOCASE);

-- The NOCASE index above is the collision guard, but every application
-- lookup (auth.go LoginWithContext/Register, lockout.go CheckLockout/
-- RecordFailedAttempt, users/service.go Create) compares username with
-- the column's default BINARY collation, because the application always
-- passes an already-normalised (lowercase) value and does a plain `=`
-- comparison. SQLite will not use a NOCASE index to satisfy a BINARY `=`
-- predicate, so those lookups would silently fall back to a full table scan.
--
-- idx_users_username_bin (BINARY, non-unique — uniqueness is already
-- enforced by the NOCASE index) is what login/lockout/create queries hit.

CREATE INDEX IF NOT EXISTS idx_users_username_bin ON users(username);


-- ─── Password reset tokens ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id                    TEXT PRIMARY KEY,
    user_id               TEXT NOT NULL,
    token_hash            TEXT NOT NULL,      -- sha256(secret) hex
    created_by            TEXT NOT NULL,      -- 'self' or admin user id
    created_at            TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    expires_at            TEXT NOT NULL,      -- 1h email, 24h admin
    consumed_at           TEXT,
    consumed_ip           TEXT,
    consumed_user_agent   TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_prt_user ON password_reset_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_prt_hash ON password_reset_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_prt_expires ON password_reset_tokens(expires_at);

-- ─── Account lockout / disable / force-change ───────────────────────
ALTER TABLE users ADD COLUMN disabled_at           TEXT;
ALTER TABLE users ADD COLUMN disabled_by           TEXT;
ALTER TABLE users ADD COLUMN disabled_reason       TEXT;
ALTER TABLE users ADD COLUMN must_reset_password   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN password_changed_at   TEXT;
ALTER TABLE users ADD COLUMN password_failed_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN password_locked_until TEXT;
ALTER TABLE users ADD COLUMN last_login_ip         TEXT;
ALTER TABLE users ADD COLUMN last_login_user_agent TEXT;

-- ─── Sessions table additions (token metadata for revoke) ───────────
ALTER TABLE sessions ADD COLUMN ip_address TEXT;
ALTER TABLE sessions ADD COLUMN user_agent TEXT;
ALTER TABLE sessions ADD COLUMN revoked_at  TEXT;
ALTER TABLE sessions ADD COLUMN revoked_by  TEXT;
CREATE INDEX IF NOT EXISTS idx_sessions_revoked ON sessions(revoked_at);

-- ─── Login attempt throttling (forensic visibility) ────────────────
CREATE TABLE IF NOT EXISTS login_attempts (
    id           TEXT PRIMARY KEY,
    username     TEXT,
    ip_address   TEXT NOT NULL,
    succeeded    INTEGER NOT NULL,
    user_id      TEXT,
    user_agent   TEXT,
    reason       TEXT,           -- 'invalid_credentials','disabled','locked','mfa_required','must_reset_password'
    created_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_login_attempts_ip ON login_attempts(ip_address, created_at);
CREATE INDEX IF NOT EXISTS idx_login_attempts_user ON login_attempts(user_id, created_at);


-- ─── Legacy owner placeholder ──────────────────────────────────────
--
-- Rows that predate ownership tracking (stacks, backup plans, plugins,
-- seeded policies) are assigned to a synthetic placeholder user that no
-- real login maps to, rather than silently to the first admin — that
-- would grant ownership to a user without consent. The row-level
-- authorization layer treats the placeholder as not-owning, so existing
-- data stays visible to anyone with the appropriate permission, but no
-- one can claim ownership through it. Admins can later reassign ownership
-- to real users.
INSERT INTO users (
    id, username, email, password_hash,
    preferred_language, created_at, updated_at,
    must_reset_password, password_failed_count
) VALUES (
    '__legacy_owner__', '__legacy_owner__', NULL, '',
    'en', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP,
    1, 0
)
ON CONFLICT (id) DO NOTHING;


-- Automatic sign-out (idle timeout).
--
-- `expires_at` is the absolute session lifetime (24h). It says nothing
-- about whether the human is still at the keyboard, so it cannot answer
-- "log this person out after ten idle minutes".
--
-- `last_activity_at` is that second clock. It is stamped by the browser
-- via POST /auth/session/touch, throttled to at most once a minute while
-- real input events are happening, and never by ordinary API traffic --
-- a background poll is the server's activity, not the user's, and
-- treating the two the same would mean an open dashboard never idles.
--
-- Existing rows are left NULL on purpose: ValidateSession initialises a
-- NULL column to "now" the first time it sees it, so an upgrade does not
-- sign every logged-in operator out mid-session.
ALTER TABLE sessions ADD COLUMN last_activity_at TEXT;


-- Per-user toast overrides.
--
-- Every column is NULL by default and NULL means "inherit the app-wide
-- default" (settings table, category 'toast'). That is deliberate: a user who
-- never opens Settings must keep following the operator's default, including
-- when the operator later changes it. A NOT NULL column with a baked-in
-- default would silently freeze every existing user at install-time values.
--
-- The whole row is ignored while the app-wide toast_allow_user_override flag
-- is off, so an operator can revoke the ability without discarding what people
-- already chose.

ALTER TABLE users ADD COLUMN toast_position TEXT;
ALTER TABLE users ADD COLUMN toast_duration_ms INTEGER;
ALTER TABLE users ADD COLUMN toast_max_visible INTEGER;
ALTER TABLE users ADD COLUMN toast_rich_colors INTEGER;
ALTER TABLE users ADD COLUMN toast_close_button INTEGER;

-- Per-user appearance overrides: theme, accent, interface font, base font size.
--
-- Same contract as the toast columns. Every column is NULL by default and NULL
-- means "inherit the app-wide default" (settings table, category
-- 'appearance'), and the whole row is ignored while
-- appearance_allow_user_override is off.

ALTER TABLE users ADD COLUMN appearance_theme_id TEXT;
ALTER TABLE users ADD COLUMN appearance_accent TEXT;
ALTER TABLE users ADD COLUMN appearance_font TEXT;
ALTER TABLE users ADD COLUMN appearance_base_font_px INTEGER;


-- Step-up re-authentication ("sudo mode"): a short-lived window minted by
-- re-verifying a password or a TOTP code immediately before a sensitive
-- action.
--
-- Bound to the **session**, not the user. That is the whole point: an approved
-- elevation protects against standing over-privilege and against a single
-- insider acting alone, but it does nothing about a stolen session cookie
-- belonging to someone who has one. A window the thief cannot mint without the
-- password or the second factor closes exactly that gap.
--
-- The table is deliberately separate from `sessions`: a session row is read on
-- every request and a sudo window is read on a handful, so widening `sessions`
-- would put a hot column in a cold path. It is also its own lifecycle — a
-- session outlives many sudo windows.
CREATE TABLE IF NOT EXISTS sudo_windows (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- The scope this window was minted for, e.g. 'secrets.reveal'. A window
    -- minted to delete an environment must not silently authorise a credential
    -- disclosure.
    scope      TEXT NOT NULL,
    -- How the user proved it: 'password' or 'totp'.
    method     TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_sudo_windows_lookup
    ON sudo_windows(session_id, scope, expires_at);
CREATE INDEX IF NOT EXISTS idx_sudo_windows_expiry
    ON sudo_windows(expires_at);
CREATE INDEX IF NOT EXISTS idx_sudo_windows_user_id ON sudo_windows(user_id);


CREATE TABLE IF NOT EXISTS table_views (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    table_id    TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('layout', 'filter')),
    name        TEXT NOT NULL DEFAULT '',
    payload     TEXT NOT NULL,
    is_default  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    UNIQUE (user_id, table_id, kind, name)
);

CREATE INDEX IF NOT EXISTS idx_table_views_user_table ON table_views (user_id, table_id);
