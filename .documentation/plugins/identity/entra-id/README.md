# Microsoft Entra ID

Signs users in against Microsoft Entra ID using the OIDC authorization-code flow on the v2.0 endpoints, and can pull group membership from Microsoft Graph.

- **Slug** `identity-entra-id`, **version** 2026.09.01, **provider type** `entra_id`
- **Host range** `>=2026.09.01`
- **Where it appears** Security → Identity Providers, as a provider type
- **Gated by** `identity_providers.view` to read, `identity_providers.manage` to add or change one

## How it works

- `tenant_id` accepts a tenant GUID, `common`, or `organizations`. A GUID restricts login to that tenant, which is usually what you want.
- Groups come from `/v1.0/me/memberOf`. Enabling group fetch means the token needs Graph scope: set `scopes` to `https://graph.microsoft.com/.default` and consent the Graph permission, rather than leaving the OIDC-only default.
- Groups arrive as object ids, not display names, unless the tenant is configured to emit names. Map them in the group mapper accordingly.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `SupportsGroupFetch` | Yes | The provider can return the groups a user belongs to, which the group mapper turns into McHarbor roles. |
| `SupportsMetadata` | No | The plugin serves SP metadata the IdP can consume. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/identity/entra-id.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Identity providers](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
