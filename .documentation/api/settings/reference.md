# Settings reference

Every setting key McHarbor reads, with its category, type, default, accepted range and what
it actually changes.

**A value outside the accepted range is ignored at read time, not rejected at write time**,
unless it went through the typed endpoint, which validates. A hand-edited row holding
nonsense therefore leaves the default in force rather than breaking anything, which also
means a setting that "does not work" is usually one written out of range. The typed
endpoints exist so that a value which survives a write is always a value the reader accepts.

`0` means "keep forever" throughout the retention family, with two documented exceptions.

## Agent

Category `agent`. Typed endpoint: `GET`/`PUT /api/settings/agent`.

| Key | JSON field | Type | Default | Range | Effect |
|---|---|---|---|---|---|
| `agent_event_mode` | `eventMode` | enum | `poll` | `poll`, `stream` | How the agent reports Docker events: polling on an interval, or a held-open stream |
| `agent_event_poll_interval` | `eventPollInterval` | seconds | `30` | 10 to 300 | Poll interval, when the mode is `poll` |
| `agent_ping_interval` | `pingInterval` | seconds | `30` | 10 to 120 | Keepalive interval on the agent WebSocket |
| `agent_metrics_enabled` | `metricsEnabled` | bool | `true` | — | Whether agents report resource metrics at all |
| `agent_request_timeout` | `requestTimeout` | seconds | `30` | 5 to 120 | How long the host waits for an agent to answer one request |

## Scanners

Category `scanners`. Typed endpoint: `GET`/`PUT /api/settings/scanners`.

| Key | JSON field | Type | Default | Range | Effect |
|---|---|---|---|---|---|
| `scanner_timeout` | `scanTimeout` | seconds | `300` | 30 to 1800 | How long the host holds a scan open before giving up |
| `scanner_scan_on_install` | `scanOnInstall` | bool | `false` | — | Scan an image automatically when an app is installed |
| `scanner_scan_on_update` | `scanOnUpdate` | bool | `false` | — | Scan automatically when an image is updated |

Five keys in this category are **no longer read**: `scanner_trivy_enabled`,
`scanner_grype_enabled`, `scanner_clair_enabled`, `scanner_clair_url` and `scanner_default`.
`079_scanners.sql` copied them into per-instance scanner configuration rows and deliberately left
the originals in place so a rollback finds them intact. Editing them today does nothing.

## Retention

Category `retention`. Typed endpoint: `GET`/`PUT /api/settings/retention`.

| Key | JSON field | Type | Default | Range | Effect |
|---|---|---|---|---|---|
| `retention_audit_days` | `auditRetentionDays` | days | `90` | 0 to 3650 | Audit entries older than this are deleted by the purge. `0` keeps forever |
| `retention_activity_days` | `activityRetentionDays` | days | `30` | 0 to 3650 | Container event feed |
| `retention_lifecycle_days` | `lifecycleRetentionDays` | days | `30` | 0 to 3650 | Lifecycle events |
| `retention_backup_log_days` | `backupLogRetentionDays` | days | `30` | 0 to 3650 | Backup log entries |
| `retention_scan_days` | `scanRetentionDays` | days | `90` | 0 to 3650 | Scans and their vulnerability rows |
| `retention_api_key_usage_days` | `apiKeyUsageRetentionDays` | days | `90` | 0 to 3650 | Per-key request telemetry, and the tombstones deleted keys leave behind |
| `retention_backup_count` | `backupRetentionCount` | count | `0` | 0 to 1000 | Keep at most N backup runs per plan. `0` keeps all |
| `retention_backup_days` | `backupRetentionDays` | days | `0` | 0 to 3650 | Delete backup runs older than this. `0` keeps all |
| `retention_metrics_raw_hours` | `metricsRawHours` | hours | `24` | **1** to 8760 | How long 30-second samples survive before being folded into 10-minute averages. Does **not** accept 0 |
| `retention_metrics_rollup_days` | `metricsRollupDays` | days | `30` | **1** to 3650 | How long those averages survive. Does **not** accept 0 |
| `retention_metrics_disk_enabled` | `metricsDiskEnabled` | bool | `true` | — | Whether disk metrics are collected alongside CPU and memory |
| `retention_record_exec_events` | `recordExecEvents` | bool | `false` | — | Whether Docker exec events are written to the event log at all |
| `retention_audit_preset` | `activeAuditRetentionPreset` | string | `custom` | any non-empty | Which named retention preset the UI shows as active |

