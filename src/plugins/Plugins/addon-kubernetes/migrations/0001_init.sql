-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Kubernetes plugin schema.
--
-- The table is prefixed plugin_kubernetes_ so the host's uninstall gate can
-- drop the plugin's storage in one pass, and so the plugin's fenced DB handle
-- will accept the statements at all.
--
-- IF NOT EXISTS is load-bearing here rather than habit. On an install that is
-- upgrading from a build where Kubernetes was still part of the host, this
-- table already exists: host migration 147 created and populated it from the
-- `environments` rows before dropping the Kubernetes columns. That order is
-- forced — host migrations run at boot, plugin migrations run at install, so
-- a plugin migration could never read columns the host had already dropped.
-- On a fresh install nothing has run yet and this statement is what creates
-- the table.

CREATE TABLE IF NOT EXISTS plugin_kubernetes_clusters (
    id           TEXT PRIMARY KEY,
    name         TEXT    NOT NULL,
    -- The namespace the cluster's pages scope to. Empty means all namespaces.
    namespace    TEXT    NOT NULL DEFAULT '',
    server_url   TEXT    NOT NULL DEFAULT '',
    -- Encrypted with the host's AES-256-GCM service through the plugin's
    -- EncryptionHandle. Empty string means "not set", never "empty secret":
    -- the store skips encrypt/decrypt on empty values so a blank column
    -- cannot be mistaken for ciphertext.
    kubeconfig   TEXT    NOT NULL DEFAULT '',
    bearer_token TEXT    NOT NULL DEFAULT '',
    ca_cert      TEXT    NOT NULL DEFAULT '',
    is_default   INTEGER NOT NULL DEFAULT 0,
    is_active    INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL
);

-- The pool resolves the "" / "default" alias on every request that does not
-- name a cluster, which is most of them.
CREATE INDEX IF NOT EXISTS idx_plugin_kubernetes_clusters_default
    ON plugin_kubernetes_clusters (is_default, is_active);
