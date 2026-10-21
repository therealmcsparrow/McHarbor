# LDAP / Active Directory: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A directory reachable from the McHarbor container, and usually a read-only service account, because most directories will not return user entries to an anonymous bind.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `url` | string | Yes | — | LDAP server URL: ldap://host:389 for plain, ldaps://host:636 for implicit TLS. Use StartTLS instead of LDAPS when the server listens on 389. `ldapi://` is refused. |
| `bind_dn` | string | No | — | DN of a service account used to search for users and groups. Empty = anonymous bind (most directories refuse to return user entries anonymously). Example: cn=mcharbor-svc,ou=service,dc=example,dc=com. |
| `bind_password` | secret | No | — | Password for the service account. Stored encrypted at rest (AES-256-GCM). |
| `user_base_dn` | string | Yes | — | Base DN under which user entries live. Searches for incoming usernames are scoped to this subtree. Example: ou=users,dc=example,dc=com. |
| `user_filter` | string | No | — | LDAP filter template for looking up a user by their login name. The placeholder %s is replaced with the escaped username. Default (uid=%s) works for OpenLDAP; for Active Directory use (sAMAccountName=%s) or (&(objectCategory=person)(objectClass=user)(sAMAccountName=%s)). |
| `user_attr` | string | No | — | Attribute that contains the login name. Default: uid (OpenLDAP). Active Directory: sAMAccountName. |
| `email_attr` | string | No | — | Attribute that contains the user's email address. Default: mail. |
| `name_attr` | string | No | — | Attribute that contains the user's display name. Default: cn. |
| `start_tls` | boolean | No | — | When the URL is ldap://, upgrade the connection to TLS via the StartTLS extended operation. Has no effect when the URL is already ldaps://. |
| `skip_tls_verify` | boolean | No | — | Skip certificate verification. Test/development only; do NOT enable in production. |
| `group_base_dn` | string | No | — | Base DN for group searches. Defaults to User Base DN. Example: ou=groups,dc=example,dc=com. |
| `group_filter` | string | No | — | Filter template for fetching groups a user belongs to. The %s placeholder is replaced with the user's full DN. Default: (\|(member=%s)(uniqueMember=%s)). |
| `group_attr` | string | No | — | Attribute used as the group name in McHarbor's group mapper. Default: cn. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Destination rules

The plugin checks the address it resolved, right before it connects, and refuses an address that is never a real directory: `0.0.0.0/8`, link-local (`169.254.0.0/16`, `fe80::/10`), the cloud metadata endpoints (`169.254.169.254`, `100.100.100.200`, `fd00:ec2::/32`), multicast, broadcast and `::`. IPv6 forms that carry one of these (NAT64, 6to4, Teredo, ISATAP, IPv4-compatible) are refused as well. A name that resolves to a refused address is refused like the address itself. Loopback, private ranges (RFC 1918) and ULA stay allowed, so a directory on your LAN or on the McHarbor host works. `ldapi://` URLs (a Unix socket) are refused too, so an LDAP server must be reached over `ldap://` or `ldaps://`. A refusal reads "identity-ldap: the LDAP server address is not an allowed destination".

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Open **Security → Identity Providers** and add a provider of this type.
3. Fill in the directory fields (LDAP has no redirect URI to register) and save.
4. Test the login from a private browser window with **Sign in with <provider name>**.
5. Map directory groups to McHarbor roles in the group mapper.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `identity.adapter.register` | Register a provider type with the host identity registry. |
| `identity.read` | Receive its own provider's configuration, with the bind password decrypted by the host. |
| `identity.write` | Have the host lock out, provision and sign in the user it authenticated; the host issues the session. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [LDAP / Active Directory](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
