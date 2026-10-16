-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Hand Kubernetes over to the add-on plugin: carry the clusters, then drop
-- everything the host no longer needs, in one pass over `environments`
-- rather than two.

-- Kubernetes left the host in this release: `modules/kubernetes`,
-- `core/kubernetes` and `k8s.io/client-go` are gone, and the feature ships as
-- the bundled `kubernetes` plugin instead. Three things have to happen in the
-- database for that to be true, and they have to happen here rather than in
-- the plugin's own migration.
--
-- WHY HERE. Host migrations run at boot; plugin migrations run at install. A
-- plugin migration therefore cannot read `environments.kubeconfig` — by the
-- time it runs, this file has already dropped the column. So the hand-off is
-- the host's job, and the plugin's `0001_init.sql` is written
-- `CREATE TABLE IF NOT EXISTS` so it is a no-op on an install that came
-- through here and the creating statement on a fresh one.
--
-- WHAT IS CARRIED. The *ciphertext*, not the plaintext. The plugin's
-- `Encryption` handle is the host's own AES-256-GCM service, so what the host
-- encrypted the plugin decrypts; no credential is ever in the clear during the
-- move, and no re-encryption pass is needed.

-- 1. The plugin's cluster table, and the rows to fill it.
CREATE TABLE IF NOT EXISTS plugin_kubernetes_clusters (
    id           TEXT PRIMARY KEY,
    name         TEXT    NOT NULL,
    namespace    TEXT    NOT NULL DEFAULT '',
    server_url   TEXT    NOT NULL DEFAULT '',
    kubeconfig   TEXT    NOT NULL DEFAULT '',
    bearer_token TEXT    NOT NULL DEFAULT '',
    ca_cert      TEXT    NOT NULL DEFAULT '',
    is_default   INTEGER NOT NULL DEFAULT 0,
    is_active    INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_kubernetes_clusters_default
    ON plugin_kubernetes_clusters (is_default, is_active);

-- The cluster keeps its environment id, so anything that referenced a cluster
-- by id — an audit entry, a saved link — still names the same thing.
-- `is_default` is recomputed rather than copied: the host's default flag was
-- shared with Docker hosts, so a Kubernetes environment was almost never the
-- one carrying it.
INSERT OR IGNORE INTO plugin_kubernetes_clusters
    (id, name, namespace, server_url, kubeconfig, bearer_token, ca_cert,
     is_default, is_active, created_at, updated_at)
SELECT
    id,
    name,
    COALESCE(k8s_namespace, ''),
    COALESCE(k8s_server_url, ''),
    COALESCE(kubeconfig, ''),
    COALESCE(k8s_bearer_token, ''),
    COALESCE(k8s_ca_cert, ''),
    0,
    COALESCE(is_active, 1),
    COALESCE(created_at, datetime('now')),
    COALESCE(updated_at, datetime('now'))
FROM environments
WHERE orchestrator_type = 'kubernetes';

-- The oldest active cluster becomes the default, so the plugin's pool can
-- resolve the "" alias on the very first request after the upgrade.
UPDATE plugin_kubernetes_clusters
SET is_default = 1
WHERE id = (
    SELECT id FROM plugin_kubernetes_clusters
    WHERE is_active = 1
    ORDER BY created_at ASC
    LIMIT 1
);

-- 2. Clear a dangling foreign key that blocks everything below.
--
-- `environment_notifications` was created by migration 001 with two foreign
-- keys: one to `environments` (ON DELETE CASCADE) and one to
-- `notification_channels`, which is dropped when notifications move out to
-- plugins. No Go code references it.
--
-- With `foreign_keys=ON` — which is how the host opens the database — the
-- DELETE in step 3 cascades into `environment_notifications`, and resolving
-- *that* row's own foreign keys fails on the missing parent. The error names
-- `notification_channels`: neither of the tables this migration is actually
-- touching, which is what makes it so confusing to read. The same fault would
-- block the `ALTER TABLE` in step 4 for the same reason: SQLite re-parses the
-- whole schema during DROP COLUMN, and that table's foreign key failed the
-- parse.
DROP TABLE IF EXISTS environment_notifications;

-- 3. Kubernetes environments are no longer environments.
--
-- They are cluster rows in the table above now, and leaving them behind would
-- put a row on the Docker environments page that no Docker code can connect
-- to. Deleted only after the INSERT above has copied them.
DELETE FROM environments WHERE orchestrator_type = 'kubernetes';

-- 3b. Record whether this install had clusters, for the auto-installer.
--
-- The bundled Kubernetes plugin is installed automatically on exactly one
-- condition: this upgrade carried clusters over. A fresh install, or an
-- upgrade by someone who only ever ran Docker, gets nothing — the plugin sits
-- in the App Store like every other optional one.
--
-- Writing the count rather than a boolean makes the reason legible later: "0"
-- says the check ran and found nothing, which is different from the key being
-- absent because the migration never ran.
INSERT INTO settings (id, key, value, category, created_at, updated_at)
SELECT
    'kubernetes_clusters_migrated',
    'kubernetes_clusters_migrated',
    CAST((SELECT COUNT(*) FROM plugin_kubernetes_clusters) AS TEXT),
    'plugins',
    datetime('now'),
    datetime('now')
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'kubernetes_clusters_migrated');

