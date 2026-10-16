# Parameters

Four families of parameter recur across the API. The endpoint pages list what each route
takes; this page says what they mean.

## `?env=`: which environment

McHarbor manages more than one Docker or Kubernetes host. Any endpoint that talks to a
daemon takes `?env=<environmentId>` to say which one.

```
GET /api/containers?env=cq8f3k1s0abc
```

Omitting it uses the **default environment**, the one flagged `isDefault`. That makes
`GET /api/containers` a valid request on a single-host install and a dangerous habit on a
multi-host one, because the answer changes when an operator moves the default flag.

`?env=` is also the scope the permission check uses. A user granted `containers.view` on one
environment gets `403` on another, on exactly the same path.

Environment ids are [xid](https://github.com/rs/xid) values: 20 characters, sortable, safe
in a URL.

## Pagination

List endpoints that paginate accept:

| Parameter | Default | Bounds |
|---|---|---|
| `page` | `1` | 1-based; a non-numeric or zero value falls back to 1 |
| `per_page` | `25` | 1 to 1000. A value above 1000 is **ignored**, not rejected; you get 25 |

The response wraps the rows:

```json
{ "items": [], "total": 1043, "page": 1, "per_page": 25, "total_pages": 42 }
```

Endpoints serving very large result sets (audit entries, container events, backup log lines)
are normally called with `per_page` between 50 and 500, paired with lazy loading on the
client.

Not every list paginates. Small bounded collections (environments, roles, widget definitions,
scanner instances) return a plain array in `data`, and unpaginated selects carry a 1000-row
ceiling in the query.

Some endpoints take `?limit=` instead of `page`/`per_page`. Those are "most recent N"
endpoints (exec history, SCIM sync history, recent snapshots, lifecycle recents), where
paging backwards through history is not the access pattern.

## Filtering

Filters are per-endpoint, listed in each route's Parameters column. Four patterns recur:

### Time windows

`?from=` and `?to=` take **RFC 3339** timestamps and are inclusive at both ends. The
container health summary additionally accepts `?days=N` as shorthand for a window ending now.

### Free-text search

`?search=` with `?search_mode=`: `contains` (the default) or `exact`. Regular expressions
are deliberately not supported server-side; the audit and activity screens apply those
client-side, against the page they already fetched.

### Exact-match filters

`?action=`, `?entity_type=`, `?entity_id=`, `?status=`, `?severity=`, `?state=`,
`?category=`, `?module=`. All exact, all optional. Where a filter is repeatable (audit
`action`), repeating the parameter widens the match.

### Scoping to a parent

`?containerId=`, `?pipelineId=`, `?workflow_id=`, `?alertId=`, `?planId=`. These are
optional on purpose: the backup run list without `containerId` is the whole-estate view the
Backups overview renders, and with it is the per-container tab.

## Path parameters

Written in `{braces}` in the reference. The common ones:

| Parameter | Identifies |
|---|---|
| `{id}` | An xid for a McHarbor row, or a Docker id where the resource is Docker's |
| `{name}` | A Docker volume name, or a stack name; stacks are addressed by name, not id |
| `{key}` | A settings key, a widget key, a custom-node key, a workflow-node key |
| `{slug}` | A plugin or catalog-app slug |
| `{runId}`, `{planId}`, `{transferId}`, `{sessionId}`, `{assignmentId}` | Exactly what they say |

A container id may be given in full or in Docker's short form wherever the Docker daemon
itself accepts both.

## Booleans

Query booleans are read as the string `true`. `?force=true`, `?all=true`, `?vacuum=true`,
`?stream=false`. Anything else counts as false; there is no `1`/`yes` handling.

Body booleans are real JSON booleans.

## Destructive confirmations

Some mutations require an explicit acknowledgement in the body rather than in a query
parameter, so that a copied `curl` line cannot do damage on its own:

| Endpoint | Requires |
|---|---|
| `POST /api/metrics/host/prune` | `confirm: true`, plus `volumes: true` to include named volumes |
| `POST /api/system/reset` | `confirm` matching a specific double-confirmation string |
| `POST /api/system/os-updates/apply` | `confirm: true` |

## Optional body fields

In the request-body lists on each endpoint page, a `?` suffix marks a field that is
`omitempty` or a pointer in the Go type. On a create that means "use the default"; on an
update it means **"leave unchanged"**, which is why update payloads are pointer-typed
throughout. Sending `null` and omitting the field are the same thing; sending an empty string
is a value.

## The `vacuum` parameter

The four purge endpoints (`DELETE /api/audit`, `/api/activity`, `/api/lifecycle`,
`/api/scans`, `/api/backup-logs`) accept `?vacuum=true`. Without it, SQLite marks the freed
pages as reusable and the database file stays the same size. With it, the handler also runs a
WAL checkpoint and a background `VACUUM`, so the space goes back to the operating system.

## Related

- [Conventions](./README.md): the envelope and limits these parameters live inside
- [Errors](./errors.md): what a rejected parameter returns
