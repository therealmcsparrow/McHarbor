# Support bundle

**Enterprise → Support bundle** · `/enterprise/support-bundle`

A `.zip` an administrator generates on request from McHarbor support. It is written to
`DATA_DIR/support-bundles/` and leaves the server only when someone downloads it.

## What can go in

Pick any of these under **Include in the bundle**. Each becomes one JSON file, next to a
`manifest.json` naming the version, build, time and chosen categories.

| Category | File | Contents |
|---|---|---|
| Version and configuration | `system.json` | Version, build, Go version, platform, hostname, CPU count, database driver, where the encryption key came from, and every environment variable McHarbor reads |
| Recent server logs | `logs.json` | The last 500 log records this process kept in memory (level, message, attributes). Older lines are only in `docker logs` |
| Plugins | `plugins.json` | Enabled plugins and what the loader reported for each, including load errors |
| Cluster | `cluster.json` | Readiness conditions, this node's role and state, and the node registry |
| Database | `database.json` | Driver, server version, applied migrations, pending ones and any this build does not know |
| Environments | `environments.json` | Id, name, connection type, active flag, agent status and version, Docker version |
| Permission denials | `audit.json` | Counts of `permission.denied` and `abac.denied` audit entries in the last 24 hours and 7 days |
| Diagnostics | `diagnostics.json` | The result of every [diagnostics](./diagnostics.md) check |

## What never goes in

Keys, tokens, passwords, kubeconfigs, agent tokens, API keys, Docker socket paths and connection
addresses. Three layers enforce it:

1. **Nothing secret is read.** Every query names its columns; environments are read without
   `agent_token`, `tls_*`, `ssh_*`, `socket_path` or host columns.
2. **Environment variables are allow-listed.** A value is printed only for variables known to be
   safe (`PORT`, `LOG_LEVEL`, `MCHARBOR_DB_DRIVER`, `TLS_ENABLED` …). Anything else McHarbor
   reads, and any other `MCHARBOR_*` variable, is listed as set or unset with the value
   `[redacted]`. A variable whose name contains *key*, *secret*, *token*, *password*, *dsn*,
   *credential* or *private* is redacted even if it is added to the allow-list by mistake.
3. **Every file is scrubbed** before it is written: the literal values of `ENCRYPTION_KEY`,
   `MCHARBOR_SECRET`, `MCHARBOR_DB_DSN` (and its password), the backup key, the
   `.encryption_key` and `.secret` files and every sensitive-named environment variable (also
   in base64 and URL-encoded form), plus URL user-info, MySQL DSN passwords, PEM blocks,
   `Bearer` / `Basic` / `token` values, `password=` / `api_key=` / `token=` style pairs and
   stored ciphertext (`enc:v1:…`). Each is replaced with `[redacted]`.

## Redaction preview

**Preview redaction** shows every file exactly as it will be written, with the total size as the
size estimate. The preview and the bundle come from the same code path, so what was reviewed is
what gets zipped. Files over 64 KB are cut in the preview only.

## Generating

**Generate bundle** starts generation in the background; the history row shows **Generating**
with a progress bar and updates every second. One bundle generates at a time; the button is
disabled while one runs. Generation stops after five minutes.

## History

The newest **10** bundles are kept; older ones are deleted when a new one finishes. Each row has
**Download** and **Delete** (with a confirmation). A failed row shows **Failed** and an error
icon; the details are in the server log.

## Routes

| Method | Route | Permission |
|---|---|---|
| `GET` | `/api/support/bundles` | `support.view` |
| `GET` | `/api/support/bundles/preview?categories=system,logs,…` | `support.view` |
| `POST` | `/api/support/bundles`: body `{ "categories": [ … ] }` | `support.manage` |
| `GET` | `/api/support/bundles/{id}` | `support.view` |
| `GET` | `/api/support/bundles/{id}/download` | `support.manage` |
| `DELETE` | `/api/support/bundles/{id}` | `support.manage` |

Generating, downloading and deleting are written to the audit log as
`support.bundle.generate`, `support.bundle.download` and `support.bundle.delete`.

## What goes wrong

| Symptom | Cause |
|---|---|
| `400` with `data.reason = "invalid_categories"` | No category, or one that does not exist |
| `409` with `data.reason = "bundle_running"` | Another bundle is still generating, or a delete targeted a running one |
| `409` with `data.reason = "not_ready"` on download | The bundle is still generating or failed |
| `logs.json` says `"retained": false` | The process was started without the in-memory log ring; collect `docker logs` instead |
