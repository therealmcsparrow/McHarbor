# Service principals

`/security` → **Users** → **Service principal**

A service principal is an account that acts with nobody present: today, the principal an
[unattended runbook](../../../oncall/runbooks/unattended.md) runs as. It is a real row in the users
list, with ordinary role assignments shown and edited like anyone's, and it is audited under
its own id. It can never sign in.

## Permissions

| Key | What it unlocks |
|---|---|
| `users.manage` | Creating a service principal, assigning and removing its roles, deleting it |
| `account.manage` | Disabling and re-enabling it |
| `runbooks.automate` | Assigning a runbook to it, which is a separate decision from creating it |

## Creating one

**Service principal** in the users toolbar opens **New service principal**, which asks for:

| Field | Notes |
|---|---|
| **Username** | How it appears in the users list, in audit rows and as a run's **Started by** |
| **Display name** | Optional |
| **Role** | Required. The server refuses a service principal with no role (`err.user.service_principal_role`) |

There is no password field. A request that sends one is refused with
`err.user.service_principal_sign_in`. Over the API it is `POST /api/users` with
`"kind": "service"` and a `roleId`.

## How it differs from a person

| | A person | A service principal |
|---|---|---|
| Users list | Name | Name, labelled **Service principal** |
| Password | Yes | None; every password path refuses it |
| Sign-in / session | Yes | Never; it answers like a wrong password |
| API keys | Yes | A key it owned would not authenticate |
| Roles and overrides | Yes | Yes, the same editor |
| Elevations | Fold into an interactive session | **Never fold in** |
| Break-glass | Can be designated | Grants nothing |
| Audited as | Their id | Its id |

The kind is fixed when the account is created and cannot be changed afterwards.

## Keeping it narrow

Give a service principal the roles its runbooks need and nothing else, typically
`runbooks.run` plus the key of each action its runbooks take, such as `containers.manage`,
scoped to the environments involved. Every step of an unattended run is checked against these
roles, so a role left off is a step that is refused and recorded, not one that runs.

Disabling a service principal stops every unattended run that acts as it; each affected
runbook shows *the service principal is missing or disabled; nothing ran* on its editor page.

## Related

- [Unattended runs](../../../oncall/runbooks/unattended.md): what runs as a service principal and where it stops
- [Elevations](../elevations/README.md): why an elevation never reaches a service principal
