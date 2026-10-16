# API

McHarbor's HTTP API is the same API the web interface uses. There is no second, reduced
"public" surface: every screen in the product is built on the endpoints documented here, so
anything the UI can do, a script can do.

- **Base path** `/api`, on the same origin as the interface.
- **Ports** the container listens on `8705`; the shipped Compose file publishes that same port.
- **Format** JSON in, JSON out, in one envelope: `{ success, data?, error?, message?, code? }`.
- **Auth** a session cookie for the browser, a `Bearer` API key for everything else.
- **Authorisation** every route names the permission key that unlocks it.

There are **516 endpoints** across 21 areas.

## Sections

| Section | Covers |
|---|---|
| [Conventions](./conventions/README.md) | The envelope, status codes, headers and limits. Read this once, then the reference makes sense |
| [Conventions → Authentication](./conventions/authentication.md) | Sessions, API keys, MFA, first-run setup, SCIM tokens, agent tokens |
| [Conventions → Permissions](./conventions/permissions.md) | How routes are gated, the scope model, the full permission catalog |
| [Conventions → Parameters](./conventions/parameters.md) | `?env=`, pagination, filtering, path parameters |
| [Conventions → Errors](./conventions/errors.md) | Every `code` value the API can return |
| [Conventions → Streaming](./conventions/streaming.md) | The SSE and WebSocket endpoints and what they emit |
| [Endpoints](./endpoints/README.md) | One page per area: every route, its permission, its parameters, its body |
| [Settings](./settings/README.md) | The settings API, and every setting key with its type, default and bounds |

## Quick start

```bash
# 1. Log in and keep the session cookie
curl -c jar.txt -X POST http://localhost:8705/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"…"}'

# 2. Mint an API key for scripting
curl -b jar.txt -X POST http://localhost:8705/api/api-keys \
  -H 'Content-Type: application/json' \
  -d '{"name":"ci","roleIds":[],"scopes":["containers.view"],"environmentIds":[]}'
# → data.key is shown once. Store it now.

# 3. Use it
curl -H 'Authorization: Bearer mch_…' \
  'http://localhost:8705/api/containers?all=true&env=<envId>'
```

## How to read the reference

Each endpoint page carries one table of every route in that area:

| Column | Means |
|---|---|
| **Method** | The HTTP verb, exactly as mounted |
| **Path** | The full path including `/api`, with chi path parameters in `{braces}` |
| **Permission** | The key `RequirePermission` checks. `—` means the route is authenticated but carries no permission middleware, which the page explains |
| **What it does** | One line, taken from the handler |
| **Parameters** | Path parameters, query parameters, and multipart fields |

Request bodies follow the table, one line per endpoint, listing the JSON field names. A `?`
suffix marks an optional field, one that is `omitempty` or a pointer in the Go type, which
means "leave unchanged" on an update and "use the default" on a create.

## Conventions used here

- **Routes are written as they are mounted.** Where the code registers `/containers/{id}`
  under a `/containers` group, the page shows `/api/containers/{id}`.
- **Permissions are named with their exact keys**: `containers.manage`, never "container
  management rights".
- **No screenshots.** This is a wire-level reference.
- **The code wins.** Where a comment in the repository and the running route table disagree,
  the page documents what is mounted and says the comment is stale.

## Also see

- [Getting started](../getting-started/README.md): installing, and every environment variable
- [Screens](../screens/README.md): what each endpoint looks like from the interface
- [Agents](../agents/README.md): the remote agent, which speaks its own WebSocket protocol
- [Plugins](../plugins/README.md): plugins mount their own routes under `/api`
