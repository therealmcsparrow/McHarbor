# Identity Providers

An identity-provider plugin lets people sign in to McHarbor with an account they already
have. The host owns the provider rows, the session it issues, the group mapper and the
login page; the plugin supplies the protocol: start a login, handle the callback, and
where the protocol allows it, fetch groups.

| Plugin | Provider type | Group fetch | What it authenticates against |
|---|---|---|---|
| [Generic OpenID Connect](./generic-oidc/README.md) | `generic_oidc` | Claims only | Generic OpenID Connect identity provider. |
| [Google](./google/README.md) | `google` | Yes | Google identity provider. |
| [LDAP / Active Directory](./ldap/README.md) | `ldap` | Yes | LDAP / Active Directory identity provider. |
| [Microsoft Entra ID](./entra-id/README.md) | `entra_id` | Yes | Microsoft Entra ID identity provider. |
| [SAML 2.0](./saml/README.md) | `saml_2_0` | Claims only | SAML 2.0 identity provider. |

Each plugin has a folder of its own: a README, the configuration reference, a
troubleshooting table, and a developer page covering the adapter it registers and how to
build, sign and fork it.

## What they have in common

- They register through `AdapterRegistry.Identity()` and appear under
  **Security → Identity Providers**.
- Client secrets, service-account JSON and SP private keys are encrypted at rest with
  AES-256-GCM.
- Group fetch is what feeds the group mapper, which turns a directory group into a
  McHarbor role. Without it, every federated user lands on the default role and has to be
  adjusted by hand.
- **LDAP is the odd one out.** Every other provider redirects the browser to the identity
  provider; LDAP takes the password in McHarbor's own form and checks it against the
  directory itself. The host still owns everything around that check (lockout, MFA, the
  session) through the external-login capability described in
  [LDAP / Active Directory: Developer](../../development/plugins/identity/ldap.md).

## Which one

| Your directory | Use |
|---|---|
| Microsoft 365 / Entra ID | Microsoft Entra ID |
| Google Workspace | Google |
| Okta, Auth0, Keycloak, Cognito, Ping | Generic OpenID Connect, or SAML 2.0 if the app is SAML-only |
| On-premise AD or OpenLDAP with no IdP in front | LDAP / Active Directory |

Prefer OIDC over SAML when the identity provider offers both: discovery keeps working
through endpoint and certificate changes that would otherwise need a manual re-import.

## Related

- [Identity provider setup](../../screens/security/identity-setup/README.md): the host wizard, URLs, metadata import and mapping preview
- [Plugin catalog](../catalog.md): every shipped plugin in one table
- [Plugins](../README.md): the section index
