# Generic OpenID Connect

Signs users in against any compliant OpenID Connect provider (Auth0, Okta, Keycloak, Cognito, Ping, ForgeRock) by discovering the endpoints from `<issuer>/.well-known/openid-configuration`.

- **Slug** `identity-generic-oidc`, **version** 2026.09.01, **provider type** `generic_oidc`
- **Host range** `>=2026.09.01`
- **Where it appears** Security → Identity Providers, as a provider type
- **Gated by** `identity_providers.view` to read, `identity_providers.manage` to add or change one

## How it works

- Only the issuer is configured; authorization and token endpoints are read from the discovery document, so an IdP that moves an endpoint keeps working.
- This adapter reports **no** group fetch. It reads what the ID token carries, so groups only reach McHarbor if the IdP emits them as a claim. Add `groups` to `scopes` and configure the claim at the IdP.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `SupportsGroupFetch` | No | The provider can return the groups a user belongs to, which the group mapper turns into McHarbor roles. |
| `SupportsMetadata` | No | The plugin serves SP metadata the IdP can consume. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/identity/generic-oidc.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Identity providers](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
