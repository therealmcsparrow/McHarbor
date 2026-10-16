-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Stack orchestrator kind, and the single-default constraint on
-- environments.

-- Distinguish Compose stacks from Swarm stacks.
--
-- `stacks.type` already means "managed" (row in this DB) vs "discovered"
-- (recovered from Docker labels) and must not be overloaded with a second,
-- orthogonal meaning. Existing rows are Compose by definition, so the default
-- backfills them correctly and no data migration is needed.
--
-- Swarm stacks are derived from the com.docker.stack.namespace label at read
-- time and are not stored here; the column exists so a row that *is* stored
-- can say which orchestrator owns it.

ALTER TABLE stacks ADD COLUMN orchestrator TEXT NOT NULL DEFAULT 'compose';

CREATE INDEX IF NOT EXISTS idx_stacks_orchestrator ON stacks(orchestrator);

-- Opt-in, per stack: deliver every ${secret:NAME} this stack references as a
-- file under /run/secrets and set <VAR>_FILE to its path, instead of putting
-- the resolved value in the container's environment.
--
-- Off by default, and deliberately so. It only works with images that honour
-- the docker-library `_FILE` convention (postgres, mysql, mariadb, wordpress
-- and most of their neighbours do; plenty of others do not), and an image that
-- ignores it will not see the variable at all. That fails loudly at start-up,
-- which is the right direction to fail — but it is not something to turn on
-- for an existing stack without the operator asking.
ALTER TABLE stacks ADD COLUMN secret_file_mode INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_stack_sources_stack ON stack_sources(stack_id);

-- Stack environment variable values move from plaintext to AES-256-GCM
-- ciphertext (core/encryption, `enc:v1:` prefix). The re-encryption itself
-- cannot happen in SQL — it needs the Go encryption service — so only the
-- marker column that tells a migrated row from a legacy one is added here.
-- The backfill runs at start-up (modules/stacks/envsec.go) and is idempotent.
--
-- `value` stays TEXT: ciphertext is base64 and longer than the plaintext it
-- replaces, and the column is not part of any key.
ALTER TABLE stack_environment_variables ADD COLUMN value_encrypted INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_stack_env_vars_plaintext
    ON stack_environment_variables(value_encrypted);


-- Enforce at most one default environment.
--
-- The setup wizard (auth/service.go EnsureLocalEnvironment) and the
-- metrics collector's fallback (metrics/collector.go ensureDefaultEnv)
-- both try to mark a row as the default environment. There was no
-- constraint preventing both from succeeding — a fresh DB that
-- had the __legacy_owner__ placeholder (so setup never ran) ended up
-- with the metrics fallback's "local" row AND a later user-added row,
-- both flagged is_default = 1. The UI then shows two default cards.
--
-- A partial unique index on is_default, scoped to rows where it is 1,
-- is the simplest fix:
--
--   • It does not affect rows with is_default = 0 (every non-default
--     environment has is_default = 0; many rows can share that).
--   • The second INSERT or UPDATE that tries to set is_default = 1
--     fails with UNIQUE constraint failed, which the application
--     surface (Service.Create / Service.SetDefault) already maps to
--     a 409 Conflict.
--
-- Before creating the index we collapse any pre-existing duplicate
-- defaults so the migration is idempotent against already-broken
-- databases: pick the lowest-id default and demote the rest to
-- is_default = 0. Deterministic ordering avoids "which one wins"
-- surprises if this migration is ever re-run after a manual fix.

UPDATE environments
   SET is_default = 0
 WHERE is_default = 1
   AND id NOT IN (
       SELECT id FROM environments WHERE is_default = 1 ORDER BY id ASC LIMIT 1
   );

CREATE UNIQUE INDEX idx_environments_single_default
    ON environments(is_default)
 WHERE is_default = 1;


-- A socket environment being converted to an agent keeps working on its socket
-- until the agent actually authenticates. These two columns are what make that
-- possible: the row still reads as `socket`, so every existing query behaves,
-- while the agent auth path is allowed to match its token and promote the row.
--
-- Without the pending marker the operator would have to choose between a dead
-- environment for however long the install takes, and a row that claims to be
-- an agent before one exists.

ALTER TABLE environments ADD COLUMN pending_connection_type TEXT;
ALTER TABLE environments ADD COLUMN pending_migration_started_at TEXT;

-- The agent auth path looks a token up by hash across both the settled and the
-- pending state, so the hash column carries a second access pattern now.
CREATE INDEX IF NOT EXISTS idx_environments_pending_connection
  ON environments(pending_connection_type);

