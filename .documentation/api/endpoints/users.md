# Users, groups and elevations

41 endpoints: accounts and their lifecycle, group membership, role assignments, per-user
permission overrides, session administration, and the elevation queue.

| Key | Covers |
|---|---|
| `users.view` | List and inspect users, their roles, groups and overrides; the lifecycle report; break-glass accounts and sessions |
| `users.manage` | Create, update, delete users (singly or in bulk); assign and unassign roles |
| `account.manage` | Disable, enable, unlock an account, singly or in bulk; designate break-glass accounts |
| `account.password.reset` | Set a password outright, or force a reset at next login |
| `sessions.manage` | List and revoke another user's sessions |
| `groups.view` / `groups.manage` | Groups, members and their role assignments |
| `rbac.policies.manage` | Replace a user's permission overrides |

## Things worth knowing first

**Deleting and disabling are different answers.** Delete removes the row; disable keeps it,
with a reason, and refuses if the target is the last administrator
(`err.user.last_admin`) or the caller themselves.

**Accounts provisioned by an identity provider are locked** against local edits to the fields
the provider owns: profile, password, disable. Those return
`err.user.identity_provider_locked`.

**Role assignments carry a scope.** `POST /api/users/{id}/roles` takes an optional
`environmentId` or `stackName`, which is how a role is granted on one environment or one
stack rather than globally. Groups take the same shape. A role granted this way contributes only
its environment-, stack- and owner-scoped keys: its `global` and `account` keys are ignored until
the role is assigned globally, so a per-environment role of `users.manage` cannot be used to
manage users. See [Permissions](../conventions/permissions.md#a-role-granted-on-one-environment-carries-only-environment-scoped-keys).

**Overrides are replaced atomically.** `PUT /api/users/{id}/overrides` takes the whole set
and writes one audit entry summarising the diff, rather than one entry per key.

**Session administration is split by target.** Reading your own sessions is
`GET /api/auth/sessions` with no permission; reading someone else's is
`GET /api/users/{id}/sessions` under `sessions.manage`. Revoking every session for a user
preserves the caller's own unless `?includeSelf=true` says otherwise, and the caller can
never revoke their own current session through these routes; that is what logout is for.

**Elevations are the request queue for permissions a user does not hold.** Creating,
listing your own and withdrawing take no permission, because they only touch rows the caller
owns. The inbox and the decision are `users.manage`. A permission that is not marked
elevatable is refused with `err.permission_not_elevatable`.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `POST` | `/api/elevations` | signed in; API key needs `account.self.manage` | Validates and inserts a new elevation request. `429` `err.rate_limit_exceeded` with `reason` `too_many_pending` (five pending per user) or `already_pending` (same key and environment) | — |
| `GET` | `/api/elevations/elevatable` | — | Lists every host and plugin permission with whether it can be requested, and whether an admin or the default rule decided | — |
| `PUT` | `/api/elevations/elevatable` | `users.manage` | Switches one permission requestable or not; audited as an `elevation_policy` update | — |
| `GET` | `/api/elevations/all` | `users.manage` | Returns every request (admin only, gated by route) | — |
| `GET` | `/api/elevations/mine` | — | Returns the requester's own queue | — |
| `GET` | `/api/elevations/pending` | `users.manage` | Returns the admin inbox | — |
| `DELETE` | `/api/elevations/{id}` | signed in; API key needs `account.self.manage` | Lets the requester cancel their own pending request before an admin acts on it | `{id}` |
| `GET` | `/api/elevations/{id}` | requester, or `users.manage` (global) | Returns a single request. Anyone else gets `404`, so the request's existence does not leak | `{id}` |
| `POST` | `/api/elevations/{id}/decide` | `users.manage` | Approves or denies a pending request | `{id}` |
| `GET` | `/api/groups` | `groups.view` | Returns all groups with member counts | — |
| `POST` | `/api/groups` | `groups.manage` | Creates a new group | — |
| `DELETE` | `/api/groups/{id}` | `groups.manage` | Removes a group | `{id}` |
| `GET` | `/api/groups/{id}` | `groups.view` | Returns a group with members and role assignments | `{id}` |
| `PUT` | `/api/groups/{id}` | `groups.manage` | Updates an existing group | `{id}` |
| `POST` | `/api/groups/{id}/members` | `groups.manage` | Adds a user to a group | `{id}` |
| `DELETE` | `/api/groups/{id}/members/{userId}` | `groups.manage` | Removes a user from a group | `{id}`, `{userId}` |
| `POST` | `/api/groups/{id}/roles` | `groups.manage` | Assigns a role to a group | `{id}` |
| `DELETE` | `/api/groups/{id}/roles/{assignmentId}` | `groups.manage` | Unassigns a role from a group | `{assignmentId}`, `{id}` |
| `GET` | `/api/users` | `users.view` | Returns a paginated list of users | — |
| `POST` | `/api/users` | `users.manage` | Creates a local user | — |
| `POST` | `/api/users/bulk/account` | `account.manage` | Previews (`dryRun`) or applies `disable`, `enable` or `unlock` to a selection | — |
| `POST` | `/api/users/bulk/delete` | `users.manage` | Previews or applies deletion of a selection; falls under the `user.delete` approval rule (not its dry run) | — |
| `GET` | `/api/users/lifecycle` | `users.view` | Returns every account with its lifecycle flags and counts | `?inactiveDays=`, `?elevatedDays=` |
| `DELETE` | `/api/users/{id}` | `users.manage` | Removes a user | `{id}` |
| `GET` | `/api/users/{id}` | `users.view` | Returns a single user | `{id}` |
| `PUT` | `/api/users/{id}` | `users.manage` | Updates an existing user | `{id}` |
| `POST` | `/api/users/{id}/disable` | `account.manage` | Marks a user as disabled | `{id}` |
| `POST` | `/api/users/{id}/enable` | `account.manage` | Re-enables a previously disabled user | `{id}` |
| `POST` | `/api/users/{id}/force-reset-password` | `account.password.reset` | Stamps must_reset_password on the user | `{id}` |
| `GET` | `/api/users/{id}/groups` | `users.view` | Returns group memberships for a user | `{id}` |
| `GET` | `/api/users/{id}/overrides` | `users.view` | Returns the user's per-permission grant/deny overrides | `{id}` |
| `PUT` | `/api/users/{id}/overrides` | `rbac.policies.manage` or `permissions.assign` | Replaces the user's per-permission grant/deny overrides atomically | `{id}` |
| `PUT` | `/api/users/{id}/password` | — | Changes a user's password | `{id}` |
| `POST` | `/api/users/{id}/password/admin` | `account.password.reset` | Overwrites a user's password without requiring the current one | `{id}` |
| `GET` | `/api/users/{id}/roles` | `users.view` | Returns role assignments for a user | `{id}` |
| `POST` | `/api/users/{id}/roles` | `users.manage` | Assigns a role to a user | `{id}` |
| `DELETE` | `/api/users/{id}/roles/{assignmentId}` | `users.manage` | Removes a role assignment from a user | `{assignmentId}`, `{id}` |
| `GET` | `/api/users/{id}/sessions` | `sessions.manage` | Returns the active + recent sessions for the target user | `{id}` |
| `POST` | `/api/users/{id}/sessions/revoke-all` | `sessions.manage` | Revokes every session for the target user | `{id}`; `?includeSelf=` |
| `DELETE` | `/api/users/{id}/sessions/{sessionId}` | `sessions.manage` | Revokes a single session | `{id}`, `{sessionId}` |
| `POST` | `/api/users/{id}/unlock` | `account.manage` | Clears any active lockout and the failed-attempt counter for the target user | `{id}` |


## Request bodies

**`POST` `/api/elevations`**: `permissionKey`, `environmentId`?, `resourceType`?, `resourceId`?, `justification`, `requestedMinutes`

**`PUT` `/api/elevations/elevatable`**: `key`, `elevatable`. An unknown key is `400`.

Every request row carries `permissionLabel`, the permission's description from the host or plugin catalog.

**`POST` `/api/elevations/{id}/decide`**: `approve`, `note`

**`POST` `/api/groups`**: `name`, `description`

**`PUT` `/api/groups/{id}`**: `name`?, `description`?

**`POST` `/api/groups/{id}/members`**: `userId`

**`POST` `/api/groups/{id}/roles`**: `roleId`, `environmentId`?, `stackName`?

**`POST` `/api/users`**: `username`, `password`, `displayName`?, `email`?, `roleId`?, `isActive`?

**`POST` `/api/users/bulk/account`**: `action`, `userIds`, `dryRun`, `reason`

**`POST` `/api/users/bulk/delete`**: `action`, `userIds`, `dryRun`, `reason`

**`PUT` `/api/users/{id}`**: `displayName`?, `email`?, `role`?, `isActive`?

**`POST` `/api/users/{id}/disable`**: `reason`

**`PUT` `/api/users/{id}/overrides`**: `overrides`

**`PUT` `/api/users/{id}/password`**: `currentPassword`, `newPassword`

**`POST` `/api/users/{id}/password/admin`**: `newPassword`

**`POST` `/api/users/{id}/roles`**: `roleId`, `environmentId`?, `stackName`?

## Related

- [Roles](./roles.md): what a role assignment grants
- [Identity](./identity.md): accounts provisioned from a directory
- [Permissions](../conventions/permissions.md): how a grant resolves
