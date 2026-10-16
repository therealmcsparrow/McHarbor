# Roles and policies

19 endpoints: the role catalog, and the attribute-based policies layered on top of it.

| Key | Covers |
|---|---|
| `roles.view` | List roles, read one, list presets, read the permission list |
| `roles.manage` | Create, edit and delete custom roles; compute impact |
| `rbac.policies.view` | List policies and their history |
| `rbac.policies.manage` | Create, edit, delete, enable, test policies; read and write user labels |

## Things worth knowing first

**System roles cannot be edited or deleted.** They return `err.role.system_locked`. Presets
are the starting points a new custom role is cloned from.

**`POST /api/roles/{id}/impact` answers before you commit.** Given a candidate permission
list, it reports which users would gain or lose what, so a role edit that quietly locks
forty people out of production is visible before it is saved.

**Policies are expressions, not lists.** A policy has an effect (allow or deny), a resource
type, an expression evaluated against request attributes, a priority, and the permissions it
applies to. `POST /api/rbac/policies/{id}/test` dry-runs one against an attribute bundle you
supply. This is the Test button in the editor.

**User labels are the attribute bag policies match on.**
`PUT /api/rbac/users/{userID}/labels` replaces it; passing an empty value for a key removes
that label. Both label routes are gated by `rbac.policies.manage`, not by `users.view`, so
only admins who can author policies can read the attributes those policies match.

**Policy edits are versioned.** `GET /api/rbac/policies/{id}/history` is the audit trail.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/rbac/policies` | `rbac.policies.view` | Returns all ABAC policies | — |
| `GET` | `/api/rbac/policy-reference` | `rbac.policies.view` | Returns the resource types (host and loaded plugins) with their attributes, the variables and the functions a policy expression can use | — |
| `POST` | `/api/rbac/policies` | `rbac.policies.manage` | Validates and inserts a new policy | — |
| `DELETE` | `/api/rbac/policies/{id}` | `rbac.policies.manage` | Removes a policy | `{id}` |
| `GET` | `/api/rbac/policies/{id}` | `rbac.policies.view` | Returns a single policy by id | `{id}` |
| `PUT` | `/api/rbac/policies/{id}` | `rbac.policies.manage` | Modifies an existing policy | `{id}` |
| `POST` | `/api/rbac/policies/{id}/enable` | `rbac.policies.manage` | Flips the enabled flag on a policy | `{id}` |
| `GET` | `/api/rbac/policies/{id}/history` | `rbac.policies.view` | Returns the audit trail for a policy | `{id}` |
| `POST` | `/api/rbac/policies/{id}/test` | `rbac.policies.manage` | Dry-runs a policy expression against an admin-supplied attribute bundle | `{id}` |
| `GET` | `/api/rbac/users/{userID}/labels` | `rbac.policies.manage` | Returns a user's free-form label bag | `{userID}` |
| `PUT` | `/api/rbac/users/{userID}/labels` | `rbac.policies.manage` | Replaces the user's label bag | `{userID}` |
| `GET` | `/api/roles` | `roles.view` | Returns all roles | — |
| `POST` | `/api/roles` | `roles.manage` | Creates a new custom role | — |
| `GET` | `/api/roles/permissions` | `roles.view` | List Permissions | — |
| `GET` | `/api/roles/presets` | `roles.view` | Returns every preset from the role_presets table | — |
| `DELETE` | `/api/roles/{id}` | `roles.manage` | Removes a custom role | `{id}` |
| `GET` | `/api/roles/{id}` | `roles.view` | Returns a single role | `{id}` |
| `PUT` | `/api/roles/{id}` | `roles.manage` | Updates an existing custom role | `{id}` |
| `POST` | `/api/roles/{id}/impact` | `roles.manage` | Computes the user impact of setting a role's permission list to the supplied set | `{id}` |


## Request bodies

**`POST` `/api/rbac/policies`**: `name`, `description`, `effect`, `resourceType`, `expression`, `priority`, `enabled`, `permissions`

**`PUT` `/api/rbac/policies/{id}`**: `name`, `description`, `effect`, `resourceType`, `expression`, `priority`, `enabled`, `permissions`

**`POST` `/api/rbac/policies/{id}/test`**: `attributes`?

**`PUT` `/api/rbac/users/{userID}/labels`**: `labels`

**`POST` `/api/roles`**: `name`, `description`, `permissions`

**`PUT` `/api/roles/{id}`**: `name`?, `description`?, `permissions`

**`POST` `/api/roles/{id}/impact`**: `permissionKeys`, `environmentId`?

## Related

- [Users](./users.md): assigning a role
- [Permissions](../conventions/permissions.md): the catalog and the scope model
