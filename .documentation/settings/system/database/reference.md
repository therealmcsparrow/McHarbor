# Database: Reference

| Setting | Default | What it does |
|---|---|---|
| **Database backend** | The backend McHarbor is running on, badged **In use** | SQLite, PostgreSQL, MySQL / MariaDB, or Microsoft SQL Server. The in-use backend is not a copy target: choose another to open its form |
| **Host, Port, Database name, User, Password** | — | For the external drivers. Port defaults to 5432, 3306 or 1433 |
| **SSL mode** / **TLS** / **Encryption** | `require` / `preferred` / `true` | "The connection carries every credential McHarbor holds. Only turn encryption off on a network you control end to end" |
| **Trust the server certificate** | Off | SQL Server only. Skips verifying a self-signed certificate; the connection stays encrypted |
| **File path** | `$DATA_DIR/mcharbor.db` | For SQLite. "Leave empty for the configured default" |
| **Enter a connection string instead** | Off | A raw DSN for managed databases. "Every field above is ignored while this is on" |
| **Restart automatically when the migration finishes** | On | McHarbor reads its connection once, at start-up |

Credentials are "encrypted with the McHarbor secret key and stored in the data directory —
never in an environment variable, and never sent back to this page."

**Test connection** runs named steps rather than one pass/fail, so a wrong password is
distinguishable from an unreachable host.

Two states the screen calls out: **pinned by the environment**, when `MCHARBOR_DB_DRIVER`
is set and anything chosen here would be ignored at the next start; and **no restart
available**, when no local Docker socket is reachable and you must restart the container
yourself.

## What each backend is verified against

The complete migration set (82 files) is applied from empty against **SQLite**,
**PostgreSQL 16**, **MySQL 8** and **MariaDB 10.6** as part of the test suite, and against
**SQL Server 2022**. Copying a SQLite database into SQL Server 2022 through **Migrate** is
covered by a test that runs wherever `MCHARBOR_TEST_MSSQL_DSN` names a throwaway database.
So is copying back out of SQL Server into SQLite, and, where `MCHARBOR_TEST_PG_DSN` and
`MCHARBOR_TEST_MYSQL_DSN` name a server, into PostgreSQL and MySQL, each checked table by
table against the source's row counts.

**Test connection** refuses a server older than its minimum: PostgreSQL 12, MySQL 8,
MariaDB 10.6, SQL Server 2022 (engine build 16). The
migration files themselves are written in SQLite's dialect and are never rewritten;
statements are translated as they are applied, and SQLite receives every file byte for
byte.

Four things are deliberately **skipped on the external drivers**, each logged with the
migration and the statement that was skipped:

| Skipped | Where | What it costs |
|---|---|---|
| The audit-log WORM triggers | `072_audit.sql` | On PostgreSQL and MySQL the `audit.worm` setting has no *database-level* enforcement. The application-level audit path (write-once storage and the hash chain) is unaffected. |
| The API-key archive trigger | `080_api_keys.sql` | A deleted key is still archived to a tombstone by the API-keys service on both delete paths. What is lost is the backstop for a key removed outside the service, such as a manual repair. |
| One expression index over `COALESCE(...)` | `034_stack_role_scope.sql` | MariaDB has no functional key parts, so the uniqueness that index enforced is not enforced by the database there. |
| Conditional permission-key renames | `067_rbac.sql` | Upgrade repair for an older SQLite install. A no-op on the fresh install that is the only kind these drivers can currently have. |
| The audit environment index over `COALESCE(environment_id, '')` | `120_skewed_data_indexes.sql` | MySQL, MariaDB and SQL Server only. No data is lost; the audit list filtered by environment reads the timestamp index instead. The health check reports this index as missing there. |

And one semantic difference rather than a skip: `ON DELETE SET NULL` on a `NOT NULL`
column becomes `NO ACTION` on MySQL and MariaDB, so deleting the referenced row is refused
rather than nulling the column.

## Whole numbers are 64-bit on every backend

SQLite's `INTEGER` is 64-bit. PostgreSQL's and MySQL's are 32-bit, which tops out at
2,147,483,647, about 2 GiB as a byte count. The migration files are written against SQLite,
so on the external drivers every `INTEGER` column is created as `BIGINT`, and a runtime
`CAST(… AS INTEGER)` becomes a cast to `BIGINT` on PostgreSQL. SQL Server has always received
`BIGINT`.

A PostgreSQL or MySQL database created by an earlier build still has 32-bit columns.
`119_widen_integer_columns.sql` widens every one of them to `BIGINT` on the first start after
the upgrade, keeping each column's `NOT NULL` and default. The widening rewrites each affected
table once, so on a large database that first start can take a few minutes. A column a view
or a foreign key depends on is left as it is, and its name is logged. On SQLite the migration
changes nothing.

Before this, **Migrate** to PostgreSQL failed with `unable to encode … into binary format for
int4`, and to MySQL with `Out of range value for column`, on the first row holding a value
above 2 GiB. On a real install that is usually the first disk-capacity sample.

## Planner statistics on SQLite

