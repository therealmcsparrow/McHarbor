# Storage

**Settings → Integrations → Storage** · `?tab=storage`

Reusable storage locations for backups and exports, and the backup encryption key.

## Permissions

| Key | What it unlocks |
|---|---|
| `settings.view` or `storage_locations.view` | See destinations |
| `settings.manage` or `storage_locations.manage` | Add or change one |
| `storage_locations.delete` | Remove one |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- Named destinations that backup plans and exports can write to.
- The backup encryption key: generating one, installing one you already hold, and rotating it.
- How much space each destination has left.

## Capacity

Each enabled destination shows used of total space under its name, with a bar that is
neutral up to 75%, amber past 75% and red past 90%, so a backup target that is about to
run out is visible on the page you already open to manage it, rather than in the failure of
the next run.

The number comes from the destination itself: a local path reads the filesystem hosting it,
and every other type asks the owning plugin for its provider's own quota. A destination
whose protocol has no concept of capacity (FTP is the one that says so outright) reports
that usage is not available and shows no bar. Nothing here fails a destination: a missing
number means unknown, not full.

## The backup encryption key

Encrypted backups need a key, and McHarbor is deliberately not the thing that holds it:
"Generate a 32-byte key for the Docker secret `mcharbor_backup_key`. **McHarbor only shows
it once and does not store it.**"

From there you either copy it and store it as the Docker secret yourself (the section gives
you the host setup command) or press **Install and restart**, which writes the secret and
restarts McHarbor for you.

### Rotation

Regenerating replaces the current key. Copy or install the current key first, because
McHarbor cannot show it again, and **keep the old key or existing encrypted backups may
become unreadable**.

Old archives are not re-encrypted. They stay readable with the key they were written with,
which is why restore asks for the original secret when it sees an archive encrypted under a
different one. Keep retired keys for as long as you keep the archives they protect.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Storage destination plugins](../../../plugins/storage/README.md): all 14, with their fields
- [Backups](../../../backup/screen/README.md): plans, retention and restore
