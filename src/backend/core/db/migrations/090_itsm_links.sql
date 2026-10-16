-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS incident_links (
    id               TEXT PRIMARY KEY,
    channel_id       TEXT NOT NULL,
    channel_type     TEXT NOT NULL,
    dedup_key        TEXT NOT NULL,
    alert_id         TEXT NOT NULL DEFAULT '',
    environment_id   TEXT NOT NULL DEFAULT '',
    subject_id       TEXT NOT NULL DEFAULT '',
    external_id      TEXT NOT NULL DEFAULT '',
    external_number  TEXT NOT NULL DEFAULT '',
    external_url     TEXT NOT NULL DEFAULT '',
    local_state      TEXT NOT NULL DEFAULT 'pending',
    remote_state     TEXT NOT NULL DEFAULT '',
    remote_assignee  TEXT NOT NULL DEFAULT '',
    last_error       TEXT NOT NULL DEFAULT '',
    opened_at        TEXT,
    resolved_at      TEXT,
    last_synced_at   TEXT,
    created_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_incident_links_live
    ON incident_links(channel_id, dedup_key)
    WHERE local_state IN ('pending', 'open', 'remote_closed');
CREATE INDEX IF NOT EXISTS idx_incident_links_channel ON incident_links(channel_id);
CREATE INDEX IF NOT EXISTS idx_incident_links_alert ON incident_links(alert_id);
CREATE INDEX IF NOT EXISTS idx_incident_links_external ON incident_links(channel_id, external_id);
CREATE INDEX IF NOT EXISTS idx_incident_links_state ON incident_links(local_state);

CREATE TABLE IF NOT EXISTS incident_outbox (
    id               TEXT PRIMARY KEY,
    link_id          TEXT NOT NULL,
    channel_id       TEXT NOT NULL,
    op               TEXT NOT NULL,
    event_json       TEXT NOT NULL,
    seq              INTEGER NOT NULL,
    attempts         INTEGER NOT NULL DEFAULT 0,
    next_attempt_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    last_error       TEXT NOT NULL DEFAULT '',
    parked           INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_incident_outbox_due ON incident_outbox(parked, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_incident_outbox_link ON incident_outbox(link_id, seq);
CREATE INDEX IF NOT EXISTS idx_incident_outbox_channel ON incident_outbox(channel_id);

ALTER TABLE alert_states ADD COLUMN acknowledged_at TEXT;
ALTER TABLE alert_states ADD COLUMN acknowledged_by TEXT NOT NULL DEFAULT '';

ALTER TABLE communication_channels ADD COLUMN webhook_secret_enc TEXT NOT NULL DEFAULT '';
ALTER TABLE communication_channels ADD COLUMN webhook_token_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE communication_channels ADD COLUMN webhook_token_enc TEXT NOT NULL DEFAULT '';
ALTER TABLE communication_channels ADD COLUMN webhook_base_url TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_communication_channels_webhook_token ON communication_channels(webhook_token_hash);
