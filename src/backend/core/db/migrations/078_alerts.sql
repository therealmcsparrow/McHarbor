-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Alerts expansion: registered condition sources, structured conditions,
-- multi-channel destinations, durable firing state, and history.
--
-- Before this migration a rule was a hardcoded `type` string matched in a Go
-- switch, a free-text `condition` parsed by two forgiving regexes, and exactly
-- one `channel_id`. A typo in the condition silently fell back to a default
-- threshold, and "notify Telegram *and* Slack" needed two duplicate rules.
--
-- The old columns (`type`, `condition`, `channel_id`) are deliberately LEFT IN
-- PLACE and still populated. Nothing reads them after this migration, but
-- keeping them makes a rollback to the previous binary a downgrade rather than
-- a data loss.

-- ─── 1. alerts: the rule itself ──────────────────────────────────────
--
-- source_type + condition_key replace `type`. The pair resolves to a condition
-- declared by a registered source — one of the host's built-ins (container,
-- host, platform) or one a plugin contributed. The host deliberately does not
-- constrain these with a CHECK: the valid set is whatever is registered at
-- runtime, and a CHECK would make installing a plugin that adds conditions
-- require a migration.

ALTER TABLE alerts ADD COLUMN source_type TEXT NOT NULL DEFAULT 'container';
ALTER TABLE alerts ADD COLUMN condition_key TEXT NOT NULL DEFAULT '';

-- condition_config is the structured replacement for the free-text condition:
-- {"operator":">","threshold":80,"unit":"%","forSeconds":300,
--  "windowSeconds":0,"params":{}}
-- Stored as JSON rather than columns because the shape varies per condition
-- kind, and a plugin condition may declare fields the host has never heard of.
ALTER TABLE alerts ADD COLUMN condition_config TEXT NOT NULL DEFAULT '{}';

-- environment_ids is a JSON array. Empty string or '[]' means "every active
-- environment", which is what every pre-existing rule did implicitly.
ALTER TABLE alerts ADD COLUMN environment_ids TEXT NOT NULL DEFAULT '';

-- cooldown_seconds = 0 keeps the previous behaviour: notify once per episode,
-- never again until the condition clears and re-fires.
ALTER TABLE alerts ADD COLUMN cooldown_seconds INTEGER NOT NULL DEFAULT 0;

-- notify_on_recovery defaults ON. It is the one behaviour change an existing
-- rule's owner will notice after upgrade — a "resolved" message they did not
-- previously get. Defaulted on because it is the most-requested behaviour and
-- is one toggle to turn off.
ALTER TABLE alerts ADD COLUMN notify_on_recovery INTEGER NOT NULL DEFAULT 1;

-- Empty template means "render the built-in sentence", so existing rules read
-- exactly as they did before.
ALTER TABLE alerts ADD COLUMN message_template TEXT NOT NULL DEFAULT '';

-- ─── 2. alert_destinations: many channels per rule ───────────────────
--
-- No foreign key to communication_channels, on purpose. A channel deleted out
-- from under a rule must leave the rule intact and visibly broken — the engine
-- records a delivery failure and the UI renders the entry as unresolved — not
-- cascade-delete the destination row and leave a rule that looks healthy while
-- silently notifying nobody.
--
-- min_severity NULL means "deliver at any severity". A value ('info',
-- 'warning', 'critical') delivers only when the rule's severity ranks at or
-- above it, which is how one rule pages Telegram on critical while only
-- posting to Slack on warning.

