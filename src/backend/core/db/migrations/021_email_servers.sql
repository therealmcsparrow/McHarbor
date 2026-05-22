-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS email_servers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    server_type TEXT NOT NULL,
    is_default INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    host TEXT,
    port INTEGER,
    encryption TEXT,
    auth_method TEXT,
    username TEXT,
    password TEXT,
    client_id TEXT,
    client_secret TEXT,
    tenant_id TEXT,
    from_address TEXT NOT NULL,
    from_name TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_email_servers_type ON email_servers(server_type);
CREATE INDEX idx_email_servers_default ON email_servers(is_default);