Two of these deserve explanation.

**The metrics pair is tiered, which is why neither accepts "forever".** An unbounded
30-second series grows without limit. Once a 10-minute bucket lies entirely past the raw
window, the collector averages it and deletes the raw rows; the rollup horizon then governs
the averages. The raw window has a floor of one hour because the compactor can only fold
whole buckets.

**`retention_record_exec_events` is off for a measured reason.** On one production install,
`exec_create` / `exec_start` / `exec_die` accounted for 95.4% of all stored events
(1,384,647 of 1,451,867 rows, about 5.2 GB), and they are overwhelmingly healthcheck probes.
Turning it on is supported and genuinely high-volume; the interface recommends an external
database alongside it. Nothing is lost while it is off: healthcheck results are recorded
properly, with exit codes, in their own table.

## Toasts

Category `toast`. Typed endpoint: `GET`/`PUT /api/settings/toast`. These are the app-wide
defaults; a user may override them through `PUT /api/auth/preferences` if the last key allows
it.

| Key | JSON field | Type | Default | Range | Effect |
|---|---|---|---|---|---|
| `toast_position` | `position` | enum | `bottom-right` | `top-left`, `top-center`, `top-right`, `bottom-left`, `bottom-center`, `bottom-right` | Where toasts appear |
| `toast_duration_ms` | `durationMs` | ms | `4000` | 0 to 60000 | How long a toast stays. `0` means never auto-dismiss, which is why the close button is forced on for that case |
| `toast_max_visible` | `maxVisible` | count | `3` | 1 to 10 | How many stack before older ones collapse |
| `toast_rich_colors` | `richColors` | bool | `true` | — | Colour-code by severity |
| `toast_close_button` | `closeButton` | bool | `true` | — | Show an explicit close button |
| `toast_allow_user_override` | `allowUserOverride` | bool | `true` | — | Whether a signed-in user may replace these with their own |

An invalid position is rejected on write **and** ignored on read, so a hand-edited row can
never put the toaster somewhere the frontend cannot place it.

## Appearance

Category `appearance`. Typed endpoint: `GET`/`PUT /api/settings/appearance`.

| Key | JSON field | Type | Default | Range | Effect |
|---|---|---|---|---|---|
| `appearance_theme_id` | `themeId` | string | `mcharbor-default` | ≤64 chars of `a–z A–Z 0–9 - _ : .` | Which theme a fresh account renders. Plugin themes use `plugin:<slug>` |
| `appearance_accent` | `accent` | enum | `theme` | `theme`, `blue`, `green`, `purple`, `orange`, `rose`, `cyan` | Replaces the theme's primary colour. `theme` leaves it alone |
| `appearance_font` | `font` | enum | `inter-tight` | see below | The interface font |
| `appearance_base_font_px` | `baseFontPx` | px | `16` | 12 to 20 | The root font size every relative size derives from |
| `appearance_allow_user_override` | `allowUserOverride` | bool | `true` | — | Whether a user may replace these with their own |

The theme id is **shape-checked, not membership-checked**, because plugins register themes at
runtime. A theme that has been uninstalled falls back to the default client-side rather than
failing.

Fonts, by how they are loaded:

| Loading | Ids |
|---|---|
| Self-hosted, work offline | `inter-tight`, `bricolage`, `jetbrains-mono` |
| OS-native stacks, no download | `system`, `serif`, `humanist` |
| Fetched by the browser from a CDN | `inter`, `roboto`, `open-sans`, `lato`, `montserrat`, `poppins`, `nunito-sans`, `source-sans-3`, `work-sans`, `manrope`, `plus-jakarta-sans`, `dm-sans`, `ibm-plex-sans`, `atkinson-hyperlegible`, `source-serif-4`, `merriweather`, `lora`, `roboto-mono`, `ibm-plex-mono`, `fira-code` |

The backend only validates the id; it never reaches the network itself, and every CDN face
falls back to a bundled one client-side. McHarbor is frequently deployed air-gapped, so the
first two rows are the safe choices there.

The base font size bounds are not arbitrary: 12px is the smallest at which the tabular
figures in the metrics tables stay legible, and at 20px the sidebar stops fitting its labels
on one line.

## Audit

Category `audit`. No typed endpoint; use `PUT /api/settings/{key}`.

