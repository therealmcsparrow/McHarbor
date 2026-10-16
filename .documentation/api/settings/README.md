# Settings API

24 endpoints. Settings are key-value rows in one table, grouped by a category column, and
exposed two ways: a generic key-value pair of routes, and a typed group endpoint for each
family that has validation worth enforcing.

Every setting key, with its type, default, bounds and effect, is in
[the settings reference](./reference.md).

| Key | Covers |
|---|---|
| `settings.view` (global) | Reading settings and every typed group |
| `settings.manage` (global) | Writing them |
| — | `GET /api/settings/toast` and `GET /api/settings/appearance` |

Note the scope: these use `RequireGlobalPermission`, so a `settings.manage` grant scoped to
one environment does not satisfy them. Settings are install-wide, and so is the permission
that changes them.

## Generic versus typed

`GET /api/settings` returns every row, optionally filtered to one `?category=`.
`GET /api/settings/{key}` and `PUT /api/settings/{key}` read and upsert one row.
`PUT /api/settings` writes several in one call.

These are untyped: the value is a string, and nothing validates it beyond the row existing.
A value written this way that falls outside what the reader accepts is **ignored at read
time**, and the default is used instead. The system does not break, but the setting silently
does not take effect. That is why every family with real constraints also has a typed
endpoint.

The typed endpoints (`/agent`, `/scanners`, `/retention`, `/toast`, `/appearance`,
`/timezone`, `/tls`) validate on write, so a value that survives a write is always a value
the reader will accept back. **Prefer them.** Use the generic routes for the keys that have
no typed group: the security family, `auth.default_user_role_id`, the audit keys,
`disk_usage_default_threshold_percent`.

## Two ungated reads

`GET /api/settings/toast` and `GET /api/settings/appearance` are authenticated but carry no
permission, on purpose: every signed-in user needs the app-wide defaults to paint their own
toasts and theme, and neither response holds anything an operator would withhold. Writing
both is `settings.manage` like every other app-wide default.

## Per-user overrides

Toast and appearance are the two families a user may overrule for themselves, through
`PUT /api/auth/preferences`. Whether they may is itself a setting:
`toast_allow_user_override` and `appearance_allow_user_override`.

Turning an override off does not discard what people already chose: the per-user columns stay
on the account and take effect again the moment it is turned back on. To actually clear them,
`POST /api/settings/toast/reset-overrides` and
`POST /api/settings/appearance/reset-overrides` wipe every account's overrides so the whole
install follows the app-wide default again.

## Secrets

`tls_cert`, `tls_key` and `webpush_vapid_private` are never returned by the list or get-by-key
endpoints; they are on an explicit redaction list. The same three are also refused by the generic
write routes: `PUT /api/settings/{key}` and the bulk `PUT /api/settings` answer `400`
`err.protected_target`, because a raw write would store them unencrypted. Use `PUT /api/settings/tls`
for the certificate and key, and `POST /api/push/vapid/regenerate` to rotate the push key. When a
setting's change is audited, the diff stores `[redacted]` in place of any of these values.
`GET /api/settings/tls` returns status about the certificate without returning the certificate.

## Changing the timezone restarts McHarbor

`PUT /api/settings/timezone` persists the zone and schedules a container recreate with `TZ`
set, because the process reads it at startup. The response reports both the configured and
the running zone, and whether a restart is still pending.

## The backup encryption key

Three routes, and the order matters: `POST /api/settings/backup-key/generate` mints a
one-time key, `POST /api/settings/backup-key/install` writes it as a host-side Docker secret
file, and `GET /api/settings/backup-key/status` reports whether the runtime key is active.
Without an active key, backups fail with `err.container.backup_key_missing` and existing
archives cannot be restored.

## TLS

`PUT /api/settings/tls` uploads a certificate and key, toggles TLS, and sets whether HTTP is
redirected. `POST /api/settings/tls/regenerate` re-mints the server certificate McHarbor's own
root issued and reloads the listeners in place. It refuses when the running certificate was
uploaded by the operator, rather than silently discarding it, and it **keeps the root**, so
clients that already trust it stay trusting it.

