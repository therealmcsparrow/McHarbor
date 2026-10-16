# Troubleshooting

## The scrape returns JSON instead of metrics

The body is the standard error envelope, and the `code` names the cause.

| Status | `code` | Cause | Fix |
|---|---|---|---|
| 401 | `err.auth.required` | No `Authorization` header reached McHarbor | Check `credentials_file` is readable by Prometheus, and that a proxy in front of McHarbor passes the header through |
| 401 | `err.api_key.invalid` | The token is unknown, or lacks the `mch_` prefix | Recreate the key; the plaintext is shown only once |
| 401 | `err.api_key.revoked` / `err.api_key.expired` | The key was revoked or passed `expiresAt` | Create a new key |
| 403 | `err.rbac.permission_denied` | The key, or its owner, does not hold `metrics.scrape` | Grant the key `metrics.scrape`, and check its owner still holds it; a key never has more than its owner |

Each refused scrape also increments `mcharbor_permission_denials_total{permission="metrics.scrape"}`
once the scrape works, which makes an intermittent 403 from a second Prometheus easy to find.

## Prometheus reports an x509 error

The target serves McHarbor's self-signed chain. Give Prometheus the root from
**Security → HTTPS** as `tls_config.ca_file`. Do not use `insecure_skip_verify` outside a
lab: the scrape carries a bearer token.

## Dashboard panels are empty

- **Every panel empty.** The `job` variable lists jobs that report `mcharbor_build_info`.
  If it is empty, Prometheus has no successful scrape yet; check **Status → Targets**.
- **Agent panels empty.** They fill only when a remote agent environment exists and has
  carried traffic. Local and TCP environments do not go through the agent transport.
- **Background panels empty.** Workflow, backup and schedule series appear the first time
  that work happens after a restart. Counters start at zero on every restart; use `rate`
  or `increase`, never the raw value.
- **Metrics collector and audit panels empty on one node.** With several nodes, only the
  leader runs them.

## An agent is "running" on its host but nothing works

The keepalive and the request path are measured separately, which is what tells a slow
agent from a wedged one:

| Signal | Slow agent | Wedged agent |
|---|---|---|
| `mcharbor_agents{state="stale"}` | 0 | rises after two ping intervals |
| `mcharbor_agent_requests_total{outcome="timeout"}` | occasional | every request |
| Seconds since last completed request | small | climbing steadily |
| `mcharbor_agent_request_duration_seconds` | high percentiles | no new observations |

A wedged agent still shows `active (running)` on its host. Restart it there; the
**Agents** section of the documentation covers the procedure.

## Every plugin fails at once

`mcharbor_plugins{state="failed"}` jumping to the total count in one scrape, with
`mcharbor_plugin_load_failures_total` rising for every slug, is the signature of a McHarbor
build whose shared packages no longer match the installed plugins. The container log names
the package in each `plugin runtime: load failed` line. Rebuild and update the affected
plugins.

## Reconciler lag keeps rising

`mcharbor_reconciler_lag_seconds` is the age of the least recently checked desired state.

| What the other gauges show | Cause |
|---|---|
| `mcharbor_reconciler_desired_states{state="unknown"}` above zero | An environment is unreachable. Its rows are left untouched rather than reported as drifted, so they age. Fix the agent or the daemon |
| No new `mcharbor_reconciler_pass_duration_seconds` observations | The loop is not running: `MCHARBOR_RECONCILE_INTERVAL=0`, or this node is not the leader |
| Passes with `outcome="failed"` | The desired states could not be read; the container log has `reconciler loop: listing desired states failed` |

## Agent spans are missing from a trace

The McHarbor spans arrive but `mcharbor-agent` ones do not:

- The agent predates agent spans. Update it.
- The trace was not sampled. Agents only measure sampled requests.
- `mcharbor_agent_spans_relayed_total{outcome="dropped"}` or `{outcome="failed"}` is
  rising: the collector is refusing or too slow.

## Tracing is on but the collector receives nothing

The exporter speaks OTLP over **HTTP**. Point it at port 4318, not the gRPC port 4317.
Look for `OpenTelemetry tracing enabled` in the log; `OpenTelemetry tracing not started`
means the exporter could not be built and the log line gives the reason.