-- 4. Drop the five credential columns.
--
-- `k8s_version` goes with them: it was a cache of the cluster's reported
-- version, and the plugin reads that live from /version on every summary.
--
-- `orchestrator_type` only ever meant "is this Kubernetes?", and with the
-- clusters gone it has one possible value, so it goes in the same pass. It
-- was checked rather than assumed:
--
-- * Swarm does not use it — a Swarm is a Docker environment whose daemon
--   reports swarm mode, discovered by a capability probe, not by a column.
-- * `stacks.orchestrator` is a different column on a different table,
--   distinguishing Compose stacks from Swarm stacks, and is untouched.
-- * Every remaining reader in the Go code was `WHERE orchestrator_type =
--   'docker'` — a filter that matched every row — across metrics, autoheal,
--   alerts, activity, workflows and the container health collector.
--
-- The index has to go first. SQLite refuses to drop a column an index still
-- references, and reports it as "error in index idx_environments_orchestrator
-- after drop column" — which reads like index corruption rather than a missing
-- prerequisite. `idx_stacks_orchestrator` is a different index on a different
-- table and is deliberately left alone.
DROP INDEX IF EXISTS idx_environments_orchestrator;

ALTER TABLE environments DROP COLUMN kubeconfig;
ALTER TABLE environments DROP COLUMN k8s_namespace;
ALTER TABLE environments DROP COLUMN k8s_server_url;
ALTER TABLE environments DROP COLUMN k8s_bearer_token;
ALTER TABLE environments DROP COLUMN k8s_ca_cert;
ALTER TABLE environments DROP COLUMN k8s_version;
ALTER TABLE environments DROP COLUMN orchestrator_type;

-- 5. Retire the ten host permission keys from the roles.
--
-- The plugin declares kubernetes.view / .manage / .delete and
-- kubernetes.clusters.manage through `adds_permissions`. The old keys are
-- removed rather than mapped: per the migration decision the grants are
-- dropped rather than translated — an administrator re-assigns Kubernetes
-- access after the upgrade.
--
-- A role's grants are a JSON array in `roles.permissions`, not rows in a join
-- table, so the removal is a rebuild of that array with the ten keys filtered
-- out. `role_admin` is untouched by construction: its permissions are '["*"]'
-- and the wildcard matches nothing in the list below.
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value)
         FROM json_each(roles.permissions)
        WHERE value NOT IN (
            'pods.view', 'pods.manage', 'pods.delete',
            'deployments.view', 'deployments.manage', 'deployments.delete',
            'k8s_services.view', 'k8s_services.manage', 'k8s_services.delete',
            'namespaces.view'
        )
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE EXISTS (
       SELECT 1 FROM json_each(roles.permissions)
        WHERE value IN (
            'pods.view', 'pods.manage', 'pods.delete',
            'deployments.view', 'deployments.manage', 'deployments.delete',
            'k8s_services.view', 'k8s_services.manage', 'k8s_services.delete',
            'namespaces.view'
        )
 );


-- Hand Docker Swarm over to the `addon-swarm` plugin.
--
-- Swarm never had a table. It was never an environment type, never a column,
-- never a credential — a Docker environment is Swarm-capable when its daemon
-- says so, discovered by a probe that can flip at runtime. So unlike
-- Kubernetes there is nothing to move.
--
-- The grants are deliberately left in `roles.permissions`. Kubernetes renamed
-- ten keys down to four; a grant naming `pods.view` after that upgrade named
-- nothing. Swarm renames nothing: the plugin's `adds_permissions` declares the
-- same twenty strings, so an operator who granted `swarm_nodes.manage` still
-- has it — the key's *origin* moved from the host catalog to
-- `plugin_permissions`, and a grant does not care which table defined the key
-- it names. On an install that never adds the plugin those grants name keys no
-- catalog defines, which is inert — `scopeFor` misses, no route consults
-- them, and the role editor simply does not list them — and installing the
-- add-on later restores every grant exactly as it was.
--
-- Record whether this install had a Swarm, for the auto-installer: the bundled
-- add-on installs itself automatically only where the feature was actually in
-- use. The signal is whether anyone was granted a Swarm permission beyond the
-- presets. Writing the count rather than a boolean keeps the reason legible
-- later: "0" says the check ran and found nothing.
INSERT INTO settings (id, key, value, category, created_at, updated_at)
SELECT
    'swarm_grants_migrated',
    'swarm_grants_migrated',
    CAST((
        SELECT COUNT(*) FROM user_permissions
         WHERE permission_key LIKE 'swarm.%' OR permission_key LIKE 'swarm\_%' ESCAPE '\'
    ) AS TEXT),
    'plugins',
    datetime('now'),
    datetime('now')
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'swarm_grants_migrated');
