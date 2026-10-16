# Identity providers and SCIM

31 endpoints: single sign-on providers, and the SCIM 2.0 surface an external directory uses
to provision accounts.

| Key | Covers |
|---|---|
| — | Identity provider CRUD and the SSO flow routes |
| `settings.manage` or `identity_providers.view` | Endpoint URLs for provider setup |
| `settings.manage` or `identity_providers.manage` | SAML metadata import, group-mapping preview |
| `scim.view` | List SCIM endpoints and sync history |
| `scim.manage` | Create, edit, delete and trigger SCIM endpoints; dry-run a sync, rotate the bearer token |

## Things worth knowing first

**Identity providers are plugins.** The host owns the flow, the plugin owns the protocol. The
only thing the host validates on create is that the plugin runtime recognises the requested
`providerType`; in other words, that the matching plugin is installed. Protocol-specific
field validation happens inside the plugin's own begin-auth and test paths.

**A configuration change needs the plugin loaded.** `PUT /api/identity-providers/{id}` with a
`config` is refused with `400 err.identity.invalid_type` when the provider's plugin is not
installed or not loaded, because without the plugin McHarbor cannot tell which keys are secret and
would store them unencrypted. Install or enable the plugin, then save again.

**Six routes are public by necessity.** The provider list on the login page, the authorize
redirect, the OIDC callback, the SAML assertion consumer service in both its verbs, and the
SAML metadata document all run for someone who is not signed in yet. They sit in the
rate-limited auth group.

**A failed provider test is a 200** with a per-step report whose overall status is `fail`,
matching storage locations, communications and AI providers. Only a missing row (404) or an
internal fault (500) is a non-200.

**Group mapping is optional and two-directional.** `groupMappings` maps provider groups onto
McHarbor groups; `autoImportGroups` pulls the provider's groups in;
`GET /api/identity-providers/{id}/groups` fetches them live from the provider's API so an
admin can pick from a real list.

**SCIM is two surfaces, not one.** `/api/scim/endpoints/*` is the admin UI (configured
endpoints, their schedules, their sync history) and is session-protected. `/api/scim/v2/*`
is the RFC 7644 surface an external directory calls, mounted public with its own bearer
middleware, because Okta or Entra has no McHarbor session.

**A SCIM bearer token is shown once.** Supply one on create, or let the service generate a
32-byte token and return the plaintext in `bearerToken`. On update, the token is only
overwritten when a non-empty value is supplied.

**`sync-now` enqueues, it does not run.** The background poller holds the only per-endpoint
sync lock, so the endpoint sets a "force now" marker the poller picks up on its next tick.
Watch `GET /api/scim/endpoints/{id}/history` for the outcome.