`GET /api/settings/tls/ca` returns the local root certificate as `application/x-pem-file`,
attachment-named `mcharbor-root-ca.pem`. Only the public half is ever read; the signing key
beside it is never served. It answers `404` with `err.settings.tls_no_ca` on an install that
has only ever served an operator upload. What clients do with the file is
[Installing the root CA](../../screens/security/install-root-ca.md).

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/settings` | `settings.view` | Returns all settings | `?category=` |
| `PUT` | `/api/settings` | `settings.manage` | Updates multiple settings at once | — |
| `GET` | `/api/settings/agent` | `settings.view` | Returns the current agent settings | — |
| `PUT` | `/api/settings/agent` | `settings.manage` | Validates and upserts agent settings | — |
| `GET` | `/api/settings/appearance` | — | Returns the app-wide appearance defaults | — |
| `PUT` | `/api/settings/appearance` | `settings.manage` | Validates and upserts the app-wide appearance defaults | — |
| `POST` | `/api/settings/appearance/reset-overrides` | `settings.manage` | Clears every account's appearance overrides so the whole install follows the app-wide default again | — |
| `POST` | `/api/settings/backup-key/generate` | `settings.manage` | Generates a one-time backup encryption key for Docker secret storage | — |
| `POST` | `/api/settings/backup-key/install` | `settings.manage` | Installs a one-time backup encryption key as a host-side Docker secret file | — |
| `GET` | `/api/settings/backup-key/status` | `settings.view` | Reports whether the runtime backup encryption key is active | — |
| `GET` | `/api/settings/retention` | `settings.view` | Returns the current retention settings | — |
| `PUT` | `/api/settings/retention` | `settings.manage` | Validates and upserts retention settings | — |
| `GET` | `/api/settings/scanners` | `settings.view` | Returns the current scanner settings | — |
| `PUT` | `/api/settings/scanners` | `settings.manage` | Validates and upserts scanner settings | — |
| `GET` | `/api/settings/timezone` | `settings.view` | Returns the configured McHarbor process timezone | — |
| `PUT` | `/api/settings/timezone` | `settings.manage` | Persists TZ and schedules a McHarbor container recreate | — |
| `GET` | `/api/settings/tls` | `settings.view` | Returns the current TLS configuration status | — |
| `PUT` | `/api/settings/tls` | `settings.manage` | Updates TLS certificate, key, and toggle settings | — |
| `POST` | `/api/settings/tls/regenerate` | `settings.manage` | Re-mints the locally-issued server cert, keeping the root, and reloads the listeners in place | — |
| `GET` | `/api/settings/tls/ca` | `settings.view` | Downloads the local root certificate as PEM (`mcharbor-root-ca.pem`) | — |
| `GET` | `/api/settings/toast` | — | Returns the app-wide toast defaults | — |
| `PUT` | `/api/settings/toast` | `settings.manage` | Validates and upserts the app-wide toast defaults | — |
| `POST` | `/api/settings/toast/reset-overrides` | `settings.manage` | Clears every account's toast overrides so the whole install follows the app-wide default again | — |
| `GET` | `/api/settings/{key}` | `settings.view` | Returns a single setting by key | `{key}` |
| `PUT` | `/api/settings/{key}` | `settings.manage` | Sets a single setting by key (upsert) | `{key}` |


## Request bodies

**`PUT` `/api/settings`**: `settings`

**`PUT` `/api/settings/agent`**: `eventMode`, `eventPollInterval`, `pingInterval`, `metricsEnabled`, `requestTimeout`

**`PUT` `/api/settings/appearance`**: `themeId`, `accent`, `font`, `baseFontPx`, `allowUserOverride`

**`POST` `/api/settings/backup-key/install`**: `key`, `projectPath`

**`PUT` `/api/settings/retention`**: `auditRetentionDays`, `activityRetentionDays`, `lifecycleRetentionDays`, `backupLogRetentionDays`, `scanRetentionDays`, `backupRetentionCount`, `backupRetentionDays`, `activeAuditRetentionPreset`, `metricsRawHours`, `metricsRollupDays`, `metricsDiskEnabled`?, `recordExecEvents`?, `apiKeyUsageRetentionDays`?

**`PUT` `/api/settings/scanners`**: `scanTimeout`, `scanOnInstall`, `scanOnUpdate`

**`PUT` `/api/settings/timezone`**: `timezone`

**`PUT` `/api/settings/tls`**: `cert`?, `key`?, `enabled`?, `forceHttps`?

**`PUT` `/api/settings/toast`**: `position`, `durationMs`, `maxVisible`, `richColors`, `closeButton`, `allowUserOverride`

**`PUT` `/api/settings/{key}`**: `key`, `value`, `category`

## Related

- [Settings reference](./reference.md): every key, its type, default and bounds
- [Settings screens](../../settings/README.md): the same settings from the interface
- [Conventions](../conventions/README.md)
