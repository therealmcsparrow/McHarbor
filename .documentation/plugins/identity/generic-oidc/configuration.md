# Generic OpenID Connect: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- An OIDC client at the IdP with a redirect URI pointing at this McHarbor host, and its client id and secret.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `issuer_url` | string | Yes | — | The OIDC issuer URL. The plugin fetches <issuer_url>/.well-known/openid-configuration to discover authorization and token endpoints. |
| `client_id` | string | Yes | — | OAuth client ID assigned by your IdP to the McHarbor application. |
| `client_secret` | secret | Yes | — | OAuth client secret issued by your IdP. Stored encrypted at rest (AES-256-GCM). |
| `scopes` | string | No | `openid profile email` | Space-separated OAuth scopes. The default covers standard OIDC claims. Add 'groups' if your IdP exposes a group claim under that name. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Open **Security → Identity Providers** and add a provider of this type.
3. Register McHarbor at the identity provider using the redirect URI the form shows.
4. Fill in the credentials, save, and test the login from a private browser window.
5. Map directory groups to McHarbor roles in the group mapper.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `identity.adapter.register` | Register a provider type with the host identity registry. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |
| `encryption.handle.read` | Read the host encryption handle so it can decrypt the credentials it stored. |

## Related

- [Generic OpenID Connect](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
