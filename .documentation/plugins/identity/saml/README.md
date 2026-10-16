# SAML 2.0

Signs users in as a SAML 2.0 Service Provider against any SAML IdP: Okta, Auth0, Keycloak, ADFS, Google Workspace, Entra ID. SP metadata is generated for you; signing your own AuthnRequests is optional.

- **Slug** `identity-saml`, **version** 2026.09.01, **provider type** `saml_2_0`
- **Host range** `>=2026.09.01`
- **Where it appears** Security → Identity Providers, as a provider type
- **Gated by** `identity_providers.view` to read, `identity_providers.manage` to add or change one

## How it works

- The IdP's SSO endpoint and signing certificate are read from the metadata document, so a certificate rollover at the IdP is picked up without editing the configuration.
- Leave `entity_id` blank to use the McHarbor metadata URL as the entity id. Whatever you use has to match what the IdP has on file.
- Fill in `sp_certificate` and `sp_private_key` only when the IdP demands signed AuthnRequests. Generate them with `openssl req -x509 -newkey rsa:2048 -nodes`.
- This adapter reports no group fetch; groups arrive only as assertion attributes, such as the `.../claims/groups` attribute an ADFS or Entra IdP sends.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `SupportsGroupFetch` | No | The provider can return the groups a user belongs to, which the group mapper turns into McHarbor roles. |
| `SupportsMetadata` | Yes | The plugin serves SP metadata the IdP can consume. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/identity/saml.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Identity providers](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
