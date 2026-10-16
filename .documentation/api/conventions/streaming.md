# Streaming

Some operations have no single moment of completion worth waiting for: a log tail, a stack
deploy, a container move across hosts. Those endpoints stream. Two mechanisms are used:
Server-Sent Events for one-way progress, WebSockets for interactive sessions.

## Server-Sent Events

An SSE endpoint answers `200` with `Content-Type: text/event-stream` and then holds the
connection open, writing named frames until the work finishes or the client disconnects.

```
event: connected
data: {"status":"connected"}

event: stats
data: {"cpuPercent":3.1,"memoryUsage":184549376}
```

The first frame is always `event: connected`, so a client can distinguish "the stream is
open and nothing has happened yet" from "the request is still being set up". Some streams
also write an `id:` line, which a browser's `EventSource` replays as `Last-Event-ID` on
reconnect.

Frames are flushed individually. A stream that produces nothing for a long time is still a
healthy stream.

### The SSE endpoints

| Endpoint | Permission | Streams |
|---|---|---|
| `GET /api/events/stream` | `events.view` | Docker daemon events, frame name = the Docker event type |
| `GET /api/logs/{id}` | `logs.view` | A container's log tail, one frame per line, with `id:` for resume |
| `GET /api/metrics/containers/{id}/stream` | `host.view` | Live container stats, frame `stats` |
| `POST /api/containers/{id}/move/stream` | `containers.manage` | Progress of a cross-environment container move |
| `POST /api/app-store/install/stream` | `store_apps.manage` | App install progress |
| `POST /api/app-store/installations/{id}/uninstall/stream` | `store_apps.manage` | App uninstall progress |
| `POST /api/appstore/plugins/install/stream` | — | Plugin install progress |
| `POST /api/appstore/plugins/{slug}/update/stream` | — | Plugin update progress |
| `POST /api/catalogs/{id}/test/stream` | `settings.manage` | Per-step catalog probe. A remote fetch can take 15 s, and the point is showing which step is spending it |
| `POST /api/workflows/{id}/execute` | `workflows.run` | Workflow execution events as the run walks the graph |
| `GET /api/workflows/{id}/live` | — | Background execution events for a workflow, for a canvas that is merely watching |
| `POST /api/ai-assistant/chat` | `ai_assistant.use` | Assistant reply: `message` per text delta, `done` at the end |

Three of these use `POST` rather than `GET` because they need a request body. A browser's
`EventSource` cannot send one; the frontend uses `fetch` with a streaming reader instead.

### Consuming a stream

```bash
curl -N -H 'Authorization: Bearer mch_…' \
  'http://localhost:8705/api/events/stream?env=<envId>'
```

`-N` disables curl's buffering. Without it, nothing appears until the stream ends.

A proxy in front of McHarbor must not buffer these responses. Nginx needs
`proxy_buffering off` on the `/api` location for SSE to work at all.

## WebSockets

Four endpoints upgrade to a WebSocket, all of them interactive sessions where both sides
talk.

| Endpoint | Permission | Session |
|---|---|---|
| `GET /api/terminal/ws` | `terminal.access` | A shell inside a container, via a Docker exec |
| `GET /api/system/os-terminal/ws` | `terminal.access` | A shell on the host OS |
| `GET /api/system/host-terminal/ws` | `terminal.access` | The same handler, mounted at a second path |
| `GET /api/compute/console/{ticket}` | — (ticketed) | A VM/LXC console bridged to the guest |
| `GET /api/agent/ws` | — (token) | The remote agent's own transport, not a client API |

### Terminal

`GET /api/terminal/ws?container=<id>&shell=/bin/sh&user=root&workDir=/app`. Only `container`
is required; `shell` is normally chosen by first calling
`POST /api/containers/{id}/shells`, which probes what the image actually has.

Messages are JSON objects carrying a type:

| Type | Direction | Payload |
|---|---|---|
| `data` | both | Terminal bytes: keystrokes in, output out |
| `resize` | client → server | New `cols` and `rows` |
| `error` | server → client | A failure, already translated |

A container marked protected refuses the session with `err.protected_target`.

### Console tickets

The console is the one case where the permission check and the socket are separate requests.
`POST /api/compute/console/ticket` is an ordinary authenticated request and is where
authorisation happens; it returns a ticket. `GET /api/compute/console/{ticket}` then upgrades
with nothing but that ticket, because a browser cannot set headers on a WebSocket handshake.
Tickets are single-use and short-lived, and the endpoint caps concurrent viewers
(`err.console.too_many_viewers`).

### The agent transport

`GET /api/agent/ws?token=…` is the remote agent connecting inbound-free: the agent dials
out, so no Docker socket is ever exposed. It is documented in
[Agents → Protocol](../../agents/protocol/README.md), not here, because it is a
machine-to-machine protocol rather than an API surface.

Two constraints are worth knowing because they are easy to regress:

- Docker's non-detached exec bypasses the agent transport entirely. Terminal exec therefore
  uses dedicated `exec_start` / `exec_input` / `exec_output` / `exec_end` / `exec_resize`
  WebSocket messages, and shell detection uses a detached exec. This needs agent v1.1.0 or
  newer.
- A one-shot stats call costs roughly two seconds through the agent. Anything that needs
  stats for many containers must fan out concurrently. A current agent avoids the cost by
  pushing its stats instead (`stats_subscribe` / `stats_push` / `stats_push_ack`), and every
  stats reader shares one sample per environment; see
  [Container stats](../../agents/stats/README.md).
- `GET /api/events/stream` shares one Docker events subscription per environment and type
  filter among every open client. A client that falls far behind is disconnected and should
  reconnect, which an `EventSource` does on its own.

## Long-running work that does not stream

Not everything slow is a stream. Several operations return immediately and are polled:

| Operation | Poll |
|---|---|
| A backup or restore run | `GET /api/container-backups/runs/{runId}` |
| A retried destination upload | The same run endpoint; watch the destination's byte counters |
| A SCIM sync triggered with `sync-now` | `GET /api/scim/endpoints/{id}/history` |
| A self-update or restart | The process goes away; reconnect |

The restore dialog polls the run endpoint, which is why that route is a plain read with no
audit entry and the same `backups.view` permission as the list.

## Related

- [Conventions](./README.md): status codes and the envelope non-streaming endpoints use
- [Agents](../../agents/README.md): the agent transport in full