**McHarbor roles map onto SCIM groups.** `/api/scim/v2/Groups` returns roles; members of a
role are members of the group. `/api/scim/v2/Users` supports the RFC's `filter`, `startIndex`
and `count` parameters, with `count` defaulting to 25 and capped at 100.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/identity-providers` | `settings.manage` or `identity_providers.view` | Returns all identity providers | — |
| `POST` | `/api/identity-providers` | `settings.manage` or `identity_providers.manage` | Creates a new identity provider | — |
| `GET` | `/api/identity-providers/callback` | — | Processes the OIDC callback after provider authentication | `?code=`, `?error=`, `?error_description=`, `?state=` |
| `GET` | `/api/identity-providers/enabled` | — | Returns the list of enabled providers for the login page | — |
| `GET` | `/api/identity-providers/endpoint-urls` | `settings.manage` or `identity_providers.view` | Returns the callback, ACS and metadata URLs, for a named provider when one is given | `?providerId=` |
| `POST` | `/api/identity-providers/mapping-preview` | `settings.manage` or `identity_providers.manage` | Shows what a group-mapping configuration would grant, for sample groups and for users who already signed in. Read-only | — |
| `POST` | `/api/identity-providers/metadata-import` | `settings.manage` or `identity_providers.manage` | Reads SAML IdP metadata from a URL or pasted XML and returns what it found plus the config keys to prefill | — |
| `DELETE` | `/api/identity-providers/{id}` | `settings.manage` or `identity_providers.manage` | Removes an identity provider | `{id}` |
| `GET` | `/api/identity-providers/{id}` | `settings.manage` or `identity_providers.view` | Returns a single identity provider | `{id}` |
| `PUT` | `/api/identity-providers/{id}` | `settings.manage` or `identity_providers.manage` | Updates an identity provider | `{id}` |
| `GET` | `/api/identity-providers/{id}/acs` | — | Processes a SAML assertion consumer service callback | `{id}` |
| `POST` | `/api/identity-providers/{id}/acs` | — | Processes a SAML assertion consumer service callback | `{id}` |
| `GET` | `/api/identity-providers/{id}/authorize` | — | Initiates the OIDC/SAML flow by redirecting to the provider's auth page | `{id}` |
| `GET` | `/api/identity-providers/{id}/groups` | `settings.manage` or `identity_providers.manage` | Fetches groups from the provider's API | `{id}` |
| `GET` | `/api/identity-providers/{id}/metadata` | — | Returns service-provider metadata for a configured SAML provider | `{id}` |
| `POST` | `/api/identity-providers/{id}/test` | `settings.manage` or `identity_providers.manage` | Runs the per-step connection test against an identity provider and returns the full report | `{id}` |
| `GET` | `/api/scim/endpoints` | `scim.view` | Returns every configured SCIM endpoint | — |
| `POST` | `/api/scim/endpoints` | `scim.manage` | Validates input and inserts a new endpoint | — |
| `DELETE` | `/api/scim/endpoints/{id}` | `scim.manage` | Removes an endpoint and (via cascade) its sync log | `{id}` |
| `GET` | `/api/scim/endpoints/{id}` | `scim.view` | Returns a single endpoint by id | `{id}` |
| `PUT` | `/api/scim/endpoints/{id}` | `scim.manage` | Applies changes to an existing endpoint | `{id}` |
| `POST` | `/api/scim/endpoints/{id}/dry-run` | `scim.manage` | Pulls the remote directory and returns the plan a real sync would apply, writing only a dry-run history row; a failed pull is `200` with status `failed` | `{id}` |
| `GET` | `/api/scim/endpoints/{id}/history` | `scim.view` | Returns the most-recent sync log entries for an endpoint | `{id}`; `?limit=` |
| `POST` | `/api/scim/endpoints/{id}/rotate-token` | `scim.manage` | Issues a new bearer token and revokes the old one in the same write; the plaintext is in this response only | `{id}` |
| `POST` | `/api/scim/endpoints/{id}/sync-now` | `scim.manage` | Triggers an immediate pull for the endpoint | `{id}` |
| `GET` | `/api/scim/v2/Groups` | — | Returns the SCIM 2.0 /Groups collection | `?count=`, `?startIndex=` |
| `GET` | `/api/scim/v2/Groups/{id}` | — | Returns a single SCIM group by id (role id) | — |
| `GET` | `/api/scim/v2/Schemas` | — | Returns the SCIM 2.0 schema documents | — |
| `GET` | `/api/scim/v2/ServiceProviderConfig` | — | Returns the SCIM 2.0 service provider configuration | — |
| `GET` | `/api/scim/v2/Users` | — | Returns the SCIM 2.0 /Users collection | `?count=`, `?filter=`, `?startIndex=` |
| `GET` | `/api/scim/v2/Users/{id}` | — | Returns a single SCIM user resource by id | — |


## Request bodies

**`POST` `/api/identity-providers`**: `name`, `providerType`, `config`?, `clientId`, `clientSecret`, `tenantId`?, `domain`?, `issuerUrl`?, `metadataUrl`?, `entityId`?, `scopes`?, `autoProvision`?, `defaultRoleId`?, `groupMappingEnabled`?, `groupMappings`?, `autoImportGroups`?

**`POST` `/api/identity-providers/mapping-preview`**: `providerId`?, `providerType`?, `config`, `sampleGroups`

**`POST` `/api/identity-providers/metadata-import`**: `url`?, `xml`?

**`PUT` `/api/identity-providers/{id}`**: `name`?, `enabled`?, `config`?, `clientId`?, `clientSecret`?, `tenantId`?, `domain`?, `issuerUrl`?, `metadataUrl`?, `entityId`?, `scopes`?, `autoProvision`?, `defaultRoleID`?, `groupMappingEnabled`?, `groupMappings`?, `autoImportGroups`?

**`POST` `/api/scim/endpoints`**: `name`, `baseUrl`, `authType`, `bearerToken`?, `username`?, `password`?, `pollSchedule`, `enabled`

**`PUT` `/api/scim/endpoints/{id}`**: `name`, `baseUrl`, `authType`, `bearerToken`?, `username`?, `password`?, `pollSchedule`, `enabled`

## Related

- [Authentication](../conventions/authentication.md): the SSO flow
- [Users](./users.md): the accounts a provider creates
