# Audit: RBAC denials

`/audit?view=denials`

Every request McHarbor refused because the caller lacked a permission. It answers "who
tried to do something they may not, and where" without reading raw `permission.denied`
rows.

## What is recorded

Two audit actions feed the tab:

| Action | Written when | Source badge |
|---|---|---|
| `permission.denied` | A route's `RequirePermission` middleware refuses a role-based check | **Role** |
| `abac.denied` | An ABAC policy denies a request | **Policy** |

Repeats of the same user, permission and route within one minute are recorded **once**, so
a page that polls a route the user cannot use does not flood the trail. A denial that is
missing from the tab while the user saw several 403s is this throttle, not a gap.

## Columns

| Column | What it is |
|---|---|
| Time | When the request was refused |
| Actor | The user, linked to their page under Security → Users (`/security/users/<id>`). An ABAC row stores only the user id; the name is looked up when you read |
| Permission | The permission key that was missing, e.g. `stacks.manage` |
| Route | The request path for a role denial; **Policy &lt;id&gt;** for an ABAC denial |
| Environment | The environment the check was scoped to, or **Global** for a global permission |
| Source | **Role** or **Policy** |

## Filters

| Filter | API parameter | What it narrows |
|---|---|---|
| Date range | `from`, `to` | RFC 3339 bounds |
| Actor | `actor` | A username (case-insensitive) or user id. Matches role denials by user and ABAC denials by the `user_id` in their details |
| Permission | `permission` | The exact key: `stacks.manage` does not match `stacks.manage_all` |
| Environment | `environment` | Exactly that environment; global denials are excluded once one is picked |

## API

`GET /api/audit/denials` is paginated (`page`, `per_page`), gated by `audit.view`. Opening
the tab records one `audit.accessed` row ("listed audit denials") for the first page.

## Related

- [Audit](./README.md): the screen as a whole
- [Troubleshooting](./troubleshooting.md): denials that seem to be missing
