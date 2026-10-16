# Effective access

**Security → Management → Effective access**

Pick a user, an environment (or **Global (no environment)**) and optionally a stack, and see
every permission the user holds there and where each one comes from. The same tab exports the
permission catalog for a security review.

## Permissions

| Key | What it unlocks |
|---|---|
| `permissions.view` | The tab, the effective-permission lookup, and both catalog exports |

## Scope: environment and stack

Roles are assigned globally, per environment, or per stack inside an environment, directly to a
user or to a group the user belongs to.

- **Global** shows what a user holds with no environment: global role assignments only.
- **An environment** adds the assignments made for that environment.
- **A stack** adds the stack-scoped assignments for that stack on top. The stack list offers
  the stacks the user has stack-scoped grants for in the chosen environment.

## Reading the table

| Column | Meaning |
|---|---|
| Permission | The catalog key, including keys installed plugins declare |
| Held | What the route gates enforce: roles, approved elevations and an open break-glass session |
| With overrides | The verdict after per-user grant/deny overrides, as the policy engine computes it |
| Granted by | Every source, e.g. "Role Operator via group Ops, in Production", "Elevation until …", "Deny override, global" |

The two columns can differ because route gates do not read per-user overrides today; overrides
are only consulted by the policy engine (`Authorize`). An override bound to one resource is
shown as "only for &lt;resource&gt;" and changes neither column.

Banners call out a **wildcard** role (every permission, including ones added later), an open
**break-glass** session, and attribute (ABAC) policies being enforced, which can still deny a
request the roles allow.

By default only held keys and keys with a source are listed; **Show permissions the user does not
hold** lists the whole catalog. The filter narrows by key.

The lookup is `GET /api/access-review/effective-permissions?userId=&environmentId=&stack=`. It
resolves through the same functions the permission gates call (`core/rbac`), so the viewer and
the gates cannot disagree.

## Exporting the catalog

**Export catalog (CSV)** and **Export catalog (JSON)** download every permission key (host and
plugin) with its category, action, description, scope, origin (`host` or `plugin:<slug>`),
whether it can be requested as an elevation, and the roles that hold it. A role holding it only
through `*` is marked `(*)` in the CSV and `wildcard: true` in the JSON; the JSON also lists each
role with its key count.

`GET /api/access-review/permission-catalog/export?format=csv|json`. Cells that begin with
`=`, `+`, `-`, `@`, tab or carriage return are prefixed with `'` so a spreadsheet does not run
them as formulas. Every export is written to the audit log as `permission_catalog.exported`.

## Related

- [Security](../README.md)
- [Identity provider setup](../identity-setup/README.md)
- [Audit](../../audit/README.md)