SQLite chooses indexes from the statistics in `sqlite_stat1`, and a database that has never
been analyzed has none. McHarbor runs `PRAGMA optimize=0x10002` once when it opens the
database, and the metrics collector runs it once a day on the leader after the retention
prunes. The pragma re-analyzes only tables whose statistics are missing or stale, and SQLite
limits how much of each index it reads. A run is capped at 60 seconds, and a failure is
logged without stopping anything. PostgreSQL (autovacuum) and MySQL (InnoDB persistent
statistics) are left alone.

## The migration files

The files live in `src/backend/core/db/migrations/`, are embedded in the binary, and run
in filename order at every start-up. `schema_migrations` records each file by name once it
has applied, so a file runs exactly once per database. `001` to `064` are the 2.0.8 schema;
`065` to `081` are this release, one file per area:

| File | What it adds |
|---|---|
| `065_settings.sql` | Removes settings rows stored without an id; seeds the lockout, password policy, password-reset (TTL, minimum length, email channel, sender, subject, body) and idle sign-out defaults |
| `066_accounts.sql` | MFA secrets and login challenges, password history and reset tokens, login attempts, account disable / lockout columns on `users`, session metadata and `last_activity_at` on `sessions`, step-up windows (`sudo_windows`), per-user toast and appearance preferences, saved table views, case-insensitive usernames, the `__legacy_owner__` placeholder |
| `067_rbac.sql` | The permission catalog, per-user grants and denies, ABAC policies and their seeded library, role presets, elevation requests, plugin permission declarations, `owner_user_id` on stacks and backup plans, and the grants the seeded roles receive |
| `068_backups_storage.sql` | Free-form `config` on storage locations, snapshot configurations, per-plan consistency mode and capture rules, per-run snapshot, name and item columns, provider rows in the backup log |
| `069_secrets.sql` | The secrets vault (`secrets`, `secret_versions`, `secret_references`) and a vault pointer beside every operator-typed credential column |
| `070_stacks_environments_gitops.sql` | Stack orchestrator and secret file mode, encrypted stack variables, a single default environment, connection-migration, agent install, SSH deployment and host-update columns on `environments`, and the GitOps last-seen commit |
| `071_kubernetes_swarm_to_plugins.sql` | Hands Kubernetes clusters to the `kubernetes` plugin and drops the host's Kubernetes columns and grants; records whether Swarm was in use for the `addon-swarm` auto-installer |
| `072_audit.sql` | Severity, state diff, hash chain and environment name on `audit_logs`, the WORM triggers, retention presets, the action catalog and route map, and log forwarders |
| `073_identity.sql` | The identity-provider table in its plugin-owned shape, and SCIM endpoints, sync log and user mappings |
| `074_plugins_appstore.sql` | Installed plugins and every plugin contribution table (routes, menu, widgets, nodes, topbar, context menus, tabs, themes, config, notification adapters, catalogs, migrations), settings catalogs, the App Store catalog, install records and registry cache, and the plugin audit actions |
| `075_ai.sql` | AI assistant conversations, messages, usage counters and action log, and AI provider connections with API-key and device-flow sign-in |
| `076_communications_notifications.sql` | Plugin-driven communication channel config, and the `error` level on in-app notifications |
| `077_observability_history.sql` | Container exec history, health probes, uptime samples and daily rollups, tiered container and host metrics (with GPU), agent health samples and logs, and RFC3339 lifecycle-event timestamps with compute subject types |
| `078_alerts.sql` | Registered condition sources, structured conditions, multiple destinations per rule, durable firing state and alert history |
| `079_scanners.sql` | Per-instance scanner configuration, carried over from the `scanners` settings category |
| `080_api_keys.sql` | API-key roles, environment fences, usage history, deleted-key tombstones and their retention |
| `081_license.sql` | License identity, the active license, usage samples, banner notices and license server defaults |

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| The screen refuses to manage the database | `MCHARBOR_DB_DRIVER` is set on the container | Remove that variable to manage it from here |
| The migration finished but nothing changed | McHarbor reads its connection once, at start-up | Restart; the screen offers to do it where a socket is available |
| Something looks wrong after migrating | The new database is in use; the old one is untouched | Remove the stored configuration to go straight back |
| Test fails at a specific step | Each step is reported separately | Read which step (details, host, credentials) rather than guessing |
| Migrate stopped with `unable to encode … int4` or `Out of range value` | The build that ran it created 32-bit columns on the target | Upgrade and run **Migrate** again: the target is migrated before any row is copied, so `119_widen_integer_columns.sql` widens its columns first |
| **Test connection** and **Migrate data** are disabled, with no form | The selected card is the backend in use | Choose a different backend |
| SQL Server: **Version supported** fails naming `XACT_ABORT` | The login or connector sets `XACT_ABORT ON`, which McHarbor refuses at start-up | Remove it from the login's or connector's session settings |
| SQL Server: **Version supported** warns about `READ_COMMITTED_SNAPSHOT` | The database still takes shared read locks | Run the `ALTER DATABASE` statement the warning shows |
| SQL Server: **Connection accepted** fails with a certificate error | The server's certificate is self-signed | Turn on **Trust the server certificate**, or install a certificate the container trusts |

## Related

- [Database](./README.md): what the section is for
- [Settings](../../README.md): the other groups
