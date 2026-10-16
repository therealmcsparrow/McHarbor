-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS cluster_agent_owners (
    id              TEXT PRIMARY KEY,
    environment_id  TEXT NOT NULL UNIQUE,
    node_id         TEXT NOT NULL,
    advertise_url   TEXT NOT NULL DEFAULT '',
    claimed_at      TEXT NOT NULL,
    expires_at      TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cluster_agent_owners_node ON cluster_agent_owners(node_id);
CREATE INDEX IF NOT EXISTS idx_cluster_agent_owners_expires ON cluster_agent_owners(expires_at);

CREATE TABLE IF NOT EXISTS console_tickets (
    id              TEXT PRIMARY KEY,
    digest          TEXT NOT NULL UNIQUE,
    user_id         TEXT NOT NULL,
    environment_id  TEXT NOT NULL,
    guest_kind      TEXT NOT NULL,
    guest_id        TEXT NOT NULL,
    console_kind    TEXT NOT NULL,
    command         TEXT NOT NULL DEFAULT '',
    issued_at       TEXT NOT NULL,
    expires_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_console_tickets_user ON console_tickets(user_id, issued_at);
CREATE INDEX IF NOT EXISTS idx_console_tickets_expires ON console_tickets(expires_at);

ALTER TABLE audit_logs ADD COLUMN exec_node_id TEXT;
CREATE INDEX IF NOT EXISTS idx_audit_logs_exec_node_id ON audit_logs(exec_node_id);
