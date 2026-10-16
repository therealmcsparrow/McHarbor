# Schedules and verification

## Verify: the restore rehearsal

A backup nobody has restored is a hypothesis. **Verify** (the shield icon on a completed
export) restores it into a throwaway SQLite database and data directory (never this instance),
then checks every table's rows against the manifest and walks the restored audit chain with the
export's own secret. The **Verified** column shows **Passed** with the date, **Verifying**, or
**Failed** with the reason behind the error icon. It needs the export's passphrase or key file.

A verification interrupted by a restart is marked failed on the next start.

## Schedules

**Add schedule**: a **Name**, **When** (the readable schedule builder, with an advanced cron
field; cron is interpreted in the server's time zone), the key (**Lock exports with** a
passphrase or a key file), **Also copy to** any number of storage locations (nothing selected
reads **Keep on this host only**),
**Keep this many** and **Keep for days** (at least one of the two), **Rehearse a restore after
each export**, **Include metrics history** and **Enabled**.

An unattended export has nobody to ask for a key, so a schedule's key **is stored on this
instance**, encrypted with the instance key, and never returned by the API. Keep the same key
elsewhere too, or the exports cannot be restored after losing the host. Editing a schedule keeps
its key unless a new one is given.

Each run: export, copy to every storage location selected (streamed through the same bridge
storage plugins use; the file is already encrypted, so it is not wrapped again), rehearse if
asked, then apply retention here **and** at the location. Every destination is attempted even
after one fails, and the run is only recorded as failed when **no** destination accepted the
export; reaching one of two still leaves an offsite copy. Saving a schedule replaces its whole
destination set. The list shows the schedule in words next to its cron,
the next run, and the last run's outcome: a failure at any step, with its reason behind the
error icon. **Run now** runs it immediately. On a multi-node install only the leader runs
schedules.

## Dashboard

The **Instance backup** widget shows when this instance was last exported and last proven
restorable, flags an instance whose last completed export is older than 48 hours, and lists each
schedule's last run with failing ones first. It needs `instance_backup.view`.

## Alerts

Two conditions under **Platform**:

| Condition | Fires |
|---|---|
| **Instance export overdue** | A schedule has not completed an export within the rule's period (counted from its creation if it never has) |
| **Instance export failed** | A schedule's last run failed: the export, the copy, or the rehearsal. Resolves on the next good run |

## Routes

| Method | Route | Key |
|---|---|---|
| `POST` | `/api/instance-backups/{id}/verify` | `instance_backup.manage` |
| `GET` | `/api/instance-backups/schedules` | `instance_backup.view` |
| `POST` | `/api/instance-backups/schedules` | `instance_backup.manage` |
| `PUT` | `/api/instance-backups/schedules/{id}` | `instance_backup.manage` |
| `DELETE` | `/api/instance-backups/schedules/{id}` | `instance_backup.manage` |
| `POST` | `/api/instance-backups/schedules/{id}/run` | `instance_backup.manage` |

A refused schedule answers `err.invalid_body` with a `reason`: `bad_name`, `bad_cron`,
`bad_retention`, `no_retention`, or a key reason.
