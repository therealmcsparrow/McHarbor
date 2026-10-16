# Authentication endpoints

35 endpoints: logging in, the first-run wizard, the caller's own profile, preferences,
sessions and second factor, the step-up window, the caller's saved table layouts and
filters, and the device sign-in the CLI uses.

Almost nothing here carries a permission key, and that is correct: every route either runs
before a principal exists, or acts only on the caller's own account with the target taken
from the session rather than from the request body.

Read [Authentication](../conventions/authentication.md) first: it covers the credentials,
the lockout policy, the idle timeout and the MFA challenge flow. This page is the route
table.

## Route groups

| Group | Routes |
|---|---|
| **auth**: rate-limited to 10/min per client address (resolved through `MCHARBOR_TRUSTED_PROXY_CIDRS`), no session | `login`, `login/mfa`, `logout`, `setup`, `status`, `password-policy`, `password/forgot`, `password/reset` |
| **protected**: session or API key | `session`, `session/policy`, `session/touch`, `me/permissions`, `profile`, `preferences`, `sessions/*`, `mfa/*`, `sudo`; and `/api/table-views/*` |

## Things worth knowing first

**`POST /api/auth/sudo` opens a five-minute step-up window** bound to the caller's session, for
one named scope. It proves nothing the login did not already prove and grants nothing alone:
routes that need it (revealing a vault secret, downloading an instance backup) check the
window on top of their own permission.

**Table views belong to the caller.** `/api/table-views/*` stores each user's column layout and
saved filters per table; the user always comes from the session, never from the path, which is
why those routes carry no key.

**`GET /api/auth/status` is the bootstrap call.** It reports `needsSetup`, `authDisabled`,
the enabled SSO providers and the default language: everything the login page needs before
anyone has authenticated.

**`PUT /api/auth/preferences` is partial.** Send any subset; absent fields are unchanged.
Values are normalised rather than rejected, so an unknown time format silently falls back to
`24h`. The toast and appearance fields are this user's override of the app-wide defaults in
[Settings](../settings/reference.md), and `clearToastOverride` /
`clearAppearanceOverride` drop back to them.

**`GET /api/auth/me/permissions`** returns the caller's effective permission set: the union
of direct roles, group-inherited roles, per-user overrides, ABAC policies and active
elevations. The frontend gates affordances on it; the server still checks every request.

**Session touch is not audited and takes no permission.** The session id comes from the
cookie, never from the body, so there is no other session it could touch.

**`PUT /api/users/{id}/password` is here in spirit but lives on the users route.** It takes
the current password, and the handler decides whether the caller may change that particular
account's password rather than a route-level permission doing it.
## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `POST` | `/api/auth/login` | — | Authenticates a user and sets a session cookie | — |
| `POST` | `/api/auth/login/mfa` | — | Completes a two-step login: it verifies the second factor against a live challenge and only then issues the session | — |
| `POST` | `/api/auth/logout` | — | Destroys the session cookie | — |
| `GET` | `/api/auth/me/permissions` | — | Returns the effective permissions granted to the current authenticated user across every environment | — |
| `DELETE` | `/api/auth/mfa` | — | Turns MFA off for the caller and destroys the secret | — |
| `POST` | `/api/auth/mfa/enroll/begin` | — | Issues a new secret and recovery codes | — |
| `POST` | `/api/auth/mfa/enroll/confirm` | — | Verifies the first code and turns MFA on | — |
| `GET` | `/api/auth/mfa/status` | — | Returns the caller's own MFA state | — |
| `POST` | `/api/auth/password/forgot` | — | Is public (no auth) and constant-time from the caller's perspective | — |
| `POST` | `/api/auth/password/reset` | — | Is public and consumes a token | — |
| `GET` | `/api/auth/password-policy` | — | Returns the effective password policy | — |
| `GET` | `/api/auth/permissions/catalog` | `roles.view` or `permissions.view` | Returns the canonical permission metadata | — |
| `PUT` | `/api/auth/preferences` | — | Updates per-user preferences for the current authenticated user | — |
| `PUT` | `/api/auth/profile` | — | Updates editable account details for the current authenticated user | — |
| `GET` | `/api/auth/session` | — | Returns the current authenticated user from context | — |
| `GET` | `/api/auth/session/policy` | — | Returns the session policy for the caller | — |
| `POST` | `/api/auth/session/touch` | — | Resets the idle-timeout clock on the caller's own session | — |
| `GET` | `/api/auth/sessions` | — | Is a convenience endpoint for the current user to list their own sessions | — |
| `POST` | `/api/auth/sessions/revoke-all` | — | Revokes all the user's other sessions (the current one is preserved) | — |
| `DELETE` | `/api/auth/sessions/{sessionId}` | — | Is a convenience endpoint for the current user to revoke one of their own other sessions (use /auth/logout to end the current one) | `{sessionId}` |
| `POST` | `/api/auth/setup` | setup token | Creates the initial admin user during first-run setup; needs the `X-McHarbor-Setup-Token` header | — |
| `GET` | `/api/auth/status` | — | Returns public auth status (needsSetup, authDisabled, oidcProviders, defaultLanguage) | — |
| `POST` | `/api/auth/sudo` | — | Re-verifies the caller and opens a five-minute step-up window for one scope on their own session. Exempt: grants nothing on its own | — |
| `POST` | `/api/table-views/import` | — | Copies the caller's browser-stored table state to the server once. Exempt: scoped to the caller | — |
| `GET` | `/api/table-views/{tableId}` | — | Returns the caller's layout and saved filters for one table | `{tableId}` |
| `POST` | `/api/table-views/{tableId}/filters` | — | Saves a named filter for the caller | `{tableId}` |
| `DELETE` | `/api/table-views/{tableId}/filters/{id}` | — | Removes one of the caller's filters | `{tableId}`, `{id}` |
| `PUT` | `/api/table-views/{tableId}/filters/{id}` | — | Renames, re-captures or (un)defaults one of the caller's filters | `{tableId}`, `{id}` |
| `DELETE` | `/api/table-views/{tableId}/layout` | — | Resets the caller's layout to the page defaults | `{tableId}` |
| `PUT` | `/api/table-views/{tableId}/layout` | — | Stores the caller's layout; not audited, because every column resize writes one | `{tableId}` |