CREATE TABLE IF NOT EXISTS alert_destinations (
    id           TEXT PRIMARY KEY,
    alert_id     TEXT NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    channel_id   TEXT NOT NULL,
    min_severity TEXT,
    created_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_destinations_unique
    ON alert_destinations(alert_id, channel_id);
CREATE INDEX IF NOT EXISTS idx_alert_destinations_alert
    ON alert_destinations(alert_id);

-- ─── 3. alert_states: durable firing state ───────────────────────────
--
-- This state used to be a map[string]engineState field on the Engine struct.
-- In-memory state has two failure modes that matter: a restart re-notifies
-- every still-firing condition, and in an HA cluster a leader failover does
-- the same because the new leader's map is empty. Cooldown (last_notified_at)
-- and sustained-for windows (active_since) both have to outlive the process
-- regardless, so the map moves to SQLite.
--
-- state_key is alertID:environmentID:subjectID. Environment is empty for
-- host-wide (TargetNone) conditions.

CREATE TABLE IF NOT EXISTS alert_states (
    state_key        TEXT PRIMARY KEY,
    alert_id         TEXT NOT NULL,
    active           INTEGER NOT NULL DEFAULT 0,
    active_since     TEXT,
    last_notified_at TEXT,
    last_seen        TEXT NOT NULL,
    created_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_alert_states_alert ON alert_states(alert_id);
CREATE INDEX IF NOT EXISTS idx_alert_states_seen  ON alert_states(last_seen);

-- ─── 4. alert_events: history ────────────────────────────────────────
--
-- alert_id carries no foreign key and alert_name is denormalised so history
-- survives the deletion of the rule that produced it — "what fired last night"
-- is exactly the question you ask after deleting a noisy rule.
--
-- deliveries is a JSON array of {channelId, ok, errorCode}. It records an
-- error *code*, never a provider error string: a webhook failure can echo the
-- token back in its body.

CREATE TABLE IF NOT EXISTS alert_events (
    id               TEXT PRIMARY KEY,
    alert_id         TEXT NOT NULL,
    alert_name       TEXT NOT NULL,
    source_type      TEXT NOT NULL,
    condition_key    TEXT NOT NULL,
    environment_id   TEXT,
    environment_name TEXT,
    subject_id       TEXT,
    subject_name     TEXT,
    state            TEXT NOT NULL,
    severity         TEXT NOT NULL,
    value            REAL,
    value_label      TEXT,
    message          TEXT NOT NULL,
    deliveries       TEXT NOT NULL DEFAULT '[]',
    created_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_alert_events_created ON alert_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_events_alert   ON alert_events(alert_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_events_state   ON alert_events(state, created_at DESC);

-- ─── 5. Backfill: type → (source_type, condition_key) ────────────────
--
-- The five legacy types map onto built-in conditions. `disk` was always a
-- per-environment Docker disk measurement, so it belongs to the `host` source;
-- the rest are container-scoped.

UPDATE alerts
   SET source_type = CASE type
           WHEN 'disk' THEN 'host'
           ELSE 'container'
       END,
       condition_key = CASE type
           WHEN 'cpu'            THEN 'cpu'
           WHEN 'memory'         THEN 'memory'
           WHEN 'disk'           THEN 'docker_disk'
           WHEN 'container_down' THEN 'down'
           WHEN 'image_update'   THEN 'image_update'
           ELSE ''
       END
 WHERE condition_key = '';

-- The free-text `condition` is NOT parsed here. Translating "> 80%" or "5m"
-- into structured JSON means reimplementing the Go regexes in SQL, and two
-- implementations of that grammar would drift. The service backfills a rule's
-- condition_config on first read using the shipped parsers, so both readings
-- are identical by construction. Until then '{}' means "fall back to the
-- condition declaration's defaults", which is what an unparseable condition
-- already did.

-- ─── 6. Backfill: channel_id → alert_destinations ────────────────────
--
-- randomblob(16) rather than an xid: SQLite cannot mint one, and these ids are
-- never parsed for their timestamp. The unique index makes a re-run a no-op.

INSERT OR IGNORE INTO alert_destinations (id, alert_id, channel_id, min_severity, created_at, updated_at)
SELECT lower(hex(randomblob(16))),
       id,
       channel_id,
       NULL,
       CURRENT_TIMESTAMP,
       CURRENT_TIMESTAMP
  FROM alerts
 WHERE channel_id IS NOT NULL
   AND trim(channel_id) <> '';
