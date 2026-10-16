# Identity provider setup

**Security → Identity → Add provider** opens a five-step wizard; **Edit** on a provider row opens
the same building blocks for an existing one. The protocol itself belongs to the identity plugin
(see [Identity provider plugins](../../../plugins/identity/README.md)); this page covers what the
host adds around it.

## Permissions

| Key | What it unlocks |
|---|---|
| `identity_providers.view` (or `settings.manage`) | The provider list and the endpoint URLs |
| `identity_providers.manage` (or `settings.manage`) | The wizard, metadata import, mapping preview, test connection, save |

## The steps

1. **Choose a provider**: one tile per installed identity plugin.
2. **Configure**: the display name, the plugin's own form, and for SAML-capable plugins
   **Import IdP metadata**. Below the form, **Addresses to register with the identity provider**
   lists the URLs with copy buttons.
3. **Access and groups**: automatic provisioning, **Default role for new users**, import groups
   by name, the group mapping editor, and **Preview group mapping**.
4. **Review and save**: every setting in one list. The provider is saved **disabled**.
5. **Provider saved**: the final URLs (now including the provider id), **Enable provider**,
   **Test connection**, and the mapping preview again, this time able to use real group names.

## Addresses to register

| Label | URL | For |
|---|---|---|
| Redirect URI (OIDC) | `<public URL>/api/identity-providers/callback` | Every OpenID Connect provider; shared |
| Assertion Consumer Service URL (SAML) | `<public URL>/api/identity-providers/<id>/acs` | SAML; needs the saved provider id |
| Service provider metadata URL (SAML) | `<public URL>/api/identity-providers/<id>/metadata` | SAML |
| Direct sign-in URL | `<public URL>/api/identity-providers/<id>/authorize` | Bookmarks, IdP-initiated links |

They are built server-side from the same public base URL the login flow uses
(`GET /api/identity-providers/endpoint-urls?providerId=`), so a reverse proxy or a configured
public URL is reflected exactly.

## Importing SAML metadata

Paste a **Metadata URL** or the XML and choose **Read metadata**
(`POST /api/identity-providers/metadata-import`). McHarbor reads the `EntityDescriptor` (or the
first IdP entity of an `EntitiesDescriptor`) and shows the IdP entity ID, the single sign-on and
logout URLs per binding, the NameID formats, and each certificate's SHA-256 fingerprint, subject
and expiry, each with a copy button.

**Fill in the form** copies only the values the plugin's form has a field for. The bundled
SAML 2.0 plugin takes the metadata **URL** (it fetches the document itself), so a URL import
fills `metadata_url`; a pasted XML document has nothing the plugin can take, and the parsed
values are there to check against the IdP console.

URL fetches are limited to http/https, 15 seconds, 5 MB and three redirects. When an import
fails the reason is named: not an http(s) URL, could not be fetched, not a SAML metadata
document, or no identity provider in it.

## Previewing the group mapping

**Preview group mapping** takes a list of provider groups (typed one per line, taken from the
connection test, or loaded from the provider when the plugin can list groups) and shows, for a
user signing in for the first time: which McHarbor groups they join and from which provider
groups, the roles each of those groups carries (and at which environment or stack), the default
role, and the groups that map to nothing. Groups that import-by-name would create are marked
"created on first sign-in".

`POST /api/identity-providers/mapping-preview` replays the same decisions a sign-in applies:
import-by-name first, then mappings in order (the last mapping naming a McHarbor group decides
it), and nothing at all when the login carried no groups.

## Who gains or loses roles

For a saved provider the preview also lists **Who gains or loses roles**. McHarbor records the
groups each user presented at their last sign-in through the provider, so the preview compares
the stored configuration with the proposed one for every such user: roles gained (green), roles
lost (red), groups joined and left. A role the user already holds directly is not counted as a
gain or a loss. Users who have not signed in since claims started being recorded are listed as
not evaluated.

Saving a changed mapping in **Edit** first opens **Apply the new group mapping?** with the same
list; **Save mapping** applies it. Changes take effect for each user at their next sign-in.

Removing a mapping does **not** remove memberships it already created: only groups a mapping
still names are kept in step with the provider.

## Known limits

- The bundled SAML plugin's own SP metadata advertises `/api/identity-providers/callback` as its
  ACS and `/api/identity-providers/metadata` as its metadata URL; the host routes are the
  per-provider ones above. Register the host URLs.

## Related

- [Security](../README.md)
- [Effective access](../access-review/README.md)
- [SCIM provisioning](../scim/README.md)
