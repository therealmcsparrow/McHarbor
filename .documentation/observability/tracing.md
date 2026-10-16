# Tracing

McHarbor exports OpenTelemetry traces over OTLP/HTTP. It is **off by default**: with no
endpoint configured, no tracer provider is installed and no span is created.

## Turning it on

Set the standard OpenTelemetry variables on the McHarbor container and recreate it:

| Variable | Example | Meaning |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://otel-collector:4318` | Base URL of an OTLP/HTTP receiver. Setting this, or the next one, turns tracing on |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | `http://otel-collector:4318/v1/traces` | Traces-only URL; wins over the base URL |
| `OTEL_EXPORTER_OTLP_HEADERS` | `authorization=Bearer abc` | Headers for the collector, if it needs them |
| `OTEL_SERVICE_NAME` | `mcharbor-prod` | Overrides the default service name `mcharbor` |
| `OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG` | `parentbased_traceidratio` / `0.1` | Sampling. The default samples every trace |

The exporter is OTLP over HTTP (port 4318 on a stock collector), not gRPC. On startup the
log says `OpenTelemetry tracing enabled`; if the exporter cannot be built it says
`OpenTelemetry tracing not started` with the reason, and McHarbor runs without traces.

Spans are batched and exported in the background. A collector that is down loses spans; it
never slows or fails a request.

## What a trace contains

| Span | Kind | Name | Notable attributes |
|---|---|---|---|
| Each HTTP request | server | `GET /api/containers/{id}`: method and route pattern | `http.route`, `http.response.status_code`; status `Error` on a 5xx |
| Each Docker API call | client | set by the Docker SDK | its own HTTP attributes |
| Each request proxied through a remote agent | client | `agent GET` | `mcharbor.environment.id`, `url.path` |

An incoming `traceparent` header is honoured, so a trace started by a reverse proxy or a
script continues through McHarbor.

## Across the agent hop

A request to a remote environment travels browser → McHarbor → WebSocket → agent → Docker
daemon. The agent span's `traceparent` is written into the request headers carried inside
the WebSocket envelope, and the agent copies those headers onto the request it makes to
its local Docker daemon. A Docker Engine with tracing enabled therefore continues the same
trace.

The agent measures two spans of its own for every traced request:

| Span | Kind | Covers |
|---|---|---|
| `agent.proxy GET` | server | From the moment the agent received the request to the moment it finished sending the response back, streams included |
| `docker GET` | client | The agent's call to its local Docker daemon, to the response head. Carries the daemon's status code |

The agent does **not** export them itself: it usually sits behind NAT and cannot reach your
collector. It sends them back over its existing WebSocket and McHarbor forwards them to the
same OTLP endpoint, under the service name `mcharbor-agent` with the environment id as a
resource attribute. Nothing needs configuring on the agent host.

So one slow page reads as a chain: the McHarbor request span, the `agent GET` span for the
WebSocket hop, the agent's `agent.proxy` span, and the `docker` span for the daemon. A long
`agent GET` over a short `agent.proxy` is the network; a long `docker` span is the daemon.

Things to know:

- The agent's timestamps are its own host's clock. A host with a skewed clock draws its
  spans shifted against McHarbor's; the durations are still right.
- Only requests that arrive with a **sampled** trace context are measured, so the agent does
  no tracing work at all while tracing is off, and follows your sampler when it is on.
- Agent spans need an agent built from this release or later. An older agent keeps working
  and simply sends none.
- Spans wait in a bounded queue for upload. When the collector is slow or down they are
  dropped and counted in `mcharbor_agent_spans_relayed_total{outcome="dropped"}`; the
  agent and its requests are never held up.
