# LDAP / Active Directory

Authenticates users against LDAP or Active Directory with a search-then-rebind flow: bind as a service account, find the user, then rebind as that user with the supplied password. This is the one identity plugin where McHarbor sees the password, so it is also the one that signs users in through a plugin route rather than a redirect, while the host keeps lockout, MFA and the session to itself.

- **Slug** `identity-ldap`, **version** 2026.09.02, **provider type** `ldap`
- **Host range** `>=2026.09.01`
- **Where it appears** Security → Identity Providers, as a provider type
- **Gated by** `identity_providers.view` to read, `identity_providers.manage` to add or change one

## How it works

- `user_filter` defaults to `(uid=%s)`, which suits OpenLDAP. Active Directory wants `(sAMAccountName=%s)`, and `user_attr` should change to match.
- Use `ldaps://` on 636, or `ldap://` on 389 with **Use StartTLS**. Plain 389 without StartTLS sends the password in clear text.
- `skip_tls_verify` is for test environments only. It removes the check that the directory is the one you think it is.
- `group_filter` defaults to matching `member` or `memberUid` against the user's DN, and `group_attr` (default `cn`) is the name handed to the group mapper.

## Signing in

Every enabled LDAP provider appears on the login page as **Sign in with <provider name>**.
Choosing it turns the password form into a directory form: the same **Username** and
**Password** fields, a **Directory** line naming the provider, and no **Forgot password?**
link, because the directory owns that password. With more than one LDAP provider the
button reads **Sign in with a directory account** and the **Directory** line becomes a
picker. **Use a McHarbor account** goes back to the local form.

A directory sign-in is held to exactly the rules of a password sign-in:

- **Lockout.** A locked client address is refused before the directory is contacted, and
  a locked or disabled McHarbor account before the user's own bind, so a locked account
  never costs a bind or trips the directory's own lockout. Every rejected password and
  unknown username is written to `login_attempts` (with the resolved address and its
  `ip_source`) and counts towards both the per-account and the per-address lockout, under
  the same thresholds a local sign-in uses.
- **Second factor.** An account with MFA gets the same code step as a local account; an
  account the MFA policy requires to enrol is sent to enrolment.
- **Session and audit.** The host issues the session cookie and writes the same
  `login`, `login_failed` and `login_mfa_challenge` audit entries, tagged with
  `identityProviderId` and `method: ldap`.
- **Messages.** A wrong password, an unknown user and a locked account show the same
  messages a local sign-in shows. A directory that cannot be reached answers
  "The directory could not be reached…" (`503`, `err.identity.directory_unavailable`)
  and does **not** count towards lockout, so an outage cannot lock everyone out.

The first sign-in links the directory entry to a McHarbor account by its DN, or by email
when an account with that address already exists. Otherwise, with **Auto-provision**
on, it creates one named `ldap_<login name>` (or the email address, when the directory
has one) with the provider's default role; with it off, the sign-in is refused like a
wrong password. Group claims come from `memberOf`, or from a `group_filter` search when
the directory has no `memberOf`, and feed the provider's group mapping and auto-import.

## Sign-in route

The login page posts to the plugin's one public route, `POST /api/identity-ldap/auth`,
with a JSON body of `providerId`, `username` and `password`. It needs no permission
and sits behind the same per-client rate limit as `POST /api/auth/login` (10 requests a
minute). It answers exactly as `POST /api/auth/login` does: `200` with the user, `200`
with `mfaRequired` and a `challengeId` (finished at `POST /api/auth/login/mfa`), `401`,
`403` for a disabled account, or `423` with `retryAfterSeconds`, plus `503` when the
directory is unreachable.

The plugin needs a host that carries the external-login capability. On an older host the
route answers `503` with the code `host_unsupported` and never contacts the directory.

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
| [Developer](../../../development/plugins/identity/ldap.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Identity providers](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
