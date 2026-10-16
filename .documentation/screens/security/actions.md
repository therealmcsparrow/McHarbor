# Security: Actions

Everything you can do on the Security screen (certificates, users, groups, roles, scoped
roles, ABAC policies, API keys, identity providers and scanners) and the permission each
one needs. Two things are shown once and never again: a generated API key, and a TLS
change, which waits for a container restart.

| Action | Gated by | What it does |
|---|---|---|
| **Upload certificate** | `settings.manage` | PEM certificate and private key. Replaces the locally-issued certificate and is never auto-renewed |
| **Download root CA** | `settings.view` | Saves `mcharbor-root-ca.pem`. Install it once per machine to remove browser warnings |
| **Regenerate** | `settings.manage` | Issues a new server certificate, keeping the root. Refused when a certificate was uploaded |
| **Copy fingerprint** | `settings.view` | The served certificate's SHA-256, for comparing against the browser or `docker logs` |
| **Create / disable user** | `users.manage` | Disabling ends access immediately while keeping history |
| **Reset password** | `users.manage` | Generate a password, or send a reset link |
| **Revoke session** | `sessions.manage` | Ends one of a user’s sessions |
| **Create group** | `groups.manage` | A named collection of users carrying roles |
| **Create role** | `roles.manage` | A named permission set; system roles cannot be deleted |
| **Assign scoped role** | `permissions.assign` | A role granted only on one environment or stack. It grants that role's environment-, stack- and owner-scoped keys there; its global and account keys (`users.manage`, `roles.manage`, `environments.view`, `dashboard.view`, `alerts.*` and the like) count only when the role is assigned globally |
| **Create / test policy** | `rbac policies manage` | ABAC rules that run after RBAC, with a tester |
| **Create API key** | `api_keys.manage` | Roles, optional extra permissions, environments, expiry. Shown once |
| **Revoke key** | `api_keys.manage` | Stops it working |
| **Delete revoked key** | `api_keys.manage` | Removes the record; its audit history stays |
| **Add identity provider** | `identity_providers.manage` | Needs an identity plugin installed |
| **Configure scanner** | `scans.manage` | Needs a scanner plugin installed |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Security](./README.md): what the screen is and what it shows
- [HTTPS](./https.md): the certificate model these actions operate on
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
