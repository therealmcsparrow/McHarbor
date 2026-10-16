# Governance

**Security → Governance**

Five destinations that decide who may do something dangerous, and prove it afterwards:
destructive-action **approvals** and the **approval rules** that trigger them, the **policy
simulator** with conflict warnings, the **user lifecycle** view with bulk actions, and
**break-glass** emergency access. Role templates and API key rotation, which live on the Roles
and API keys tabs, are described here too because they are part of the same governance work.

## Permissions

| Key | Scope | What it unlocks |
|---|---|---|
| `approvals.view` | global | Every approval request and its decisions; the rules list |
| `approvals.manage` | global | Create, edit and delete approval rules |
| `approvals.decide` | global | Approve or deny someone else's request |
| `permissions.view` or `rbac.policies.view` | global | The policy simulator |
| `roles.view` or `rbac.policies.view` | global | Conflict warnings on Roles and Policies |
| `users.view` | global | The user lifecycle view, break-glass accounts and history |
| `account.manage` | account | Bulk disable, enable and unlock; designating break-glass accounts; ending someone's session |
| `users.manage` | account | Bulk delete |
| `api_keys.manage` | environment | Rotating an API key |

Anyone signed in sees **their own** approval requests and their own break-glass status; those
routes carry no key because they only ever return the caller's rows.

## The destinations

| Tab | Page |
|---|---|
| Approvals, Approval rules | [Approvals](./approvals.md) |
| Policy simulator | [Policy simulator and conflicts](./simulator.md) |
| User lifecycle | [User lifecycle and bulk actions](./user-lifecycle.md) |
| Break-glass | [Break-glass](./break-glass.md) |
| Roles → *Start from a preset* | [Role templates](./role-templates.md) |
| API keys → *Rotate key* | [API key rotation](./api-key-rotation.md) |

## Related

- [Effective access](../access-review/README.md): a user's permissions and their sources
- [Security actions](../actions.md) and [fields](../fields.md)
- [Audit](../../audit/README.md): every action on these pages writes an audit row
