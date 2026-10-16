# Microsoft Entra ID: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- An Entra app registration with a Web redirect URI pointing at this McHarbor host, and a client secret.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `tenant_id` | string | Yes | — | Azure AD tenant ID (UUID). Use 'common' for multi-tenant apps, 'organizations' for any work/school account, or leave 'common' for personal accounts too. |
| `client_id` | string | Yes | — | Application (client) ID from the App Registration overview page. |
| `client_secret` | secret | Yes | — | Client secret value from the App Registration's Certificates & secrets page. Stored encrypted at rest. |
| `scopes` | string | No | `openid profile email` | Space-separated OAuth scopes. The Microsoft Graph default is 'https://graph.microsoft.com/.default' when group fetching is enabled. |

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

- [Microsoft Entra ID](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
