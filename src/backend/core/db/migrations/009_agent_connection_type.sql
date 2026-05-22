-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- SQLite requires table rebuild to modify CHECK constraints.
-- Add 'agent' to the connection_type CHECK constraint.

CREATE TABLE environments_new (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    connection_type TEXT NOT NULL DEFAULT 'socket' CHECK(connection_type IN ('socket','tcp','tls','ssh','podman','agent')),
    socket_path TEXT,
    host TEXT,
    port INTEGER,
    tls_ca TEXT,
    tls_cert TEXT,
    tls_key TEXT,
    ssh_host TEXT,
    ssh_port INTEGER,
    ssh_user TEXT,
    ssh_key TEXT,
    is_default INTEGER NOT NULL DEFAULT 0,
    is_active INTEGER NOT NULL DEFAULT 1,
    docker_version TEXT,
    last_connected TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    orchestrator_type TEXT NOT NULL DEFAULT 'docker' CHECK(orchestrator_type IN ('docker','kubernetes')),
    kubeconfig TEXT,
    k8s_namespace TEXT DEFAULT 'default',
    k8s_server_url TEXT,
    k8s_bearer_token TEXT,
    k8s_ca_cert TEXT,
    k8s_version TEXT,
    agent_token TEXT,
    agent_status TEXT DEFAULT 'disconnected',
    agent_version TEXT,
    agent_hostname TEXT,
    agent_os TEXT,
    agent_arch TEXT,
    agent_last_seen TEXT
);

INSERT INTO environments_new SELECT * FROM environments;
DROP TABLE environments;
ALTER TABLE environments_new RENAME TO environments;

CREATE INDEX IF NOT EXISTS idx_environments_default ON environments(is_default);
CREATE INDEX IF NOT EXISTS idx_environments_orchestrator ON environments(orchestrator_type);
CREATE INDEX IF NOT EXISTS idx_environments_agent_token ON environments(agent_token);
