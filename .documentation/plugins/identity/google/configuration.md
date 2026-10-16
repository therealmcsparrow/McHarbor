# Google: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- An OAuth 2.0 client in Google Cloud (**APIs & Services → Credentials**) with a redirect URI pointing at this McHarbor host.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `client_id` | string | Yes | — | OAuth 2.0 client ID from the Google Cloud console (APIs & Services > Credentials). |
| `client_secret` | secret | Yes | — | OAuth client secret issued by Google. Stored encrypted at rest (AES-256-GCM). |
| `domain` | string | No | — | Optional Google Workspace hosted domain (e.g. example.com). When set, the plugin passes it as the hd= query parameter so only accounts from that Workspace can sign in. Leave empty to allow personal Google accounts. |
| `scopes` | string | No | `openid profile email` | Space-separated OAuth scopes. Defaults to openid profile email. Add https://www.googleapis.com/auth/admin.directory.group.readonly to enable admin group fetching via the Directory API. |
| `service_account_json` | secret | No | — | Optional: contents of a Google service-account JSON key file with domain-wide delegation. Required for Admin SDK group fetching. Stored encrypted at rest. |
| `delegated_user_email` | string | No | — | Optional: the Workspace admin user the service account impersonates (e.g. admin@example.com). Required when service_account_json is set. |
| `delegated_user_access_token` | secret | No | — | Optional: manually-issued Google API access token to use for Admin SDK calls. Use this until v1.1 ships RS256 JWT signing for service-account assertions. |
| `user_email_for_lookup` | string | No | — | Optional: email of the user to look up groups for. Most operators leave this blank and let the host pass the authenticated user's email at runtime. |

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

- [Google](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
