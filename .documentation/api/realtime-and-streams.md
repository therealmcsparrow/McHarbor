# Realtime and Streams

McHarbor uses Server-Sent Events for one-way live updates and WebSocket for
interactive terminal access and remote-agent transport.

## SSE Endpoints

| Path | Purpose |
| --- | --- |
| `/api/events/stream` | Streams environment events in real time. |
| `/api/logs/{id}` | Streams logs through the dedicated log streaming module. |
| `/api/metrics/containers/{id}/stream` | Streams per-container metrics. |
| `/api/workflows/{id}/live` | Streams workflow execution events. |
| `/api/app-store/install/stream` | Streams app installation progress. |

## WebSocket Endpoints

| Path | Purpose |
| --- | --- |
| `/api/terminal/ws` | Interactive browser terminal for container exec sessions. |
| `/api/agent/ws?token=...` | Outbound agent transport for remote Docker hosts. |

## Related Polling-Friendly Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/metrics/host` | Returns a host-level metrics snapshot. |
| GET | `/api/metrics/containers` | Returns container metrics snapshot data. |
| GET | `/api/containers/{id}/logs` | Returns container logs in normal request-response form. |

## Notes

- Workflow execution can be started with `POST /api/workflows/{id}/execute` and observed via `GET /api/workflows/{id}/live`.
- Terminal access uses WebSocket rather than SSE because it is bidirectional.
- Agent connections also use WebSocket because they multiplex request and response traffic over one outbound channel.
