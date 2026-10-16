# Restore

**Settings → System → Instance backup → Restore**, or the restore icon on a completed export.

## The three modes

| Mode (as labelled) | Use it for | What it replaces |
|---|---|---|
| **Everything (disaster recovery)** | A clean install after losing the host | Every exported table, the encryption key, the certificate authority, stack files and custom nodes. Everyone is signed out |
| **Everything, onto new hardware or another database** | Moving an instance, including to another backend (SQLite ↔ PostgreSQL ↔ MySQL/MariaDB) | The same as above |
| **Only some categories** | Recovering, say, deleted workflows or a damaged role set on a running instance | Only the chosen categories; keys and files stay |

Categories for a selective restore: **Users, roles and policies**, **Environments and
registries**, **Workflows, schedules and alerts**, **Stacks**, **Secrets vault**, **Settings
and integrations**, **Plugin settings and data**, **Everything else**. For each, choose
**Replace the whole category with the export's** or **Keep them and add only what is missing**.

A selective restore from another instance converts encrypted values from the export's key to
this instance's key as they are written. On PostgreSQL without superuser rights a selective
*replace* is refused, because deleting the category's rows would cascade into categories you did
not choose; use *keep* there.

## Check export: the plan

Nothing is written until you have read the plan. **Check export** reads the whole file (which
is the only way to prove it complete) and shows, per category, the rows on this instance now
and the rows in the export, whether the encryption key, the audit secret and the files will be
replaced, how many plugins will be reinstalled, and any warning. Type `RESTORE` to enable
**Restore now**.

A restore is refused, with the reason in words, when:

| Reason | Meaning |
|---|---|
| The export comes from a newer McHarbor | Its schema has migrations this build does not know. Upgrade first |
| `ENCRYPTION_KEY` is set in the environment | The export's key cannot take its place. Unset it or set it to the export's key |
| `MCHARBOR_SECRET` is set in the environment | Same, for the audit secret, when audit history is restored |
| Nothing would be restored | Choose a category |
| The key is wrong | "That passphrase or key file does not open this export" |
| The file is not an export | "This file is not a McHarbor instance export." |
| The export is from a newer format | "...made by a newer McHarbor and cannot be read here. Upgrade first, then restore." |
| The file is cut short | "The export is incomplete: it ends before everything it lists. Copy it again from the source." |
| The file was altered or damaged | "The export is damaged or was altered after it was made." |
| The wrong kind of secret | "This export was sealed with the other kind of secret. Use its passphrase or key file instead." |
| The key file is not a key file | "That key file is not a readable McHarbor instance key file." |

The reason is always one of these fixed messages. The technical detail of what was wrong with the
file is written to the server log and is never sent to the browser.

## What a restore does, in order

1. The whole file is verified (first pass).
2. The **intent** (who, how, which export, and the last hash of the current audit chain) is
   appended to `DATA_DIR/restore-ledger.jsonl`, written into the live audit log, and pushed to
   every configured log forwarder (syslog / SIEM) **before anything is changed**.
3. The database is written in **one transaction**. Tables owned by the plugin installer are not
   copied; sessions are cleared in a full restore; work that was running when the export was taken
   comes back failed rather than forever running.
4. After the commit, the key material, certificate authority and files are put in place under a
   journal (`DATA_DIR/restore-journal.json`).
5. McHarbor restarts, because the key and secret are read once at boot.
6. On that start, each plugin is reinstalled at its recorded version through the same path the
   Plugin Store uses, keeping its role grants; a version no longer in the catalog fails that one
   plugin by name and the rest continue. If any plugin was installed or updated, McHarbor
   restarts once more to load it.

The status card shows **Restoring**, **Restarting**, **Reinstalling plugins**, **Restored** or
**Failed**, with each plugin's outcome. A failure's reason is behind the error icon.

**An interrupted restore leaves the instance in its prior state or finishes it.** If the process
dies before the commit, the transaction never happened. If it dies after, the next start finds
the journal, sees the commit, and finishes swapping keys and files before anything reads them.

## Audit history and the restore anchor

**Also restore audit history** replaces the audit log with the export's. The chain then
continues *and* records the seam: a row with action `instance.restore.anchor` is written as a
link in the chain, carrying the last hash of the chain that was replaced, the export manifest's
hash and the last hash of the restored segment. The verifier reports it as **verified across N
restore anchors**, listing each, never as a plain pass (`mcharbor audit verify`).

Restoring audit history needs `instance_backup.restore_audit` through an **approved elevation
request that is active at the time of the restore**. A standing role grant is not enough; ask for
the elevation under Security, have someone approve it, then restore.

Configuring an external log forwarder is what makes the audit log's guarantee survive a restore:
the intent and the replaced chain's last hash reach it before the table is replaced.

## Restoring an instance whose interface will not start

```bash
docker run --rm -it -v <mcharbor-data-volume>:/app/data <mcharbor-image> \
  mcharbor restore --local /app/data/instance.mchx --passphrase-stdin
```

`mcharbor restore --local` runs the server binary's own restore (`/app/mcharbor restore-local`)
against the data directory. It is authorised by access to that directory and the export's key,
and recorded as `local:<os-user>@<host>`, never as a McHarbor user. Flags: `--key-file`,
`--passphrase-file`, `--passphrase-stdin` (or `MCHARBOR_RESTORE_PASSPHRASE`), `--mode`,
`--categories`, `--include-audit`, `--conflict`, `--plan` (check and print, then stop) and
`--yes` (skip typing `RESTORE`).

It **refuses while McHarbor answers on this host** and prints the instance id; `--while-running
<instance-id>` overrides that, deliberately. On the local path the intent goes to the ledger and
the audit log only; no forwarder runs without the server. Plugins are reinstalled on the next
start.

## Routes

| Method | Route | Key |
|---|---|---|
| `PUT` | `/api/instance-backups/uploads` (raw file body) | `instance_backup.restore` |
| `POST` | `/api/instance-backups/restore/plan` | `instance_backup.restore` |
| `POST` | `/api/instance-backups/restore` | `instance_backup.restore`; `includeAudit` also needs an active approved elevation for `instance_backup.restore_audit` |
| `GET` | `/api/instance-backups/restore/status` | `instance_backup.view` |
| `GET` | `/api/instance-backups/restore/ledger` | `instance_backup.view` |

A refused plan answers `err.invalid_body` with a `reason` (`wrong_key`, `key_kind`,
`bad_key_file`, `not_export`, `format`, `incomplete`, `damaged`, `unreadable`, `no_categories`, `bad_category`, `bad_conflict`, `bad_mode`, `busy`); a restore
without the approval answers `403` with `reason: approval_required`.
