# Authentication

McHarbor accepts two credentials on the protected route group, and four more on specific
public routes. The protected chain tries the API key first and falls through to the session
cookie, so a request carrying both authenticates as the key.

## Session cookie

The browser credential. Obtained by logging in, carried automatically afterwards.

```bash
curl -c jar.txt -X POST http://localhost:8705/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"…"}'
```

| Property | Value |
|---|---|
| Cookie name | `mcharbor_session` |
| Path | `/` |
| `HttpOnly` | yes; JavaScript cannot read it |
| `Secure` | set when the request arrived over HTTPS, or when the configuration says to |
| `SameSite` | `Lax` |
| Absolute lifetime | **24 hours**, not admin-tunable |
| Idle timeout | `security.session_idle_timeout_seconds`, default **600 s**, range 60 to 86400, `0` disables |
| Session id | 32 random bytes from `crypto/rand`, base64url, 43 characters. The database stores only its SHA-256, so a copy of the database or its WAL holds no usable session; the id a sessions list returns is that hash, which revoke accepts |

The idle clock is driven by `POST /api/auth/session/touch`, which the browser calls at most
once a minute while the user is actually typing or clicking, not by raw request traffic. A
dashboard polling in a forgotten tab is the app being busy, not the user. A script using the
session cookie and never touching it will therefore be signed out on the idle window even
while it is making requests; use an API key instead.

`GET /api/auth/session/policy` returns the window to any signed-in caller, ungated on
purpose: everyone is subject to it, and hiding it would leave non-admins finding out from a
401 mid-task.

### Two-step login

When the account has MFA enrolled, `POST /api/auth/login` answers with a challenge instead
of a session. Complete it with `POST /api/auth/login/mfa`, passing `challengeId` and `code`.

The challenge is *peeked* before verification and burned after, so a mistyped digit costs a
retry rather than the whole password round. What keeps that affordable is the pair of limits
on it: the auth group's 10-requests-per-minute ceiling, and the challenge's five-minute TTL.

### Lockout

Repeated failures lock the account before the expensive Argon2 verification runs, which is
what makes brute force impractical rather than merely slow.

| Setting | Default |
|---|---|
| `lockout_max_attempts` | 5 |
| `lockout_window_seconds` | 900 |
| `lockout_initial_seconds` | 60 |
| `lockout_max_lock_seconds` | 3600 |

A failed password or second-factor code at `POST /api/auth/sudo` counts toward the same
lockout, and a locked account cannot open a step-up window. Locks back off exponentially from the initial value up to the maximum. An admin holding
`account.manage` clears one with `POST /api/users/{id}/unlock`, which is idempotent.

A second threshold counts failures per client address, across every username, and is
checked before the username is looked up:

| Setting | Default |
|---|---|
| `security.lockout_ip_max_attempts` | 30 (`0` disables) |
| `security.lockout_ip_window_seconds` | 900 |
| `security.lockout_ip_lock_seconds` | 900 |

A locked address answers `423` with `retryAfterSeconds`, exactly as a locked account does,
so the response never says which threshold tripped. The address comes from the
trusted-proxy resolver, not from a raw `X-Forwarded-For`: forwarding headers count only
from peers inside `MCHARBOR_TRUSTED_PROXY_CIDRS`. Behind a reverse proxy that variable
must name the proxy, or every client is attributed to it. A trusted proxy's own address and
an unattributable request are never counted. Each `login_attempts` row records how its
address was attributed in `ip_source` (`peer`, `forwarded`, `x-forwarded-for`,
`x-real-ip`). An address lock expires on its own; there is no unlock route for it.

The auth group's rate limit (10 requests a minute) is keyed on the same resolved address.

### Logging out

`POST /api/auth/logout` destroys the current session. `POST /api/auth/sessions/revoke-all`
ends every *other* session the caller owns, keeping the current one. Use logout to end the
current one.

## API key

The credential for scripts, CI and integrations. Not subject to the idle timeout.

```bash
curl -H 'Authorization: Bearer mch_…' http://localhost:8705/api/containers
```

| Property | Value |
|---|---|
| Header | `Authorization: Bearer <key>` |
| Prefix | `mch_`. A token without it is rejected before any database lookup |
| Storage | SHA-256 of the key; the plaintext exists only in the creation response |
| Expiry | optional `expiresAt`; no value means it never expires |
| Revocation | `DELETE /api/api-keys/{id}` deactivates an active key and permanently deletes an already-revoked one |

Create one with `POST /api/api-keys`. The response carries the plaintext **once**; it is
never readable again. `GET /api/api-keys/options` first tells you which roles, environments
and permissions you are allowed to put on a key.

A key's effective rights are the intersection of what it was granted and what its owner
still has. Demoting the owner immediately narrows every key they issued; that is the point,
and it is why a key cannot be used to preserve access past a role change.

Failure codes distinguish the causes: `err.api_key.invalid` (no such key, or missing
prefix), `err.api_key.revoked`, `err.api_key.expired`. Each attempt is recorded against the
key for the usage graphs, including the ones that failed.

## First-run setup

Before the first account exists, `GET /api/auth/status` reports `needsSetup: true` and
`POST /api/auth/setup` creates the initial administrator. After that the setup route returns
`err.auth.setup_completed` forever.

