-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS communication_channels (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    channel_type    TEXT NOT NULL,
    is_default      INTEGER NOT NULL DEFAULT 0,
    enabled         INTEGER NOT NULL DEFAULT 1,
    webhook_url     TEXT,
    server_url      TEXT,
    token           TEXT,
    topic           TEXT,
    chat_id         TEXT,
    phone_number_id TEXT,
    recipient_phone TEXT,
    sender_number   TEXT,
    recipients      TEXT,
    username        TEXT,
    password        TEXT,
    priority        TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX idx_comm_channels_type    ON communication_channels(channel_type);
CREATE INDEX idx_comm_channels_default ON communication_channels(is_default);
