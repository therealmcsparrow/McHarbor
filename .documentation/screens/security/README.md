# Security

`/security` · `/security/users/:id` · `/security/groups/:id` · `/security/api-keys/:id` · `/security/scans` · `/security/policies`

Eighteen sections in seven groups, covering how McHarbor is served, who can get in and as what,
where identities come from, what gets scanned, how strong secrets have to be, and what
credentials the application itself holds.

## Permissions

| Key | What it unlocks |
|---|---|
| `users.view` / `users.manage` | Accounts, including service principals; `users.manage` also decides elevation requests and sets which permissions are requestable |
| `groups.view` / `groups.manage` | Groups |
| `roles.view` / `roles.manage` | Roles |
| `rbac.policies.view` / `rbac.policies.manage` | Attribute-based policies |
| `scim.view` / `scim.manage` | SCIM provisioning |
| `permissions.view` | Effective access, the permission catalog export, and the policy simulator |
| `approvals.view` / `approvals.manage` / `approvals.decide` | Approval requests, approval rules, and deciding someone else's request |
| `api_keys.view` / `api_keys.manage` | API keys |
| `identity_providers.view` / `.manage` | Identity providers |
| `scans.view` / `scans.manage` | Image scanning |
| `secrets.view` / `.manage` / `.reveal` / `.delete` / `.apply` | The credential vault |
| `settings.manage` | HTTPS, and the password and session policy |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- HTTPS: the certificate McHarbor is serving with and its fingerprint, the local root CA
  and its download, or "No certificate uploaded."
- Users, groups, roles, policies, SCIM and API keys.
- Identity providers, image scanners, and the password and session policy.

## The sections

| Group | Sections |
|---|---|
| **HTTPS** | `https` |
| **Management** | `users`, `groups`, `roles`, `policies`, `scim`, `api-keys`, `elevations`, `access` |
| **Governance** | `approvals`, `approval-rules`, `simulator`, `lifecycle`, `break-glass` |
| **Identity** | `identity` |
| **Scanning** | `scanning` |
| **Password** | `password` |
| **Secrets** | `secrets` |

`https`, `scanning` and `password` have no dedicated permission key today; they are
covered by `settings.manage` on the backend.

## API keys never escalate

A key’s access is roles plus optional additional permissions, and the form states the rule:
**"You can only grant what you hold yourself."** A role containing a permission you do not
have is refused with "You do not hold every permission in this role." The effective access
is the intersection of the key’s grants and its owner’s.

A new key is shown **once**: "Make sure to copy this key now. You won’t be able to see it
again."

## Automatic sign-out counts real input

"Only real input (typing, clicking, scrolling) counts as activity, so a dashboard left
open in a forgotten tab does not keep the session alive." Turn it off and sessions run to
their 24-hour limit.

## Password reset needs an email channel

Until an email channel is picked under **Password → Email delivery**, "reset links are
issued but never sent". The body must contain the link placeholder, and the form refuses to
save without it.

## In this folder

| Page | What it covers |
|---|---|
| [Policies](./policies/README.md) | Attribute-based policies: resource types from the host and plugins, the permissions picker, variables and functions |
| [Elevations](./elevations/README.md) | Time-bound permission requests, deciding them, and which permissions are requestable |
| [Service principals](./service-principals/README.md) | Accounts that act with nobody present (unattended runbooks), with roles but no sign-in, no password and no elevation |
| [HTTPS](./https.md) | The local certificate authority, fingerprint verification, rotation and agent pinning |
| [Requiring two-factor authentication](./mfa-enforcement/README.md) | `security.mfa_required` (off, administrators, everyone), the forced-enrolment screen, the server-side hold and the recovery path |
| [Installing the root CA](./install-root-ca.md) | Getting the root trusted on every platform, browser and device, and removing it again |
| [Secrets](./secrets.md) | The credential vault: file mode, references, rotation, reveal and the step-up window |
| [SCIM provisioning](./scim/README.md) | Dry runs, the sync preview, token rotation and the sync history |
| [Identity provider setup](./identity-setup/README.md) | The setup wizard, URLs to register, SAML metadata import, mapping preview and role impact |
| [Governance](./governance/README.md) | Destructive-action approvals, the policy simulator and conflict warnings, user lifecycle and bulk actions, break-glass, role templates and API key rotation |
| [Effective access](./access-review/README.md) | A user's permissions by environment and stack with their sources, and the catalog export |
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Identity provider plugins](../../plugins/identity/README.md): where logins come from
- [Image scanner plugins](../../plugins/scanners/README.md): what scanning needs
- [Audit](../audit/README.md): what these identities did
