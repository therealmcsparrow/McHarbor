-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Licensing: identity, entitlement, accounting, and the notice ledger.
--
-- The design deliberately blocks nothing. These four tables exist so an
-- over-used install can be *told* (license_notices), so the facts survive to
-- the renewal conversation (license_usage_samples), and so the license server
-- can recognise this install across restarts (license_installs). No runtime
-- path reads them to decide whether a request is allowed.
--
-- No partial indexes: this directory is applied verbatim on SQLite, Postgres
-- and MySQL, and `CREATE INDEX ... WHERE` is not portable. The "exactly one
-- active license" rule is enforced in the service, inside a transaction.

-- The install's own identity. One row, ever.
CREATE TABLE IF NOT EXISTS license_installs (
    id          TEXT PRIMARY KEY,
    install_key TEXT NOT NULL,
    key_masked  TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

-- The licence server at https://www.mcharbor.eu issues keys out of band and
-- the install claims a seat against one. The seat allowance, the term and the
-- check-in verdict come back on every call.
CREATE TABLE IF NOT EXISTS licenses (
    id                  TEXT PRIMARY KEY,
    -- The licence key, encrypted at rest. The masked form is what a human
    -- ever sees, including on a support call.
    license_key         TEXT NOT NULL,
    key_masked          TEXT NOT NULL DEFAULT '',
    -- The licence server's own id for this licence.
    server_license_id   TEXT NOT NULL DEFAULT '',
    tier                TEXT NOT NULL DEFAULT 'unlicensed',
    status              TEXT NOT NULL DEFAULT 'pending',
    -- The last verdict. `valid` is the field that decides, not any HTTP
    -- status: a lapsed licence is a successful answer carrying a reason.
    valid               INTEGER NOT NULL DEFAULT 0,
    reason              TEXT,
    -- Seats. `seats_unlimited` outranks `seats_total`.
    seats_total         INTEGER NOT NULL DEFAULT 0,
    seats_used          INTEGER NOT NULL DEFAULT 0,
    seats_unlimited     INTEGER NOT NULL DEFAULT 0,
    -- Term. `perpetual` outranks `expires_at`.
    perpetual           INTEGER NOT NULL DEFAULT 0,
    starts_at           TEXT,
    expires_at          TEXT,
    days_remaining      INTEGER,
    auto_renew          INTEGER NOT NULL DEFAULT 0,
    -- This install's seat, as the licence server last described it. A
    -- registered instance that stops holding a seat is the signal to
    -- activate again, not to keep checking in against a seat we lost.
    instance_registered INTEGER NOT NULL DEFAULT 0,
    holds_seat          INTEGER NOT NULL DEFAULT 0,
    activated_at        TEXT NOT NULL,
    last_checked_at     TEXT,
    last_error          TEXT,
    activated_by        TEXT,
    active              INTEGER NOT NULL DEFAULT 1,
    created_at          TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at          TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_licenses_active ON licenses(active);
CREATE INDEX IF NOT EXISTS idx_licenses_server_id ON licenses(server_license_id);

-- Append-only usage record. This is the renewal evidence.
--
-- The seat is the instance id, so the ceiling column is seats, not a
-- `servers` entitlement. The column keeps its name — it counts the same
-- thing, and renaming it across three drivers buys nothing.
CREATE TABLE IF NOT EXISTS license_usage_samples (
    id            TEXT PRIMARY KEY,
    license_id    TEXT,
    servers_used  INTEGER NOT NULL DEFAULT 0,
    servers_limit INTEGER NOT NULL DEFAULT -1,
    agent_count   INTEGER NOT NULL DEFAULT 0,
    env_count     INTEGER NOT NULL DEFAULT 0,
    user_count    INTEGER NOT NULL DEFAULT 0,
    state         TEXT NOT NULL,
    over_since    TEXT,
    reported      INTEGER NOT NULL DEFAULT 0,
    reported_at   TEXT,
    sampled_at    TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_license_usage_sampled ON license_usage_samples(sampled_at);
CREATE INDEX IF NOT EXISTS idx_license_usage_reported ON license_usage_samples(reported, sampled_at);

-- Banner showings. Drives the daily-for-14-days-then-hourly cadence and
-- proves what the customer was shown, and when.
CREATE TABLE IF NOT EXISTS license_notices (
    id            TEXT PRIMARY KEY,
    kind          TEXT NOT NULL,
    phase         TEXT NOT NULL,
    servers_used  INTEGER NOT NULL DEFAULT 0,
    servers_limit INTEGER NOT NULL DEFAULT -1,
    shown_at      TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_license_notices_kind_shown ON license_notices(kind, shown_at);

-- Defaults. The instance endpoints need no API key: the licence key is the
-- credential.
INSERT INTO settings (id, key, value, category, created_at, updated_at)
SELECT 'set_license_server_url', 'license.server_url', 'https://www.mcharbor.eu', 'license', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'license.server_url');

-- Daily is the cadence the handshake asks for, and the check-in doubles as
-- the heartbeat.
INSERT INTO settings (id, key, value, category, created_at, updated_at)
SELECT 'set_license_heartbeat_hours', 'license.heartbeat_interval_hours', '24', 'license', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'license.heartbeat_interval_hours');

INSERT INTO settings (id, key, value, category, created_at, updated_at)
SELECT 'set_license_sample_minutes', 'license.sample_interval_minutes', '60', 'license', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'license.sample_interval_minutes');

INSERT INTO settings (id, key, value, category, created_at, updated_at)
SELECT 'set_license_offline_days', 'license.offline_tolerance_days', '30', 'license', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'license.offline_tolerance_days');

-- Where a banner sends somebody who needs to renew, extend, or ask about a
-- suspension. The handshake returns no per-licence contact URL, so this is a
-- setting with a sensible default rather than a field nobody fills in.
INSERT INTO settings (id, key, value, category, created_at, updated_at)
SELECT 'set_license_contact_url', 'license.contact_url', 'https://www.mcharbor.eu/licenses', 'license', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'license.contact_url');
