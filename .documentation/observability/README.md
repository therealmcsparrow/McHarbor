# Observability

McHarbor exposes its own health (the control plane, not your containers) as Prometheus
metrics and, optionally, OpenTelemetry traces. Point the Prometheus and Grafana you
already run at it, and McHarbor stops being the one unmonitored thing in your stack.

This is instrumentation for **your** monitoring stack. It is not a McHarbor dashboard:
container and host metrics shown inside McHarbor come from the separate
`/api/metrics/host`, `/api/metrics/containers/history`, `/api/metrics/containers/{id}/stream` and related routes, gated on `host.view`.

| Page | Covers |
|---|---|
| [Scrape configuration](./scrape-config.md) | Creating a scrape-only API key, the Prometheus job, TLS, and importing the Grafana dashboard |
| [Tracing](./tracing.md) | Turning on OTLP trace export, what a span covers, and how a trace crosses the agent hop |
| [Capacity forecasting](./capacity/README.md) | When each filesystem and storage destination runs out of space, when the forecast refuses to answer, the idle-resource list, and the trend alert condition |
| [Troubleshooting](./troubleshooting.md) | 401 and 403 from the scrape, empty panels, a wedged agent, a mass plugin failure |

## The endpoint

| | |
|---|---|
| Route | `GET /api/metrics` |
| Permission | `metrics.scrape`, scope `global` |
| Format | Prometheus text exposition (`text/plain; version=0.0.4`), **not** the JSON envelope |
| Authentication | An API key (`Authorization: Bearer mch_…`) or a session cookie |
| Cost | Every value is read from memory. A scrape makes no Docker call, no agent round trip and no database query |

The endpoint is never open. Without a principal it answers `401`; with one that lacks
`metrics.scrape` it answers `403`. The exposition names environments by id, counts
agents and plugins, and lists every route pattern. It describes the shape of your
fleet, so it is gated like anything else that does.

`metrics.scrape` is a key of its own rather than a reuse of `host.view`, so a scrape key
can read the control plane's health without being able to read a single container.

## Metric families

Every McHarbor family is prefixed `mcharbor_`. Label values are bounded: no series is
ever labelled by container id, image digest, user id or raw URL path. An environment id
is used as a label because the number of environments is small and fixed by the operator.

### HTTP and API

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `mcharbor_http_requests_total` | counter | `route`, `method`, `code` | Requests by chi route pattern (`/api/containers/{id}`, never the raw path) and status class (`2xx`…`5xx`). Paths no route claims are counted under `route="unmatched"` |
| `mcharbor_http_request_duration_seconds` | histogram | `route`, `method` | Latency. WebSocket upgrades and `text/event-stream` responses are excluded, because their duration is the life of the connection |
| `mcharbor_http_requests_in_flight` | gauge | — | Requests being served now, streams included |
| `mcharbor_permission_denials_total` | counter | `permission` | Requests a permission gate refused, by the key that was missing. Routes that accept any of several keys report them joined with `\|` |

### Agent transport

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `mcharbor_agents` | gauge | `state` | Pooled agent connections: `live`, `measuring` (connected, no keepalive answered yet), `stale` (no pong for more than two ping intervals) |
| `mcharbor_agent_request_duration_seconds` | histogram | `environment` | Round trip of a proxied Docker API request to the response head |
| `mcharbor_agent_requests_total` | counter | `environment`, `outcome` | `ok`, `timeout`, `cancelled`, `closed`, `send_error`, `error` |
| `mcharbor_agent_requests_in_flight` | gauge | `environment` | Proxied requests awaiting a response head |
| `mcharbor_agent_last_response_timestamp_seconds` | gauge | `environment` | When the agent last completed a proxied request |
| `mcharbor_agent_keepalive_latency_seconds` | histogram | `environment` | Ping-to-pong round trip |
| `mcharbor_agent_connects_total` / `_reconnects_total` / `_disconnects_total` | counter | `environment` | Pool membership changes. A reconnect is a connect for an environment that already connected since McHarbor started |
| `mcharbor_agent_handshakes_rejected_total` | counter | `stage` | `token`, `upgrade`, `auth`, `protocol`, `duplicate` |
| `mcharbor_agent_dropped_messages_total` | counter | `environment`, `reason` | `disconnect` (pending requests failed when the socket closed), `orphan_response`, `stream_overflow`, `invalid_message` |
| `mcharbor_agent_rpc_duration_seconds` | histogram | `environment`, `op` | Every agent call that is not a proxied Docker request: `compose`, `host_stats`, `prune`, `agent_update`, `host_logs`, `host_os_info`, `host_update_check`, `host_update_apply`, the `transfer_*` family, backup and restore, and every compute and snapshot message type. A call that returns a stream is timed to the stream opening |
| `mcharbor_agent_rpc_total` | counter | `environment`, `op`, `outcome` | `ok`, `timeout`, `cancelled`, `error` |
| `mcharbor_agent_spans_relayed_total` | counter | `outcome` | Spans agents measured and the server forwarded to the collector: `exported`, `failed`, `dropped`. See [Tracing](./tracing.md) |
| `mcharbor_agent_offline_batches_total` | counter | `kind`, `outcome` | Offline replay batches, by `logs` / `events` / `metrics` and `accepted` / `failed` |
| `mcharbor_agent_offline_rows_total` | counter | `kind` | Rows ingested from replay |
| `mcharbor_agent_offline_buffer_remaining` | gauge | `environment`, `kind` | Rows the agent said were still buffered after its last batch |
| `mcharbor_agent_offline_replay_lag_seconds` | histogram | `kind` | Age of the oldest row in a batch when it was ingested |

