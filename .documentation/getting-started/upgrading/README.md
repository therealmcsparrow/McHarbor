# Upgrading

McHarbor upgrades by replacing the container with a newer image. The data volume carries
everything across; the image carries nothing.

## Before you upgrade

1. **Back up the data volume.** It holds the database, the session secret, TLS material and
   every installed plugin. Everything else is reproducible; this is not.
2. **Know your secrets.** `MCHARBOR_SECRET`, `ENCRYPTION_KEY` and the backup key are what
   make the volume readable.
3. **Read the [release notes](../changelog/README.md)** for every version you are skipping,
   not just the one you are moving to.

## The upgrade

```bash
docker compose pull
docker compose up -d
docker compose logs -f mcharbor
```

Migrations run at start-up, before the listeners open. The log names each one. A migration
failure stops the boot rather than serving a half-migrated database, which is the behaviour
you want, and the reason step 1 exists.

## Credentials are encrypted on the first start

Credential columns written by an older version in plaintext (registry and storage-location
passwords, tokens, webhook secrets, identity-provider secrets, MFA secrets, AI provider keys and
the rest of the list in `src/backend/internal/atrest`) are encrypted in place on the first start of
the new version, with the module backfills for stack environment variables, snapshot
configurations, reconciler desired states and Kubernetes cluster credentials. The pass is
idempotent: a value that is already encrypted is left alone, so every later start changes nothing.
It needs `ENCRYPTION_KEY` (or `ENCRYPTION_KEY_FILE`). Without one the pass is skipped with a log
line (`credential at-rest backfill skipped: no encryption service`), nothing is converted, and the
same pass runs on the first start that has a key. A write that would have to store a credential
while encryption is unavailable fails instead of storing plaintext. A pass that fails on one column
logs it and does not stop McHarbor from starting. Keep the key unchanged across the upgrade, or the
converted values become unreadable.

## HA clusters on the Compose template

The `docker-compose.ha.yml` template reads the session secret, the encryption key and the
database connection string from Docker secrets (`MCHARBOR_SECRET_FILE`, `ENCRYPTION_KEY_FILE`,
`MCHARBOR_DB_DSN_FILE`) instead of variables. Before recreating a cluster from the new
template, write the values it **already runs with** into `secrets/mcharbor_secret`,
`secrets/mcharbor_encryption_key` and `secrets/mcharbor_db_dsn`, remove `MCHARBOR_SECRET` and
`ENCRYPTION_KEY` from `ha.env`, and only then run `docker compose up -d`. New values end every
session and leave the database unreadable; a missing file stops `up` before any node starts; a
node given both a variable and its `_FILE` form refuses to start. The commands are on
[Shared keys as Docker secrets](../../settings/system/high-availability/README.md#shared-keys-as-docker-secrets).
A cluster that keeps its own compose file is unaffected until it adopts the `_FILE` forms.

The bundled PostgreSQL follows the same pattern: the template now sets
`POSTGRES_PASSWORD_FILE=/run/secrets/postgres_password` instead of `POSTGRES_PASSWORD`, and
`ha.env` no longer carries the password. **Existing deployments write their current
PostgreSQL password into `secrets/postgres_password` before recreating**:

```bash
printf '%s' "$POSTGRES_PASSWORD" > secrets/postgres_password
chmod 0400 secrets/postgres_password
```

PostgreSQL reads the password only when it first initialises an empty data volume, so a wrong
value changes nothing in an existing database; but the password inside `secrets/mcharbor_db_dsn`
must match the real one, and `postgres_password` should match it too so the two files do not
disagree later. A missing `secrets/postgres_password` stops `docker compose up` before any
container starts. The same change applies to
`deploy/compose/external-database/docker-compose.yml`, which now also reads its connection
string from `secrets/mcharbor_db_dsn`: create both files as described on
[External database](../deployment/compose.md#external-database), then remove `POSTGRES_PASSWORD`
and `MCHARBOR_DB_DSN` from its `.env`.

## What happens to plugins

Plugins are compiled Go shared objects loaded into the host process, and Go checks a
per-package ABI hash when it opens one. A rebuilt host binary can therefore reject a plugin
that worked yesterday.

The entrypoint handles this for **bundled** plugins. For every `/tmp/<slug>.so` in the image
whose install directory exists on disk, it copies the bundled `.so` over the installed one,
and, when the tarball matches the installed version, re-extracts the whole payload, so the
frontend bundle, the i18n namespaces and the icons move with it.

That second half matters: a rebuild that changes a plugin's page ships a new UMD bundle, and
copying only the `.so` would leave the old page rendering indefinitely.

Plugins **not** bundled in the image are not refreshed this way. If one fails to load after
an upgrade, reinstall it from the App Store against the new host version. See
[Plugins → Architecture](../../development/plugins/architecture.md).

## Upgrading from the interface

[Settings → Updates → McHarbor](../../settings/updates/mcharbor/README.md) can do this for
you: a release checker on an interval, **Update now**, and an automatic-install switch that
does nothing while the checker is off.

Self-update needs a Docker socket McHarbor can write to; it is starting a container.

## Agents

An agent speaks a protocol version, and some features have floors: interactive terminal
exec needs agent v1.1.0 or newer.
[Settings → Updates → Agents](../../settings/updates/agents/README.md) pushes the matching
binary to every connected agent after McHarbor updates, and **Update agents now** runs the
sweep on demand.

## Container images

Upgrading McHarbor and upgrading the containers McHarbor manages are separate jobs. The
second is [Settings → Updates → Containers](../../settings/updates/containers/README.md).

## Rolling back

The data volume is the constraint. Migrations move it forward, and an older binary may not
understand a newer schema.

- **Same schema**: start the previous image tag. Nothing else is needed.
- **Schema changed**: restore the volume backup from step 1, then start the previous image.
  This is the only reliable path, and it is why the backup is step 1.

## Changing database backend

Moving between SQLite, PostgreSQL and MySQL is done from
[Settings → System → Database](../../settings/system/database/README.md) rather than by
editing the environment. While the copy runs, McHarbor holds itself still: mutating requests
get 503 and background work skips its ticks, while reading the interface keeps working. The
old database is never touched, so removing the stored configuration puts McHarbor straight
back on it.

## Factory reset

[Settings → System → About](../../settings/system/about/README.md) has one. It erases every
environment, user, setting, schedule and history entry McHarbor stores and returns the
installation to its first-run state. Containers, images and volumes on your hosts are
untouched. It asks you to type a confirmation phrase, and it cannot be undone.

## Related

- [Changelog](../changelog/README.md): what changed in each release, and the breaking changes
- [Installation](../installation/README.md): the Compose file and the volumes
- [Configuration](../configuration/README.md): the variables an upgrade might change
- [Settings → Updates](../../settings/updates/README.md): the three update audiences
- [Plugins → Architecture](../../development/plugins/architecture.md): why a plugin can fail to load after an upgrade
