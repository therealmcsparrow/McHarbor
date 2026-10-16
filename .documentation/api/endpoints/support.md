# Support and diagnostics

10 endpoints: live diagnostics, the state inventory, the pre-upgrade check, and support
bundles: redacted archives an operator can hand to someone else.

| Key | Covers |
|---|---|
| `support.view` | Diagnostics, the encryption-key source, the state inventory, bundle list, preview and detail |
| `support.manage` | Generate, download and delete a bundle |
| `updates.view` | The pre-upgrade check |

## Things worth knowing first

**Bundles are redacted and previewable.** `GET /api/support/bundles/preview` returns exactly what
a bundle with those categories would contain, so the operator can read it before it exists.
Categories are `system`, `logs`, `plugins`, `cluster`, `database`, `environments`, `audit` and
`diagnostics`. Only one bundle generates at a time; the ten newest finished ones are kept.

**Diagnostics never hang on one dependency.** Each check runs concurrently under its own timeout.
`?check=` names a subset: `database`, `migrations`, `data_dir`, `docker`, `agents`, `plugins`,
`identity`, `outbound`, `tls`, `backup_key`, `encryption_key`, `readiness`.

**Nothing here returns a key.** `GET /api/support/encryption-key` says only where the
data-encryption key came from.

**The pre-upgrade check changes nothing** and never blocks the update path; it reports what an
admin should know before starting one.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/support/bundles` | `support.view` | Lists bundles (running first, then finished newest first) with how many are kept | — |
| `POST` | `/api/support/bundles` | `support.manage` | Starts generating a bundle; `409` `bundle_running` while another one is | — |
| `GET` | `/api/support/bundles/preview` | `support.view` | Returns exactly what a bundle with those categories would contain, redacted, each file capped for display | `?categories=` |
| `DELETE` | `/api/support/bundles/{id}` | `support.manage` | Deletes a finished bundle; `409` while it is still generating | `{id}` |
| `GET` | `/api/support/bundles/{id}` | `support.view` | Returns one bundle and its progress | `{id}` |
| `GET` | `/api/support/bundles/{id}/download` | `support.manage` | Streams the bundle as `mcharbor-support-<id>.zip`; `409` `not_ready` until it has finished | `{id}` |
| `GET` | `/api/support/diagnostics` | `support.view` | Runs the named checks, or all of them, concurrently, each under its own timeout | `?check=` |
| `GET` | `/api/support/encryption-key` | `support.view` | Says where the data-encryption key came from (`env` or `file`) and whether a key file was seen; never the key | — |
| `GET` | `/api/support/inventory` | `support.view` | Lists every category of state under the data directory with its size, and whether a backup covers it | — |
| `GET` | `/api/support/pre-upgrade` | `updates.view` | Measures the running instance against a target version; changes nothing | `?target=` |

## Request bodies

**`POST` `/api/support/bundles`**: `categories`

## Related

- [Support bundle](../../settings/system/support/support-bundle.md)
- [Diagnostics](../../settings/system/support/diagnostics.md)
- [State inventory](../../settings/system/support/state-inventory.md)
- [Pre-upgrade check](../../settings/system/support/pre-upgrade-check.md)
