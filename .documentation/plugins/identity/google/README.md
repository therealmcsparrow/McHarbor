# Google

Signs users in with Google, for personal accounts or a Workspace tenant, and can optionally read group membership from the Admin SDK Directory API.

- **Slug** `identity-google`, **version** 2026.09.01, **provider type** `google`
- **Host range** `>=2026.09.01`
- **Where it appears** Security → Identity Providers, as a provider type
- **Gated by** `identity_providers.view` to read, `identity_providers.manage` to add or change one

## How it works

- Setting **Workspace Domain** passes `hd=` on the authorization request so only accounts in that domain can sign in. Leave it blank and any Google account can attempt login.
- Group fetch needs a service account with domain-wide delegation, the `admin.directory.group.readonly` scope, and `delegated_user_email` naming a Workspace admin to impersonate.
- `delegated_user_access_token` is an explicit stopgap: it lets you paste a manually issued token until RS256 JWT signing for service accounts ships. It is not a long-term configuration, because the token expires.

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
| [Developer](../../../development/plugins/identity/google.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Identity providers](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
