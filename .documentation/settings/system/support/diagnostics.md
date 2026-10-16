# Diagnostics

**Enterprise → Diagnostics** · `/enterprise/diagnostics` · `support.view`

Health checks for the node serving the page. Each reports **Pass**, **Warning**, **Fail** or
**Skipped** (the check does not apply here), says what to do about it, and lists the facts it
found. **Run all checks** repeats the whole set; the ▶ button on a row re-runs just that check.
**Copy JSON** copies the full result, already redacted, for a support ticket.

All checks run concurrently, each bounded by its own timeout.

## The checks

| Check | Pass | Warning | Fail |
|---|---|---|---|
| **Database** | The database answers a ping; driver and server version shown | — | No answer |
| **Database migrations** | Every migration this build ships is applied | Some are pending, or the database has migrations this build does not know (it was used by a newer McHarbor) | The ledger cannot be read |
| **Data directory** | Exists, a probe file can be written and removed, 2 GB or more free | Less than 2 GB free | Missing, or not writable |
| **Docker reachability** | Every active non-agent environment answers a ping within 5 s | Some do not | None do. Skipped when there are no such environments |
| **Remote agents** | Every active agent environment has an agent connected to this node | Some are disconnected | Skipped when there are no agent environments |
| **Plugins** | Every enabled plugin loaded | — | At least one failed to load; its error is listed. Skipped when the plugin runtime is not active |
| **Identity providers** | Always passes; lists enabled providers, or says only local accounts can sign in | — | The list cannot be read |
| **Outbound network** | A `HEAD` to the catalog URL (`APPSTORE_CATALOG_URL`, else the first enabled catalog) answers within 5 s | No answer, or a 5xx | Skipped when no catalog URL is configured |
| **TLS certificate** | Valid for more than 30 days | Expires within 30 days, or no certificate file | Expired or unparseable. Skipped when `TLS_ENABLED=false` |
| **Backup encryption key** | A backup key is loaded | None is loaded; mount one at `BACKUP_ENCRYPTION_KEY_FILE` | — |
| **Data encryption key** | The key comes from `ENCRYPTION_KEY` | The key was generated into `DATA_DIR/.encryption_key` (see [the encryption-key warning](./README.md#the-encryption-key-warning)) | — |
| **Readiness** | The node would pass `/api/readyz` | — | It would not; the false condition is listed |

Only the host of the catalog URL is shown, never the full URL. Every detail value passes through
the same scrubber as the [support bundle](./support-bundle.md).

## Routes

| Method | Route | Permission |
|---|---|---|
| `GET` | `/api/support/diagnostics`: every check | `support.view` |
| `GET` | `/api/support/diagnostics?check=tls,docker`: only those | `support.view` |
| `GET` | `/api/support/encryption-key`: `{ "source": "env" \| "file", "fileSeen": bool }` | `support.view` |

An unknown check id answers `400` with `data.reason = "unknown_check"`.