| Key | Type | Default | Range | Effect |
|---|---|---|---|---|
| `audit.clock.drift_threshold_seconds` | seconds | `5` | 1 to 60 | How far the clock may drift from the reference before the audit page warns |
| `audit.clock.reference_url` | URL | `https://www.cloudflare.com/` | any `http`/`https` URL | The time reference the six-hourly background clock-drift check compares against |
| `audit.hash_chain.enabled` | bool | `true` | — | Chain each audit entry to the previous one, so tampering is detectable by `GET /api/audit/verify` |
| `audit.action_catalog.enabled` | bool | `true` | — | Maintain the catalog of auditable actions that drives the coverage report |
| `audit.worm` | enum | `off` | `off`, on-values | Write-once-read-many mode: audit entries cannot be deleted |
| `audit.worm_delete_allowed` | bool as `0`/`1` | `0` | — | The escape hatch that permits deletion while WORM is on |
| `audit.<type>_forwarding.enabled` | bool | per row | — | Per-forwarder kill switch. The key is generic, so a new log-forwarder plugin gets one for free |

Forwarding itself is no longer a core concern; it is the `logging-` plugin family. The two
rows seeded for the original SIEM and syslog transports still work and are still honoured.

## Security

Category `security`. No typed endpoint; use `PUT /api/settings/{key}`. The password policy
is readable unauthenticated at `GET /api/auth/password-policy`, because the setup wizard and
the reset page both need to state the rules while logged out.

| Key | Type | Default | Effect |
|---|---|---|---|
| `password_min_length` | count | `12` | Minimum length, regardless of complexity |
| `password_require_upper` | bool | `true` | Require an uppercase letter |
| `password_require_lower` | bool | `true` | Require a lowercase letter |
| `password_require_digit` | bool | `true` | Require a digit |
| `password_require_symbol` | bool | `true` | Require a symbol |
| `password_max_age_days` | days | `0` | Force a change after this many days. `0` disables |
| `password_history_size` | count | `5` | Refuse reuse of the last N passwords. `0` disables |
| `lockout_max_attempts` | count | `5` | Failures before the account locks |
| `lockout_window_seconds` | seconds | `900` | The window those failures are counted in |
| `lockout_initial_seconds` | seconds | `60` | The first lock duration |
| `lockout_max_lock_seconds` | seconds | `3600` | The ceiling the exponential backoff stops at |
| `security.lockout_ip_max_attempts` | count | `30` | Failed logins from one client address, across every username, before that address locks. `0` disables the per-address lock |
| `security.lockout_ip_window_seconds` | seconds | `900` | The window those per-address failures are counted in |
| `security.lockout_ip_lock_seconds` | seconds | `900` | How long a locked address stays locked. Fixed, no backoff |
| `security.session_idle_timeout_seconds` | seconds | `600` | Automatic sign-out after inactivity. 60 to 86400; `0` disables. An out-of-range value is **clamped**, not ignored, so a bad row cannot lock everyone out |
| `security.password_reset_ttl_minutes` | minutes | `60` | How long a reset link is valid. Accepted between 5 minutes and 7 days |
| `security.password_reset_email_channel_id` | id | — | Which communication channel sends reset mail |
| `security.password_reset_email_from` | string | — | The sender address |
| `security.password_reset_email_subject` | string | built-in | Subject line |
| `security.password_reset_email_body` | string | built-in | Body template |
| `rbac_v2_enabled` | bool | `true` | Role-based access control |
| `rbac_v3_enabled` | bool | `true` | Attribute-based policies on top of it |
| `auth.default_user_role_id` | role id | — | The role a newly provisioned account receives. Without it, account creation that relies on a default fails with `err.user.create_no_default` |

The lockout check runs **before** password verification, not after: a locked account never
reaches the Argon2 comparison, which is what makes brute force impractical rather than merely
slow.

The per-address lock runs before the username is even looked up, so one password tried
against many usernames from one source is caught too. The address is the one the
trusted-proxy resolver attributes the request to: behind a reverse proxy,
`MCHARBOR_TRUSTED_PROXY_CIDRS` must name the proxy, or every client counts as the proxy's
address. An address inside those CIDRs is never counted or locked, and neither is a request
with no attributable address. A locked address gets the same `423` response as a locked
account.

## General

Category `general`.

| Key | Type | Default | Effect |
|---|---|---|---|
| `mcharbor_timezone` | IANA zone | the container's `TZ`, else `UTC` | The process timezone. Written through `PUT /api/settings/timezone`, which also schedules the container recreate that makes it take effect |
| `default_language` | locale | `en` | The language the login page and unauthenticated responses use. One of `en`, `nl`, `de`, `es`, `fr`, `pt` |

