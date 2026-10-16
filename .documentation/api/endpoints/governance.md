# Approvals, break-glass and access review

23 endpoints: the two-person approval queue and its rules, emergency break-glass sessions,
the permission simulator and conflict report, and the access-review exports.

| Key | Covers |
|---|---|
| `approvals.view` or `approvals.decide` | Read the approval queue |
| `approvals.view` or `approvals.manage` | Read the rules and the categories a rule can name |
| `approvals.manage` | Create, replace and delete rules |
| `approvals.decide` | Approve or deny a request, plus the rule's own approver permission |
| `users.view` | List break-glass accounts and sessions |
| `account.manage` | Designate or undesignate an account, end someone's session |
| `permissions.view` | Effective permissions and the catalog export |
| `permissions.view` or `rbac.policies.view` | The permission simulator |
| `roles.view` or `rbac.policies.view` | The conflict report |
| — | The caller's own requests and break-glass state; withdraw, run, activate, end |

Every key here is checked **globally**: a grant scoped to one environment does not satisfy it.

## Things worth knowing first

**No rule, no change.** A fresh install has no approval rules, so every action runs
immediately. When a rule covers a request, the route answers `202` with `success: false`,
`data.reason: "approval_required"` and `data.approvalId`, and nothing runs until someone other
than the requester approves it and the requester runs it.

**Running an approved request replays it.** `POST /api/approvals/{id}/execute` sends the stored
request through the full router as the requester, so every gate runs again and a permission
removed in the meantime is refused then. It runs exactly once.

**The simulator and the effective-permissions answers use the real evaluator.** The explanation
of why a key is granted or denied, which the simulator, the effective-permissions view and the
access review read, runs the same policy evaluation as an actual authorisation check, so an ABAC
policy that would deny a request is reported as denying it. Explaining records nothing: it writes
no ABAC-denied audit entry and raises no alert.

**The ungated routes act only on the caller.** Withdraw, run, break-glass activate and end are
exempt from permission middleware because the target is the caller's own request or session;
the handler refuses anyone else. Only an account designated for break-glass can activate.

**Break-glass is meant to be loud.** Activating raises an error-severity audit row and pages
everyone who manages accounts.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/access-review/effective-permissions` | `permissions.view` | Returns one user's effective permissions at an environment and stack, each key with the sources that grant or deny it | `?userId=`, `?environmentId=`, `?stack=` |
| `GET` | `/api/access-review/permission-catalog/export` | `permissions.view` | Streams the permission catalog as CSV or JSON for a security review; the export is audited | `?format=` |
| `GET` | `/api/approvals` | `approvals.view` or `approvals.decide` | Returns every approval request, for approvers and auditors. `status` is `pending`, `approved`, `denied`, `expired`, `cancelled`, `executing` or `executed` | `?status=` |
| `GET` | `/api/approvals/categories` | `approvals.view` or `approvals.manage` | Lists the risk categories a rule can name | — |
| `GET` | `/api/approvals/mine` | — | Returns the caller's own requests. Signed in; no key | `?status=` |
| `GET` | `/api/approvals/rules` | `approvals.view` or `approvals.manage` | Returns every approval rule | — |
| `POST` | `/api/approvals/rules` | `approvals.manage` | Stores a new rule | — |
| `DELETE` | `/api/approvals/rules/{id}` | `approvals.manage` | Removes a rule; requests already held keep their terms | `{id}` |
| `PUT` | `/api/approvals/rules/{id}` | `approvals.manage` | Replaces a rule; requests already held keep their terms | `{id}` |
| `GET` | `/api/approvals/{id}` | — | Returns one request to its requester, or to anyone who may read the queue; the handler checks, not the route | `{id}` |
| `POST` | `/api/approvals/{id}/cancel` | signed in; API key needs `account.self.manage` | Lets the requester withdraw an open request. Exempt: self-service, withdraws the caller's own request | `{id}` |
| `POST` | `/api/approvals/{id}/decide` | `approvals.decide` | Records an approve or deny from someone other than the requester; the rule's approver permission is checked on top | `{id}` |
| `POST` | `/api/approvals/{id}/execute` | signed in; API key needs `account.self.manage` | Replays an approved request exactly once, as its requester; a second call or a race finds it already spent (`409`). An API key needs `account.self.manage` to call it. Exempt: the replayed route re-checks its own permission | `{id}` |
| `GET` | `/api/break-glass/accounts` | `users.view` | Lists the accounts designated for break-glass | — |
| `DELETE` | `/api/break-glass/accounts/{userId}` | `account.manage` | Removes a designation | `{userId}` |
| `PUT` | `/api/break-glass/accounts/{userId}` | `account.manage` | Designates an account for break-glass, or updates its limit and note | `{userId}` |
| `POST` | `/api/break-glass/accounts/{userId}/end` | `account.manage` | Lets an account manager close someone else's emergency session | `{userId}` |
| `POST` | `/api/break-glass/activate` | — | Opens an emergency session for the caller. Raises an error-severity audit row and pages everyone who manages accounts. Needs a live `break_glass` step-up window on the caller's session (`403` `err.sudo_required` otherwise) and refuses an API key; `409` `already_active` when the caller already has an open session. Exempt: only a designated account can activate, for itself | — |
| `POST` | `/api/break-glass/end` | — | Closes the caller's own emergency session early. Exempt: self-service | — |
| `GET` | `/api/break-glass/sessions` | `users.view` | Lists recent emergency sessions | `?limit=` |
| `GET` | `/api/break-glass/status` | — | Returns the caller's own designation and session. Signed in; no key | — |
| `GET` | `/api/governance/conflicts` | `roles.view` or `rbac.policies.view` | Lists wildcard grants and shadowed ABAC policies | — |
| `POST` | `/api/governance/simulate` | `permissions.view` or `rbac.policies.view` | Answers allow or deny, with the reasons, for another user and a given permission or request | — |

## Request bodies

**`POST` `/api/approvals/rules`**: `category`, `environmentId`?, `enabled`?, `requiredApprovers`, `approverPermission`, `expiryMinutes`, `description`

**`PUT` `/api/approvals/rules/{id}`**: `category`, `environmentId`?, `enabled`?, `requiredApprovers`, `approverPermission`, `expiryMinutes`, `description`

**`POST` `/api/approvals/{id}/decide`**: `approve`, `note`

**`PUT` `/api/break-glass/accounts/{userId}`**: `maxMinutes`, `note`

**`POST` `/api/break-glass/activate`**: `reason`, `minutes`

**`POST` `/api/governance/simulate`**: `userId`, `environmentId`, `stackName`, `permission`, `method`, `path`

## Related

- [Approvals](../../screens/security/governance/approvals.md): categories, statuses, redaction of held bodies
- [Break-glass](../../screens/security/governance/break-glass.md)
- [Permission simulator](../../screens/security/governance/simulator.md)
- [Users, groups and elevations](./users.md): bulk account actions and the lifecycle report
- [Roles and policies](./roles.md)