-- What a stalled migration reverts to.
--
-- An immediate conversion overwrote connection_type and erased the endpoint,
-- so there was nothing left to go back to. This column records the type the
-- environment came from, and the endpoint columns are left intact until the
-- agent actually authenticates.
--
-- Non-NULL means a conversion is in flight and has not proved itself yet, which
-- is what the reverting sweep looks for alongside pending_connection_type.

ALTER TABLE environments ADD COLUMN migration_revert_to TEXT;

CREATE INDEX IF NOT EXISTS idx_environments_migration_in_flight
  ON environments(pending_migration_started_at);

-- How the agent was installed: 'native' or 'container'.
--
-- The difference decides whether the host can run guests at all — guests are
-- supervised as systemd transient units and need QEMU and /dev/kvm, none of
-- which a container agent has — and until now it was invisible everywhere. The
-- only tell was that a container reports its container id as its hostname, so
-- diagnosing "why will this host not run a VM" started from a 12-character hex
-- string rather than from a field.
--
-- NULL means an agent older than 3.5.0, which does not report it. That is
-- rendered as unknown rather than guessed at.
ALTER TABLE environments ADD COLUMN agent_install_kind TEXT;

-- Remembered SSH deployment details for an agent environment.
--
-- These are deliberately NOT the existing ssh_host / ssh_port / ssh_user /
-- ssh_key columns. Those belong to the `ssh` connection type, which reaches a
-- Docker socket through an SSH tunnel; this is how the agent was installed on
-- a host, which is a different question with a different lifetime. Sharing one
-- set of columns would make changing how McHarbor connects silently change how
-- it deploys.
--
-- No credential is stored here. deploy_ssh_secret_id is the id of a row in the
-- secrets vault, and the key or password itself lives there under the vault's
-- own encryption, step-up and audit rules.

ALTER TABLE environments ADD COLUMN deploy_ssh_host TEXT;
ALTER TABLE environments ADD COLUMN deploy_ssh_port INTEGER;
ALTER TABLE environments ADD COLUMN deploy_ssh_user TEXT;
-- key | password. Which of the two the vault secret holds.
ALTER TABLE environments ADD COLUMN deploy_ssh_auth_type TEXT;
ALTER TABLE environments ADD COLUMN deploy_ssh_secret_id TEXT;
-- Pinned on first connect so a later redeploy refuses a host that changed key.
ALTER TABLE environments ADD COLUMN deploy_host_key_fingerprint TEXT;
-- docker | binary, so a redeploy repeats what was done rather than guessing.
ALTER TABLE environments ADD COLUMN deploy_method TEXT;
ALTER TABLE environments ADD COLUMN deploy_last_at TEXT;

ALTER TABLE environments ADD COLUMN host_update_check_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN host_update_check_time TEXT NOT NULL DEFAULT '03:00';
ALTER TABLE environments ADD COLUMN host_update_check_days TEXT NOT NULL DEFAULT 'mon,tue,wed,thu,fri,sat,sun';
ALTER TABLE environments ADD COLUMN host_update_last_checked_at TEXT;
ALTER TABLE environments ADD COLUMN host_update_available INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN host_update_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN host_update_manager TEXT;
ALTER TABLE environments ADD COLUMN host_update_packages TEXT;
ALTER TABLE environments ADD COLUMN host_update_with_new_packages INTEGER NOT NULL DEFAULT 0;


-- Remembers the commit each pipeline stage was last observed at, so the
-- scheduler can tell "the branch moved" from "this is the first time I
-- looked". Without it a poller either promotes on every tick or never
-- promotes at all; the column is what makes change detection possible.
--
-- Deliberately per stage, not per pipeline: stages can track different
-- branches, so a single pipeline-level marker would make a push to one
-- branch look like a change on every stage.

ALTER TABLE gitops_pipeline_stages ADD COLUMN last_seen_commit TEXT;

CREATE INDEX IF NOT EXISTS idx_gitops_stages_last_seen
  ON gitops_pipeline_stages(pipeline_id, last_seen_commit);

CREATE INDEX IF NOT EXISTS idx_gitops_approvals_stage ON gitops_approvals(stage_id);
CREATE INDEX IF NOT EXISTS idx_gitops_promotions_deployment ON gitops_promotions(deployment_id);
CREATE INDEX IF NOT EXISTS idx_gitops_promotions_from_stage ON gitops_promotions(from_stage_id);
CREATE INDEX IF NOT EXISTS idx_gitops_pr_previews_preview_env ON gitops_pr_previews(preview_environment_id);