**The first-run routes need the setup token.** With no account, McHarbor mints a random token at
start and writes it once to its log (`docker logs mcharbor` for the container,
`journalctl -u mcharbor` for a native install; the line beginning `first-run setup token`).
The native installers read it back from the journal and print it when they finish. `POST /api/auth/setup`, `POST /api/database/setup/test` and
`POST /api/database/setup/apply` must carry it in the `X-McHarbor-Setup-Token` header. A missing
or wrong token is `403` with `data.reason: "setup_token"`; it is compared in constant time, it
changes on every restart, and no token exists once an account does. The three routes share one
lock and re-check that no account exists inside it, so concurrent requests cannot both succeed.
A SQLite `path` outside the data directory is `400` with `data.reason: "path_outside_data_dir"`,
and the connection test is written to the audit log as `database.setup.test`.

`GET /api/auth/password-policy` is deliberately unauthenticated and sits in the rate-limited
auth group: the setup wizard and the token-based reset page both run logged out and must be
able to state the rules before the user starts typing. It carries complexity requirements
only.

`GET /api/database/setup/status` tells the wizard whether to show the database step at all,
and `POST /api/database/setup/test` is the connection test without the permission, for the
installer who has no account yet.

## Password reset

`POST /api/auth/password/forgot` is public and constant-time from the caller's perspective:
it looks up the user, issues a token and best-effort emails it, swallowing every error into
one neutral response so the endpoint cannot be used to enumerate accounts.

`POST /api/auth/password/reset` consumes the token, which must be valid, unexpired and
unconsumed. The TTL is `security.password_reset_ttl_minutes`.

Administrators have two other paths, both under `account.password.reset`:
`POST /api/users/{id}/password/admin` sets a password outright, and
`POST /api/users/{id}/force-reset-password` stamps the account so the user must change it at
next login.

## Single sign-on

Identity providers are plugins. The host owns the flow; the plugin owns the protocol.

| Route | Purpose |
|---|---|
| `GET /api/identity-providers/enabled` | The login page's provider list. Public |
| `GET /api/identity-providers/{id}/authorize` | Starts the flow: a redirect for OIDC and SAML redirect binding, an auto-submitting form for SAML POST binding |
| `GET /api/identity-providers/callback` | The OIDC callback |
| `GET` `POST /api/identity-providers/{id}/acs` | The SAML assertion consumer service |
| `GET /api/identity-providers/{id}/metadata` | Service-provider metadata for a SAML provider |

All six are in the rate-limited auth group. An account provisioned by a provider is locked
against local edits to the fields the provider owns; attempts return
`err.profile.identity_provider_locked`.

## Multi-factor authentication

MFA is per-account and self-service; there is no permission key, because every route acts
only on the caller's own account.

| Route | Purpose |
|---|---|
| `GET /api/auth/mfa/status` | The caller's own MFA state |
| `POST /api/auth/mfa/enroll/begin` | Issues a secret and recovery codes. **The response is the only time either is readable** |
| `POST /api/auth/mfa/enroll/confirm` | Verifies the first code and turns MFA on |
| `DELETE /api/auth/mfa` | Turns MFA off and destroys the secret |

## SCIM bearer tokens

`/api/scim/v2/*` is mounted in the public group with its own bearer middleware, because an
external identity system has to reach it without a McHarbor session. Each SCIM endpoint row
carries its own token: supply one on create, or let the service generate a 32-byte token and
return the plaintext once in `bearerToken`.

## Agent tokens

A remote agent connects *outbound* to `GET /api/agent/ws` with `?token=`. The token is the
credential; the route sits in the auth group with no session requirement.

`GET /api/agent/install/{token}` serves the install script against a one-time token, for the
same reason: the operator running it has no session on the target host.

Agent tokens are shown once at creation and redacted in every later response.
`POST /api/agents/{envId}/regenerate-token` mints a new one, which invalidates the old.

## Internal transfer endpoints

Three public routes exist purely so a connected agent can stream a large archive without
going back through the WebSocket:

- `GET /api/containers/internal/move-transfers/{transferId}`
- `GET` `PUT /api/container-backups/internal/agent-archives/{transferId}`
- `GET /api/container-backups/internal/transfers/{transferId}`

They authenticate on the `Authorization` header against the one-shot transfer id, not on a
session. They are not a general-purpose surface and are documented here so an operator
reading an access log recognises them.

## Elevation

A user can request a permission they do not hold, for a bounded time, with a justification:
`POST /api/elevations`. Requests appear in an admin inbox (`GET /api/elevations/pending`,
`users.manage`) and are settled with `POST /api/elevations/{id}/decide`. A requester can
list their own queue and withdraw a pending request without any catalog key, because both act
only on rows they own; an API key needs `account.self.manage` to create or withdraw one (see
[Self-service routes](./permissions.md#self-service-routes)). At most **five** requests can be
pending per user, and asking again for a key and environment already pending is refused; both
answer `429` `err.rate_limit_exceeded` with `data.reason` `too_many_pending` or
`already_pending`. `GET /api/elevations/{id}` answers only the requester and holders of
`users.manage` at global scope; anyone else gets `404`.

Only permissions marked elevatable can be requested; anything else returns
`err.permission_not_elevatable`.

## Auth disabled

When authentication is switched off, the middleware injects a synthetic `system` user on
every request. It has no database role assignments, so `GET /api/auth/me/permissions`
short-circuits and returns the wildcard `*`, which is what the server-side permission checks
also do for it, keeping the UI gates and the route gates in agreement.

## Related

- [Permissions](./permissions.md): what happens after authentication
- [Auth endpoints](../endpoints/auth.md): the full route table
- [API key endpoints](../endpoints/api-keys.md)
- [Identity endpoints](../endpoints/identity.md)