## Disk usage

Category `disk_usage`.

| Key | Type | Default | Effect |
|---|---|---|---|
| `disk_usage_default_threshold_percent` | percent | `80` | The Docker disk-usage notification threshold used by every environment whose own setting says "use the global default" |

Each environment can override this on its own row:
`dockerDiskUsageThresholdPercent`, with `dockerDiskUsageUseGlobalDefault` deciding which
wins. See [Environments](../endpoints/environments.md).

## Self-update

Category `self_update`. Typed endpoint: `GET`/`PUT /api/updates/settings`.

| Key | JSON field | Type | Default | Effect |
|---|---|---|---|---|
| `self_update_enabled` | `enabled` | bool | `true` | Run the periodic release checker at all |
| `self_update_interval_hours` | `intervalHours` | hours | `24` | How often to check. Clamped to 1 to 168; `0` resets to the default |
| `self_update_channel_ids` | `channelIds` | list | `[]` | Which communication channels are told about a new release |
| `self_update_auto_install` | `autoInstall` | bool | `false` | Install a new release automatically rather than only announcing it |
| `self_update_last_seen_version` | `lastSeenVersion` | version | empty | Set by `POST /api/updates/dismiss` so the banner stops highlighting a version |
| `self_update_auto_update_agents` | `autoUpdateAgents` | bool | `false` | Sweep connected agents onto the new version after McHarbor updates |
| `self_update_agent_excluded_env_ids` | `agentExcludedEnvIds` | list | `[]` | Environments the agent sweep skips |
| `self_update_state` | — | JSON | — | Written by the checker, read back by `GET /api/updates/state`. Not edited by hand |

## Licence

Category `license`. Written by the product rather than by an operator; listed so a reader
recognises them.

| Key | Effect |
|---|---|
| `license.server_url` | Where the licence heartbeat is sent |
| `license.contact_url` | The link the licence banner offers |
| `license.heartbeat_interval_hours` | How often the heartbeat runs |
| `license.sample_interval_minutes` | How often usage is sampled for the entitlement count |

## TLS

Category `tls`. Typed endpoint: `GET`/`PUT /api/settings/tls`, plus
`POST /api/settings/tls/regenerate`.

| Key | JSON field | Type | Effect |
|---|---|---|---|
| `tls_enabled` | `enabled` | bool | Serve HTTPS |
| `tls_force_https` | `forceHttps` | bool | Redirect plain HTTP to HTTPS |
| `tls_cert` | `cert` | PEM | The certificate. **Never returned**, and the generic write routes refuse it with `400 err.protected_target`; write it through `PUT /api/settings/tls` |
| `tls_key` | `key` | PEM | The private key. **Never returned**, and refused by the generic write routes the same way |
| `tls_mode` | — | string | Whether the running certificate was issued by the local root or supplied by the operator. Regeneration refuses on an operator-supplied one |

Certificate and key must be written as a pair; sending one alone returns
`err.settings.tls_pair_required`. The minimum TLS version is an environment variable, not a
setting, and is reported in the TLS status payload.

The status payload also carries the local root, read from disk rather than from `settings`:

| JSON field | Type | Effect |
|---|---|---|
| `hasCa` | bool | Whether `$DATA_DIR/tls/ca.pem` exists at all |
| `caFingerprint` | string | The root's SHA-256, lowercase colon-separated, the value to compare before installing it |
| `caNotAfter` | RFC 3339 | The root's expiry. Ten years from first boot |

The root itself is downloaded from `GET /api/settings/tls/ca`, not from any settings key.

## Not settings

Three families look like settings and are not:

- **Environment variables**: the listen address, data directory, encryption key, allowed
  origins, HSTS max-age, TLS floor, plugin catalog directory. They are read at startup and
  are documented in [Getting started](../../getting-started/configuration/README.md).
- **Per-environment options**: update checks, pruning, event tracking, metric collection,
  the auto-update window, per-environment retention. They live on the environment row; see
  [Environments](../endpoints/environments.md).
- **Per-user preferences**: language, time and date format, and the toast and appearance
  overrides. They live on the account; see `PUT /api/auth/preferences` in
  [Authentication endpoints](../endpoints/auth.md).

## Related

- [Settings API](./README.md): the endpoints that read and write these keys
- [Settings screens](../../settings/README.md): the same settings from the interface
- [Getting started → Configuration](../../getting-started/configuration/README.md): environment variables
