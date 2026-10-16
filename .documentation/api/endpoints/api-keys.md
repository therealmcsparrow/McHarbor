# API keys

10 endpoints: minting keys, seeing what they can do, and reading what they have done.

Permissions are enforced in `routes.go` through two composed middlewares rather than one key
per route:

| Gate | Routes |
|---|---|
| `api_keys.view` or `api_keys.manage` | List, detail, usage series, usage events |
| `api_keys.manage` | Create, update, revoke, grant options |
| `settings.manage` | Clear all usage history |

A holder of only `manage` still sees their own keys; the handler narrows the list rather
than the route refusing it.

A key acts on its owner's own account only where it carries `account.self.manage`: profile,
second-factor enrolment, session revocation, elevation requests, approval execution and
cancellation, and licence notice dismissal all refuse a key without it (see
[Self-service routes](../conventions/permissions.md#self-service-routes)). A key cannot open a
step-up window or activate break-glass at all.

## Things worth knowing first

**The plaintext key exists once.** `POST /api/api-keys` returns it in the creation response
and it is never readable again; only a SHA-256 hash is stored.

**A key cannot outrank its owner.** Its effective rights are the intersection of what it was
granted and what the owner still holds, so demoting the owner immediately narrows every key
they issued.

**`GET /api/api-keys/options` first.** It returns the roles, environments and permissions the
caller is actually allowed to put on a key, which is a smaller set than the full catalog.

**`GET /api/api-keys/{id}` is the full access picture**: assigned roles and what each grants,
explicit grants, the configured union, and what the key can do once the owner's rights are
applied. That last column is the one that matters.

**Revoke and delete share a route.** `DELETE /api/api-keys/{id}` deactivates an active key
and permanently deletes one that is already revoked. Same path, same ownership rule, distinct
audit actions.

**Clearing usage history is a retention decision, not a key action.** That is why
`DELETE /api/api-keys/usage` is gated by `settings.manage` and deliberately leaves live and
revoked credentials alone: an operator clearing history is saying "forget what happened", not
"break my automation".

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/api-keys` | `api_keys.view` or `api_keys.manage` | Returns API keys | — |
| `POST` | `/api/api-keys` | `api_keys.manage` | Creates a new API key for the caller | — |
| `GET` | `/api/api-keys/options` | `api_keys.manage` | Returns the roles, environments, and permissions the caller may put on a key | — |
| `DELETE` | `/api/api-keys/usage` | `settings.manage` | Destroys all usage history and every deleted-key tombstone | — |
| `DELETE` | `/api/api-keys/{id}` | `api_keys.manage` | Deactivates an active API key the caller may manage, and permanently deletes one that is already revoked | `{id}` |
| `GET` | `/api/api-keys/{id}` | `api_keys.view` or `api_keys.manage` | Returns one key's full access picture: assigned roles and what each grants, explicit grants, the configured union, and what the key can actually do once the owner's rights are applied | `{id}` |
| `PATCH` | `/api/api-keys/{id}` | `api_keys.manage` | Changes name and grants of a key the caller may manage | `{id}` |
| `POST` | `/api/api-keys/{id}/rotate` | `api_keys.manage` | Replaces an active key with a new secret carrying the same grants and revokes the old one; the new secret is returned once | `{id}` |
| `GET` | `/api/api-keys/{id}/usage` | `api_keys.view` or `api_keys.manage` | Returns the request-count graph plus its summary | `{id}` |
| `GET` | `/api/api-keys/{id}/usage/events` | `api_keys.view` or `api_keys.manage` | Returns one page of the request history | `{id}` |


## Request bodies

**`POST` `/api/api-keys`**: `name`, `roleIds`, `scopes`, `environmentIds`, `expiresAt`?

**`PATCH` `/api/api-keys/{id}`**: `name`?, `roleIds`, `scopes`, `environmentIds`

## Related

- [Authentication](../conventions/authentication.md): using a key
- [Permissions](../conventions/permissions.md): what a key may be granted
