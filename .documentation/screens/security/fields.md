# Security: Fields

Every column and form field across the Security screen's sections (users, groups, roles,
API keys and scan history), plus the HTTPS upload, the automatic sign-out window and
password-reset delivery.

## Columns

| Column | What it is |
|---|---|
| Users | Username, email, display name, role, status, last login, created |
| Groups | Name, description, members, roles |
| Roles | Name, description, permission count, system or custom |
| API keys | Name, key, scopes, expires, last used, status |
| Scan history | Image, when, status, findings |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Per section, over its own list |
| Status | Active, revoked or expired, on API keys |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Certificate (PEM), Private key (PEM) | secret | The HTTPS upload |
| SHA-256 fingerprint | read-only | The served certificate, with a copy button. Compare against the browser or the startup log |
| Root CA fingerprint, Root CA expires | read-only | The local certificate authority, when one exists |
| Enable HTTPS, Force HTTPS | toggle | Serve over TLS; redirect via `X-Forwarded-Proto` |
| Roles, Additional permissions, Environments, Expiry | mixed | On an API key. Empty environments means every environment |
| Inactivity window (seconds) | number | Automatic sign-out, with the equivalent shown in minutes |
| Reset link lifetime (minutes) | number | Between 5 minutes and 7 days |
| Email channel, From address, Subject, Message body | mixed | Password reset delivery; the body must contain the link placeholder |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Security](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
