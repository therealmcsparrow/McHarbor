# Settings

27 endpoints: the settings API (a generic key-value pair of routes, and a typed endpoint
for each family with validation worth enforcing), plus the public branding read.

| Key | Covers |
|---|---|
| `settings.view` | Reading settings and every typed group |
| `settings.manage` | Writing them, TLS, the backup key, resetting user overrides |
| — | `GET /api/settings/toast` and `GET /api/settings/appearance` (signed in); `GET /api/branding` (public) |

Both keys are checked **globally**: a `settings.manage` grant scoped to one environment does
not satisfy them.

## Things worth knowing first

**Prefer the typed endpoints.** `/agent`, `/scanners`, `/retention`, `/toast`, `/appearance`,
`/timezone`, `/tls` and `/branding` validate on write. A value written through the untyped
`PUT /api/settings` or `PUT /api/settings/{key}` that the reader does not accept is ignored at
read time and the default used instead.

**Certificates and keys are neither returned nor writable here.** `tls_cert`, `tls_key` and
`webpush_vapid_private` are redacted from the list and get-by-key reads, and `PUT /api/settings/{key}`
and the bulk `PUT /api/settings` refuse them with `400 err.protected_target` (use `PUT /api/settings/tls`
and `POST /api/push/vapid/regenerate`). The audit diff of a settings change stores `[redacted]` for them; `GET /api/settings/tls` reports status without the certificate, and
`GET /api/settings/tls/ca` serves only the public root.

**Changing the timezone restarts McHarbor**, because the process reads `TZ` at startup.

The concepts, and every key with its type, default and bounds, are in the
[Settings API](../settings/README.md) and the [settings reference](../settings/reference.md).

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/branding` | — | Returns the operator-supplied application marks. Public: the login page needs them before anyone has a session | — |
| `GET` | `/api/settings` | `settings.view` | Returns all settings rows; `tls_cert` and `tls_key` are redacted | `?category=` |
| `PUT` | `/api/settings` | `settings.manage` | Updates several settings at once, untyped | — |
| `GET` | `/api/settings/agent` | `settings.view` | Returns the current agent settings | — |
| `PUT` | `/api/settings/agent` | `settings.manage` | Validates and upserts agent settings | — |
| `GET` | `/api/settings/appearance` | — | Returns the app-wide appearance defaults. Signed in; no key, because every user paints their own theme from it | — |
| `PUT` | `/api/settings/appearance` | `settings.manage` | Validates and upserts the app-wide appearance defaults | — |
| `POST` | `/api/settings/appearance/reset-overrides` | `settings.manage` | Clears every account's appearance overrides so the whole install follows the app-wide default again | — |
| `POST` | `/api/settings/backup-key/generate` | `settings.manage` | Generates a one-time backup encryption key for Docker secret storage | — |
| `POST` | `/api/settings/backup-key/install` | `settings.manage` | Installs a one-time backup encryption key as a host-side Docker secret file | — |
| `GET` | `/api/settings/backup-key/status` | `settings.view` | Reports whether the runtime backup encryption key is active | — |
| `PUT` | `/api/settings/branding` | `settings.manage` | Validates and upserts the application marks | — |
| `GET` | `/api/settings/retention` | `settings.view` | Returns the current retention settings | — |
| `PUT` | `/api/settings/retention` | `settings.manage` | Validates and upserts retention settings | — |
| `GET` | `/api/settings/scanners` | `settings.view` | Returns the current scanner settings | — |
| `PUT` | `/api/settings/scanners` | `settings.manage` | Validates and upserts scanner settings | — |
| `GET` | `/api/settings/timezone` | `settings.view` | Returns the configured McHarbor process timezone | — |
| `PUT` | `/api/settings/timezone` | `settings.manage` | Persists `TZ` and schedules a McHarbor container recreate | — |
| `GET` | `/api/settings/tls` | `settings.view` | Returns the TLS configuration status, never the certificate or key | — |
| `PUT` | `/api/settings/tls` | `settings.manage` | Uploads a certificate and key, toggles TLS and the HTTPS redirect | — |
| `GET` | `/api/settings/tls/ca` | `settings.view` | Downloads the local root certificate as PEM (`mcharbor-root-ca.pem`); the signing key is never served | — |
| `POST` | `/api/settings/tls/regenerate` | `settings.manage` | Re-mints the self-signed server certificate, keeping the root, and reloads the listeners in place; refuses an operator-uploaded certificate | — |
| `GET` | `/api/settings/toast` | — | Returns the app-wide toast defaults. Signed in; no key | — |
| `PUT` | `/api/settings/toast` | `settings.manage` | Validates and upserts the app-wide toast defaults | — |
| `POST` | `/api/settings/toast/reset-overrides` | `settings.manage` | Clears every account's toast overrides so the whole install follows the app-wide default again | — |
| `GET` | `/api/settings/{key}` | `settings.view` | Returns a single setting by key | `{key}` |
| `PUT` | `/api/settings/{key}` | `settings.manage` | Sets a single setting by key (upsert), untyped | `{key}` |

## Request bodies

**`PUT` `/api/settings`**: `settings`

**`PUT` `/api/settings/agent`**: `eventMode`, `eventPollInterval`, `pingInterval`, `metricsEnabled`, `requestTimeout`, `statsPushInterval`

**`PUT` `/api/settings/appearance`**: `themeId`, `accent`, `font`, `baseFontPx`, `allowUserOverride`

**`POST` `/api/settings/backup-key/install`**: `key`, `projectPath`

**`PUT` `/api/settings/branding`**: `logoWide`, `logoWideDark`, `logoIcon`, `logoIconDark`, `logoWidePng`, `logoIconPng`

**`PUT` `/api/settings/retention`**: `auditRetentionDays`, `activityRetentionDays`, `lifecycleRetentionDays`, `backupLogRetentionDays`, `scanRetentionDays`, `backupRetentionCount`, `backupRetentionDays`, `activeAuditRetentionPreset`, `metricsRawHours`, `metricsRollupDays`, `metricsDiskEnabled`?, `recordExecEvents`?, `apiKeyUsageRetentionDays`?

**`PUT` `/api/settings/scanners`**: `scanTimeout`, `scanOnInstall`, `scanOnUpdate`

**`PUT` `/api/settings/timezone`**: `timezone`

**`PUT` `/api/settings/tls`**: `cert`?, `key`?, `enabled`?, `forceHttps`?

**`PUT` `/api/settings/toast`**: `position`, `durationMs`, `maxVisible`, `richColors`, `closeButton`, `allowUserOverride`

**`PUT` `/api/settings/{key}`**: `key`, `value`, `category`

## Related

- [Settings API](../settings/README.md): generic versus typed, per-user overrides, TLS, the backup key
- [Settings reference](../settings/reference.md): every key
- [Settings screens](../../settings/README.md)
