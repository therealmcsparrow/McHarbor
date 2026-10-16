# Conventions

Everything on this page is true of every endpoint. The reference pages assume it and do not
repeat it.

## Base path

All API routes live under `/api`, mounted on the same chi router that serves the built
frontend. The backend listens on **8705** inside the container; `docker-compose.yml`
publishes that same port on the host. In development, Vite serves the UI on **8173** and
proxies `/api` through to 8705.

Nothing is versioned in the path. `/api/scim/v2/*` is the one exception, and that `v2` is
SCIM's own version, not McHarbor's.

## The response envelope

Every JSON response uses one shape, defined in `core/response/response.go`:

```json
{
  "success": true,
  "data": {},
  "error": "",
  "message": "",
  "code": ""
}
```

| Field | When it appears | Meaning |
|---|---|---|
| `success` | always | `true` for 2xx, `false` otherwise |
| `data` | on success, and on some failures | The payload. Omitted when empty |
| `error` | on failure | The message, already translated into the request's language |
| `message` | occasionally | A human sentence alongside a success |
| `code` | on failure, and on confirmation successes | The stable machine-readable identifier. **This is what you branch on**, never `error` |

`error` is translated and may change wording between releases and between locales. `code`
does not. See [Errors](./errors.md) for the full list.

A failure can still carry `data`: a stack deploy that rejected four services by name returns
the code *and* the list, so the client can name them rather than showing "deploy failed" over
an empty panel.

### Paginated responses

List endpoints that paginate wrap their payload:

```json
{
  "success": true,
  "data": {
    "items": [],
    "total": 1043,
    "page": 1,
    "per_page": 25,
    "total_pages": 42
  }
}
```

### Empty successes

`204 No Content` is sent with no body at all. A `200` carrying only a confirmation sends
`{ "success": true, "data": { "message": "…" }, "code": "msg.…" }`.

## Status codes

| Code | Used for |
|---|---|
| `200 OK` | Read succeeded, or a mutation that returns the updated resource |
| `201 Created` | A resource was created |
| `204 No Content` | Succeeded, nothing to return |
| `400 Bad Request` | Malformed body, missing required field, value out of range |
| `401 Unauthorized` | No session, expired session, invalid / expired / revoked API key |
| `403 Forbidden` | Authenticated, but the permission check failed |
| `404 Not Found` | No such row. Also returned instead of 403 where existence itself is sensitive |
| `409 Conflict` | Name already taken, resource in use, state transition not allowed |
| `500 Internal Server Error` | Something broke. The real error is in the server log, never in the response |
| `503 Service Unavailable` | A capability is not wired in this build (plugin reload, backup migration) |

A **failed test is a successful request**. `POST /api/storage-locations/{id}/test`,
`/api/communication-channels/{id}/test`, `/api/ai-providers/{id}/test` and
`/api/identity-providers/{id}/test` all return `200` with a per-step report whose overall
status is `fail`. Only a missing row (404) or an internal fault (500) is a non-200, because
the client needs to render *which step* broke, and an HTTP error would collapse that into
one toast.

## Errors never leak internals

A handler that hits a Docker error, a SQL error or a filesystem error logs it server-side
with context and returns a generic i18n code. Raw Go error text is never sent to a client.
This is deliberate: raw errors carry socket paths, file paths and host names.

## Request headers

| Header | Effect |
|---|---|
| `Content-Type: application/json` | Required on any request with a JSON body |
| `Authorization: Bearer mch_…` | Authenticates as an API key. Checked before the session cookie |
| `Cookie: mcharbor_session=…` | Authenticates as a browser session |
| `Accept-Language` | Picks the language of `error` and `message`. Falls back to `en` |
| `X-Requested-With` | Allowed by CORS; not required |

`Accept-Language` is handled by `i18n.Middleware` on every `/api` request, so the same code
comes back in Dutch, German, Spanish, French or Portuguese without any per-endpoint work.

## Response headers

Security headers are set globally by `middleware.SecurityHeaders`, including HSTS when
`HSTS_MAX_AGE` is configured. Static assets under `/assets/` are pinned for a year
(content-hashed by Vite); everything else is `Cache-Control: no-cache` and revalidated.

## Limits

| Limit | Value | Where |
|---|---|---|
| JSON request body | **8 MiB** | `MaxJSONBodySize` on the whole `/api` tree |
| Auth endpoint rate limit | **10 requests / minute** | The auth group only: login, setup, password reset, SSO callbacks |
| Default page size | **25** | `ParsePagination` |
| Maximum page size | **1000** | `ParsePagination`. A larger value is ignored, not rejected |
| Unpaginated list ceiling | **1000 rows** | Convention enforced in the services |
| Docker / Kubernetes SDK call | **30 s** | Every SDK call takes a bounded context |

Multipart uploads (container file upload, image import, backup restore upload) are not bound
by the 8 MiB limit; that middleware applies to JSON bodies.

## CORS

`ALLOWED_ORIGINS` sets the allow-list; with nothing configured the app's own dev and
production origins are used. Credentials are allowed, so a wildcard origin is not possible.
Methods `GET, POST, PUT, PATCH, DELETE, OPTIONS`; headers `Accept`, `Accept-Language`,
`Authorization`, `Content-Type`, `X-Requested-With`; preflight cached for 300 seconds.

## Maintenance mode

While the database is being copied to another SQL backend, the maintenance middleware pauses
**mutating** requests across the whole `/api` tree. Reads continue. At every other moment it
is a no-op.

## Middleware order

Understanding the order explains most 401-versus-403 questions:

```
Recovery → Logger → SecurityHeaders → CORS → i18n
  └── /api
        MaxJSONBodySize(8 MiB) → Maintenance
          ├── public group     (no auth, no rate limit)
          ├── auth group       (rate limit 10/min, no auth middleware)
          └── protected group  (API key → session → usage telemetry → audit fallback)
                                  └── per-route RequirePermission
        └── plugin routes (the same three chains, chosen per route at rebuild time)
```

The API key middleware runs **first** and falls through when there is no `Bearer` header, so
a request carrying both a cookie and a key authenticates as the key.

Plugin routes are mounted as one catch-all after the host modules, so a mistyped `/api` path
answers `404` rather than `401`.

## The three route groups

| Group | Auth | Rate limit | Examples |
|---|---|---|---|
| **public** | none | none | `/api/health`, `/api/about`, `/api/scim/v2/*` (own bearer token), the agent transfer endpoints (own `Authorization` token), the GitOps PR webhook (signature-verified) |
| **auth** | none | 10/min | `/api/auth/login`, `/api/auth/setup`, `/api/auth/status`, `/api/auth/password/*`, `/api/identity-providers/{id}/authorize`, `/api/database/setup/*`, `/api/agent/ws` |
| **protected** | session or API key | none | Everything else, the large majority of the 516 routes |

"Public" never means unauthenticated in the security sense. Every public route authenticates
by some other means (a SCIM bearer token, a one-shot transfer token, an HMAC signature). It
just does not use a McHarbor session.

## Related

- [Authentication](./authentication.md): how to obtain each credential
- [Permissions](./permissions.md): what the permission column means
- [Parameters](./parameters.md): `?env=`, pagination, filters
- [Errors](./errors.md): every `code`
- [Streaming](./streaming.md): SSE and WebSocket