## Device sign-in

The flow behind `mcharbor login --device`. The two terminal-facing routes are public and
share the auth group's rate limit (10 requests a minute per address), which is why the
server asks for a 10-second poll interval. Approval creates an API key, so it takes
`api_keys.manage`, and the chosen roles and scopes are checked against the approver's own
exactly as `POST /api/api-keys` checks them.

| Method | Path | Permission | What it does |
|---|---|---|---|
| `POST` | `/api/device/code` | — | Opens a request. Body `{host, osUser}` (shown to the approver, control characters stripped). Returns `{deviceCode, userCode, verificationPath, expiresIn: 600, interval: 10}`; only a hash of `deviceCode` is stored |
| `POST` | `/api/device/token` | — | One poll, body `{deviceCode}`. `200` returns the new key once, then the code is spent. Otherwise `data.error` is `authorization_pending` (400), `slow_down` (429), `expired_token` (400), `access_denied` (403) or `invalid_grant` (400) |
| `GET` | `/api/device/requests/{userCode}` | `api_keys.manage` | A pending, unexpired request: `userCode`, `host`, `osUser`, `expiresAt`, `createdAt`. The code is accepted in any case, with or without its dash |
| `POST` | `/api/device/requests/{userCode}/approve` | `api_keys.manage` | Body as `POST /api/api-keys`: `name`, `roleIds`, `scopes`, `environmentIds`, `expiresAt`. The key is minted on the terminal's next poll. Audited `approve` on `device_sign_in` |
| `POST` | `/api/device/requests/{userCode}/deny` | `api_keys.manage` | Refuses it; the terminal stops with `access_denied`. Audited |

The error strings are the OAuth 2.0 device-flow ones (RFC 8628), carried in `data.error`
beside an existing McHarbor `code`.

## Request bodies

**`POST` `/api/auth/login`**: `username`, `password`

**`POST` `/api/auth/login/mfa`**: `challengeId`, `code`

**`POST` `/api/auth/mfa/enroll/confirm`**: `code`

**`POST` `/api/auth/password/forgot`**: `identifier`, `email`, `username`

**`POST` `/api/auth/password/reset`**: `token`, `newPassword`

**`PUT` `/api/auth/preferences`**: `preferredLanguage`?, `timeFormat`?, `dateFormat`?, `clearToastOverride`?, `toastPosition`?, `toastDurationMs`?, `toastMaxVisible`?, `toastRichColors`?, `toastCloseButton`?, `clearAppearanceOverride`?, `appearanceThemeId`?, `appearanceAccent`?, `appearanceFont`?, `appearanceBaseFontPx`?

**`PUT` `/api/auth/profile`**: `displayName`, `email`

**`POST` `/api/auth/setup`**: `username`, `password`, `email`?. Needs the `X-McHarbor-Setup-Token` header (`403` `setup_token` without it); see [First-run setup](../conventions/authentication.md#first-run-setup).

**`POST` `/api/auth/sudo`**: `scope`, `password`, `totpCode`. `scope` is one of `secrets.reveal`, `instance_backup.download`, `sessions.terminate`, `environments.delete`, `break_glass`; anything else is `400`. The window binds to the session cookie, so an API key cannot open one, and a session that is not the caller's own is refused. A failed verification is `403`, audited as `sudo_failed`, and counts toward the account lockout; a locked account is refused. Attempts are limited to **5 per minute** per user and client address (`429` beyond that).

**`POST` `/api/table-views/import`**: `tables`

**`POST` `/api/table-views/{tableId}/filters`**: `name`, `state`, `isDefault`

**`PUT` `/api/table-views/{tableId}/filters/{id}`**: `name`?, `state`, `isDefault`?

**`PUT` `/api/table-views/{tableId}/layout`**: `v`, `columns`, `density`, `pageSize`, `autoRefreshMs`. Each column is `id`, `visible`, `size`, `headerAlign`?, `cellAlign`?.

## Related

- [Authentication](../conventions/authentication.md): the credentials in full
- [Users](./users.md): administering other accounts
- [API keys](./api-keys.md): the non-browser credential
