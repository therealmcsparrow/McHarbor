# SAML 2.0: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- The IdP's metadata URL, and an application registered at the IdP that trusts this McHarbor host as an SP.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `metadata_url` | string | Yes | — | URL of the IdP's SAML 2.0 metadata XML document. The plugin fetches this to discover the IdP's SSO endpoint + signing certificate. |
| `entity_id` | string | No | — | Optional SP entity ID. Leave empty to use the McHarbor metadata URL. Must match what's configured on the IdP side. |
| `sp_certificate` | secret | No | — | Optional PEM-encoded X.509 certificate. Required if the IdP requires signed AuthnRequests / assertions. Generate with `openssl req -x509 -newkey rsa:2048 -keyout sp.key -out sp.crt -days 365 -nodes -subj '/CN=mcharbor'`. Stored encrypted at rest. |
| `sp_private_key` | secret | No | — | Optional PEM-encoded RSA private key matching the SP certificate. Stored encrypted at rest. |

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

- [SAML 2.0](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
