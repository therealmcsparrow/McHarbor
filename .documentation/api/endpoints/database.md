# Database

11 endpoints: the running database: its configuration, a connection test, the health
check and repair, and migrating the data onto another backend.

| Key | Covers |
|---|---|
| `database.view` | The active configuration, health-check status, migration status |
| `database.manage` | Connection test, starting and cancelling a check, repair, migration, cleanup |

Both keys are checked **globally**. The first-run database wizard (`/api/database/setup/*`) is
a separate, pre-account surface on [System, updates and licence](./system.md); it is guarded by
the one-time setup token in the `X-McHarbor-Setup-Token` header rather than by a key, and
confines a SQLite path to the data directory.

## Things worth knowing first

**A migration ends in a restart.** McHarbor reads its connection once, at boot, so a new
database is put into service by restarting the container. `restartAfter` defaults to true; an
install that cannot restart itself says so, and the operator restarts by hand.

**The status routes are what the dialogs poll.** `GET /api/database/check` and
`GET /api/database/migration` report the current or most recent run; the migration status stays
reachable while the maintenance pause is up.

**Testing a connection dials an arbitrary host** from inside the McHarbor container, which is why
`POST /api/database/test` takes `database.manage` rather than `view`. Its response carries
sanitized driver diagnostics.

**Cleanup is irreversible.** `POST /api/database/cleanup` deletes the SQLite file a completed
migration left behind; the service re-checks its own guard rather than trusting the status the
browser was holding.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/database/check` | `database.view` | Reports the current or most recent health-check run; the progress dialog polls it | — |
| `POST` | `/api/database/check` | `database.manage` | Starts a health-check run. `manage`, not `view`: a full sweep is real load on the shared database | — |
| `POST` | `/api/database/check/cancel` | `database.manage` | Asks a running check to stop | — |
| `POST` | `/api/database/check/repair` | `database.manage` | Applies the remedies the operator selected | — |
| `GET` | `/api/database/cleanup` | `database.manage` | Reports whether an old SQLite file left by a migration is on disk and may be deleted | — |
| `POST` | `/api/database/cleanup` | `database.manage` | Deletes the SQLite file a completed migration left behind. Irreversible; the service re-checks its own guard | — |
| `GET` | `/api/database/config` | `database.view` | Returns the active database, where its configuration came from, and a size readout | — |
| `GET` | `/api/database/migration` | `database.view` | Reports the current or most recent migration job; reachable while the maintenance pause is up | — |
| `POST` | `/api/database/migration` | `database.manage` | Starts a data migration onto another backend | — |
| `POST` | `/api/database/migration/cancel` | `database.manage` | Asks a running migration to stop | — |
| `POST` | `/api/database/test` | `database.manage` | Runs the step-by-step connection test against a candidate database and returns sanitized driver diagnostics | — |

## Request bodies

**`POST` `/api/database/migration`**: `target`, `restartAfter`?

**`POST` `/api/database/test`**: `driver`, `host`?, `port`?, `name`?, `user`?, `password`?, `sslMode`?, `params`?, `dsn`?, `path`?

## Related

- [System, updates and licence](./system.md): the first-run database wizard
- [Database settings](../../settings/system/database/README.md): the same operations from the interface