### Background work

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `mcharbor_workflow_runs_total` | counter | `trigger`, `outcome` | Finished workflow runs; `trigger` is `manual`, `auto`, `link` or `webhook`, `outcome` is `completed`, `failed` or `cancelled` |
| `mcharbor_workflow_run_duration_seconds` | histogram | `outcome` | Run duration |
| `mcharbor_schedule_fires_total` | counter | `scheduler` | `workflow` (cron triggers) and `container_backup` (backup plans) |
| `mcharbor_schedule_lateness_seconds` | histogram | `scheduler` | Delay from the schedule's minute to the moment it fired |
| `mcharbor_backup_job_duration_seconds` | histogram | `operation`, `outcome` | Container `backup` / `restore` runs, `success` / `failure` |
| `mcharbor_metrics_collector_sweep_duration_seconds` | histogram | — | One pass of the background container and host metrics collector |
| `mcharbor_audit_chain_verify_ok` | gauge | — | `1` when the last hourly audit hash-chain verification was clean, `0` when it failed or warned |
| `mcharbor_audit_chain_verify_timestamp_seconds` / `_checked_rows` | gauge | — | When it last ran, and how many rows it checked |
| `mcharbor_queue_depth` / `mcharbor_queue_capacity` | gauge | `queue` | `api_key_usage` and `registry_meta_refresh` |
| `mcharbor_reconciler_lag_seconds` | gauge | — | Age of the least recently checked desired state. It is computed at scrape time, so it keeps rising if the drift loop stalls, and an environment that stays unreachable holds it up. `0` on a node that is not the leader, and when there are no desired states |
| `mcharbor_reconciler_desired_states` | gauge | `state` | What the last pass found: `in_sync`, `drifted`, `pending` (drift seen once, not yet confirmed) or `unknown` (environment unreachable) |
| `mcharbor_reconciler_pass_duration_seconds` | histogram | `outcome` | One pass of the drift loop; `failed` when the desired states could not be listed |
| `mcharbor_reconciler_drift_transitions_total` | counter | `direction` | A desired state's recorded drift flag flipping: `detected` or `resolved` |

### Runtime and storage

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `go_*` | various | — | Go runtime: goroutines, heap, GC pauses |
| `go_sql_*` | various | `db_name` | Database pool saturation. `db_name` is the backend: `sqlite`, `postgres` or `mysql` |
| `mcharbor_db_query_duration_seconds` | histogram | `backend`, `operation` | Every statement McHarbor and its add-ons run, by backend and `exec` / `query`. A query is timed to its first result set, not to the last row read. The statement text is never a label |
| `mcharbor_db_query_errors_total` | counter | `backend`, `operation` | Statements that returned an error |
| `mcharbor_plugins` | gauge | `state` | Plugins `loaded` and `failed` |
| `mcharbor_plugin_load_failures_total` | counter | `plugin` | Failed load attempts, by plugin slug |
| `mcharbor_build_info` | gauge | `version`, `build` | Always `1` |

Process metrics (`process_*`) are deliberately absent: they are read from `/proc` on every
scrape, and the scrape reads memory only.

## What is measured where

- **Docker API calls through an agent** are in `mcharbor_agent_request_*`; every other
  agent call is in `mcharbor_agent_rpc_*`. Terminal and console streams are in neither:
  they are sessions, not calls.
- **Query timing** stops at the first result set. A query that returns quickly and is then
  read slowly shows up as a long HTTP request, not a long query.
