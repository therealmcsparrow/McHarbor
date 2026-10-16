# Backups

`/backups`

Encrypted container backups: what has run, what is scheduled, and how to get data back.

## Permissions

| Key | What it unlocks |
|---|---|
| `backups.view` | See runs and plans |
| `backups.manage` | Create plans and run backups |
| `backups.download` | Download an archive |
| `backups.delete` | Delete runs and plans |
| `restore.view` / `restore.run` | See and perform a restore |
| `backup_log.view` | Read the backup log |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- Four sections: **Overview** (coverage), **Activity** (runs and logs), **Plans**, and **Settings**.
- The run list, with status, destinations, size, duration and per-row restore, download and delete.
- The plan list, with schedule, destinations, retention and next run.

## Consistency: the setting that matters most

Every backup picks one of three modes, and this is the choice that decides whether the
archive is usable:

| Mode | What it does | Cost |
|---|---|---|
| **Pause** | Stops the container while the archive is written, then starts it again | Correct everywhere, at the cost of downtime |
| **Snapshot** | Freezes the filesystem and reads the frozen copy, so the container keeps serving traffic | Needs a snapshot configuration on that environment |
| **Live** | Reads the running container’s files as they are | Fastest, and **the only mode that can produce a torn archive** |

If a snapshot cannot be taken when the backup actually runs, **the container is paused
instead** and the reason is recorded on the run, so a failed snapshot degrades to a
correct backup rather than to a broken one.

Live is the right choice for a container whose data is not a database. It is the wrong
choice for one that is.

## Download is its own permission

An archive is the container’s data, off the host, in one file. That is why
`backups.download` is separate from `backups.view`.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Backup](../README.md): the section index: consistency, destinations, encryption, retention and restore
- [Containers](../../screens/containers/README.md): per-container backup settings, schedules and runs
- [Settings → Integrations → Storage](../../settings/integrations/storage/README.md): destinations and the backup encryption key
- [Settings → Integrations → Snapshots](../../settings/integrations/snapshots/README.md): what makes Snapshot mode available
- [Storage destination plugins](../../plugins/storage/README.md): where archives can go
