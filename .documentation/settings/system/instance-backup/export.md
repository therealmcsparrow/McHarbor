# Export

**Settings → System → Instance backup → Create export**

## The key

An export is encrypted as a whole. Choose, per export, how it is locked:

| Lock with | What you keep |
|---|---|
| **A passphrase** | At least 12 characters. It is stretched with Argon2id (time 3, 64 MiB, 4 threads); the parameters and salt travel in the file, so later releases can raise them without breaking old exports |
| **A generated key file** | A file starting `mcharbor-instance-key-v1:`, shown once by **Generate a key file** → **Save the key file**. The export cannot be created until "I have saved the key file somewhere other than this host" is ticked |

The key is **never stored** by a manual export. It is also separate from the container-backup
key: an instance export's blast radius is every secret in the system, and rotating one key must
not rotate the other.

A key kept only on the host being backed up is not a key. Store it elsewhere.

## Creating one

Fields: **Lock with**, the passphrase (twice) or the key file, **Include metrics history**, and
a **Note** (up to 500 characters) that shows in the history.

**Also copy to** sends the finished export to any number of storage locations; selecting none
keeps it on this host, which is the default. The copies follow the completed local export and
never replace it, so a refused upload is recorded against the backup rather than losing it. The
history's **Copies** column names each destination with its own outcome, a refusal behind the
error icon.

The export runs as a job; the history row shows **Running** with row progress, then
**Completed** or **Failed**. A failed export shows its reason behind the error icon next to the
status. One export runs at a time; starting a second answers "An export is already running".

The database is read inside one transaction, so the row counts recorded in the file and the rows
that follow describe the same moment. An export interrupted by a restart is marked failed on the
next start.

## The file

`mcharbor-<yyyymmdd-hhmmss>.mchx`, kept under `DATA_DIR/instance-backups/`.

| Part | Readable without the key |
|---|---|
| First line `MCHARBOR-INSTANCE-EXPORT/1` | yes |
| Manifest: version, build, backend, creation time and user, instance id, host, schema, row counts per table, plugin list, file list, key kind, key id, note | yes |
| Manifest signature | yes, checked with the key before anything is decrypted |
| Contents: AES-256-GCM in 1 MiB chunks over a gzip stream of tables, key material and files | no |

The contents end with a SHA-256 over everything before them, so a truncated, reordered or
altered file is refused in full before a restore writes anything.

## Downloading

**Download** asks for your password or authenticator code first (a five-minute step-up window
scoped to `instance_backup.download`), then saves the file. The download is audited. API keys
cannot open a step-up window, so exports are downloaded from the web interface.

## Deleting

**Delete** removes the file from this host after a confirmation, **and removes every copy it
made at a storage location**; deleting the row first would leave those objects behind with
nothing left that knows their keys. Copies you downloaded yourself are not affected.

## Routes

| Method | Route | Key |
|---|---|---|
| `GET` | `/api/instance-backups` | `instance_backup.view` |
| `GET` | `/api/instance-backups/{id}` | `instance_backup.view` |
| `POST` | `/api/instance-backups` | `instance_backup.manage` |
| `POST` | `/api/instance-backups/key-file` | `instance_backup.manage` |
| `GET` | `/api/instance-backups/{id}/download` | `instance_backup.manage` + step-up |
| `DELETE` | `/api/instance-backups/{id}` | `instance_backup.manage` |

A refused request answers `err.invalid_body` with a `reason`: `key_required`, `key_both`,
`passphrase_too_short`, `key_file_invalid`, `note_too_long` or `busy`.
