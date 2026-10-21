# LDAP / Active Directory: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Bind succeeds, user search returns nothing | The filter or the base DN does not match the directory layout | Test the same filter with `ldapsearch` from a shell before blaming the plugin. |
| Active Directory logins fail with the default filter | `(uid=%s)` is not an AD attribute | Set `user_filter` to `(sAMAccountName=%s)` and `user_attr` to `sAMAccountName`. |
| StartTLS fails on an `ldaps://` URL | StartTLS upgrades a plain connection and is meaningless on an already-TLS one | Leave **Use StartTLS** off when the URL is `ldaps://`. |
| "The directory could not be reached" on the login page | The directory is down or unreachable, the service-account bind failed, or the provider is disabled | Check the container log for the `identity-ldap` warning, then test the provider. These attempts do not count towards lockout. |
| The route answers `503` with `host_unsupported` | The plugin is newer than the host and the host has no external-login capability | Update McHarbor; the plugin refuses to sign anyone in without the host's lockout and session handling. |
| A user is locked out after mistyped directory passwords | Directory failures count towards the same per-account and per-address lockout as local ones | Wait for the lock to expire, or unlock the account from the user's page as for a local account. |
| A correct directory password is refused as invalid | **Auto-provision** is off and no McHarbor account is linked to the entry or has its email address | Turn **Auto-provision** on, or create the account with the directory email first. |
| Signed in, but with no roles | No default role on the provider, and no group mapping matched | Set a default role, or map the groups the user carries (`memberOf`, or the `group_filter` search). |
| "identity-ldap: the LDAP server address is not an allowed destination" | The URL is, or resolves to, a link-local, metadata, multicast or unspecified address, or it is an `ldapi://` URL | Use `ldap://` or `ldaps://` with the server's routable address. `ldapi://` (Unix socket) is no longer accepted. |

Test a provider by signing in from a private browser window rather than by trusting the
form. A configuration that saves cleanly can still fail at the identity provider, and the
redirect is where that shows up.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i identity-ldap
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [LDAP / Active Directory](./README.md): what it is and how it works
