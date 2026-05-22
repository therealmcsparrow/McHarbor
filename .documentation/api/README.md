# McHarbor API Documentation

This folder contains the repository Markdown reference for the McHarbor API.
It mirrors the public static API reference in `website/api/index.html`, but is
organized for direct reading inside the repo.

## Scope

- Base path: `/api`
- Authentication: session cookie or Bearer API key
- Response formats: JSON, Server-Sent Events (SSE), and WebSocket
- Environment selection: many workload routes use `?env=<environmentId>`

## Authentication

Protected routes accept one of these approaches:

1. Session auth via the `mcharbor_session` cookie.
2. API key auth via `Authorization: Bearer mch_...`.

Remote agents use a separate token flow:

- `GET /api/agent/ws?token=...`

## Common Conventions

### Response envelope

Typical JSON responses use a shared wrapper:

```json
{
  "success": true,
  "data": {},
  "message": "Optional translated message",
  "code": "optional_machine_code"
}
```

### Pagination

Paginated endpoints use:

- `page`
- `per_page`

### Environment selection

Docker, agent, and Kubernetes routes frequently read:

- `?env=<environmentId>`

### Auth bootstrap examples

Log in:

```bash
curl -X POST http://localhost:8705/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "changeme"
  }'
```

List environments with an API key:

```bash
curl http://localhost:8705/api/environments/ \
  -H "Authorization: Bearer mch_your_api_key"
```

## Documents

- [System and Access](./system-and-access.md)
- [Environments and Agents](./environments-and-agents.md)
- [Docker and Stacks](./docker-and-stacks.md)
- [Kubernetes](./kubernetes.md)
- [Automation and Delivery](./automation-and-delivery.md)
- [Governance and Operations](./governance-and-operations.md)
- [Realtime and Streams](./realtime-and-streams.md)

## Related Endpoints

- Human-readable website reference: `website/api/index.html`
- Machine-readable authenticated docs: `GET /api/docs/`
