# How the Agent Connects

One WebSocket, opened by the agent, carrying every message family below. This page is the
technical view: useful when something behaves oddly, and required reading before changing
anything on either side of it.

`src/agent/protocol.go` and `src/backend/core/agent/protocol.go` must stay identical. The
agent is a separate Go module and cannot import the host's, so the constants are duplicated
and kept in step deliberately.

## The connection

1. The agent dials `MCHARBOR_URL`, promoted to `wss://` when `MCHARBOR_FORCE_TLS` is set.
2. It sends **`auth`**: token, hostname, Docker version, and `tlsMode`: how the agent
   secured its own side of that dial (`pinned`, `verified`, `insecure` when
   `MCHARBOR_INSECURE` is set, or `plaintext` when the URL is `ws://`). Only the agent can
   know it, since McHarbor sees a connection a proxy may already have terminated. An agent
   too old to report one leaves McHarbor with the request scheme alone, which it records as
   `encrypted`: encryption proven, verification unknown. It surfaces as the shield on the
   Environments table and cards.
3. McHarbor answers **`auth_result`**, accepting with the environment id or rejecting.
4. **`ping`** / **`pong`** run in both directions on the interval set in
   [Settings → System → Agent](../../settings/system/agent/README.md).
5. On disconnect the agent marks itself offline, starts buffering locally, and reconnects
   with exponential backoff, from **1 second up to 60 seconds**, jittered.

Everything below rides that one connection.

## Docker, proxied

| Message | Direction | Purpose |
|---|---|---|
| `http_request` | Server → Agent | A proxied Docker API call |
| `http_request_start` / `_chunk` / `_end` | Server → Agent | The same, with a streamed request body |
| `http_response` | Agent → Server | A complete response |
| `http_response_start` / `_chunk` / `_end` | Agent → Server | A streaming response (logs, stats) |
| `http_cancel` | Server → Agent | Cancel an in-flight request |

This is how containers, images, volumes and networks appear in McHarbor at all: the host
issues Docker API calls and the agent answers them against its local socket.

## The streams that cannot be proxied

Hijacked byte streams do not survive the `http_*` path, so each gets its own family:

| Family | Messages | What it carries |
|---|---|---|
| **Exec** | `exec_start`, `exec_input`, `exec_output`, `exec_resize`, `exec_end` | An interactive terminal into a container |
| **Host terminal** | `host_terminal_start`, `_input`, `_output`, `_resize`, `_end` | A shell on the **host** |
| **Host logs** | `host_log_request`, `host_log_chunk`, `host_log_end` | A bounded journal or file snapshot |
| **Console** | `console_start`, `console_input`, `console_output`, `console_resize`, `console_end` | Guest byte streams: KVM VNC/SPICE/serial, LXC PID 1 tty |

This is why an interactive terminal through an agent needs **agent v1.1.0 or newer**: the
Docker SDK's non-detached exec bypasses the agent transport entirely, so the dedicated
messages had to exist first. Shell *detection* uses a detached exec, which is why an older
agent can list shells and still fail to open one.

### The ordering hazard

The stream reader for a session must exist **before** the first chunk is dispatched.

The `http_response_start` bug was exactly this: chunks arriving before the reader existed
were silently dropped, and short responses hung forever. In the console family the same
mistake shows up as an RFB handshake that never completes. If you are changing either side of
a streaming family, this is the failure mode to test for.

## Compose

| Message | Purpose |
|---|---|
| `compose_run` | Run a Compose command in a staged project under `MCHARBOR_COMPOSE_DIR` |
| `compose_result` | The outcome |
| `compose_cancel` | Cancel it |

## Direct transfers

| Message | Purpose |
|---|---|
| `transfer_prepare` → `transfer_ready` | Ask the target agent to stand up a receiver |
| `transfer_probe` | Test direct reachability from source to target |
| `transfer_image`, `transfer_archive` | Stream an image or a container archive, source agent → target agent |
| `transfer_restore` | Pull a restore archive from McHarbor and apply it locally |
| `transfer_progress` | Bytes sent directly |
| `transfer_result`, `transfer_cancel` | Outcome and cancellation |

The receiver binds `MCHARBOR_TRANSFER_LISTEN`, default `0.0.0.0:8788`. Where the direct path
does not work, transfers fall back to relaying through McHarbor.

## Backup and restore, on the host

| Message | Purpose |
|---|---|
| `backup_run` | Create the archive locally and upload it |
| `backup_restore` | Download the archive locally and restore entries |

Doing it on the host is what keeps a large backup from crossing the link twice.

## Snapshots

| Message | Purpose |
|---|---|
| `snapshot_detect_request` / `_response` | Which snapshot drivers exist here? |
| `snapshot_capability_request` / `_response` | Targets for one driver, and the tool version |
| `snapshot_create_request` / `_response` | Freeze the given targets; returns the snapshot name and mount path |
| `snapshot_remove_request` / `_response` | Destroy it |

ZFS, Btrfs and LVM on Linux, VSS on Windows. The agent owns these calls because only it can
see the host's PATH.

## Host stats and prune

| Message | Purpose |
|---|---|
| `host_stats_request` / `_response` | A host stats snapshot, read from `/proc` |
| `prune_request` / `_response` | Run `docker prune` locally |

## Container stats

| Message | Direction | Purpose |
|---|---|---|
| `stats_subscribe` | Server → Agent | Start pushing at `intervalSeconds`, or retune a pusher that is already running |
| `stats_push` | Agent → Server | One interval's samples, stamped with a per-connection `session` and an increasing `seq` |
| `stats_push_ack` | Server → Agent | `accepted`, `duplicate`, the next `intervalSeconds`, or `stop` |

One batch is in flight at a time. A batch at or below the last `seq` of its session is
acknowledged as a duplicate and not applied, so a resend never rolls the view back. A batch
that is not acknowledged goes into the offline buffer and replays under [Offline
replay](#offline-replay). The server sends `stats_subscribe` only to an agent at 2026.09.01
build 202609251653 or later: an older one has no handler for it and would drop it silently.
Details and costs are in [Container stats](../stats/README.md).

## Compute: guests on the host

| Family | Messages |
|---|---|
| **Capability** | `compute_probe_request` / `_response`: what can this host actually run? |
| **Images** | `compute_image_fetch`, `_list`, `_listed`, `_progress`, `_remove`, `_removed`, `_result`, `_cancel` |
| **Storage pools** | `pool_detect_*`, `pool_create_*`, `pool_stats_*`, `pool_remove_*` |
| **Networking** | `net_detect_*`, `net_ensure_*` |
| **System containers** | `ct_create`, `ct_start`, `ct_stop`, `ct_restart`, `ct_shutdown`, `ct_freeze`, `ct_unfreeze`, `ct_delete`, `ct_query`, `ct_state`, `ct_stats`, `ct_processes`, `ct_update_limits`, `ct_snapshot_*`, `ct_clone_*` |
| **Virtual machines** | The equivalent VM family, plus the console family above |

The capability probe runs on agent connect, on demand, and every six hours. **Every reported
item carries its own remediation string**, so the interface renders a checklist with copyable
install commands rather than a generic "capability check failed".

Guest hosting is core rather than plugin territory for one reason: a plugin cannot extend the
agent protocol or its dispatch, and the agent binary is a versioned core artifact. Everything
above the executor (schema, RBAC, routes, navigation, frontend) lives in the compute
plugins.

## Offline replay

| Message | Purpose |
|---|---|
| `offline_batch` | One bounded batch of buffered rows, agent → server |
| `offline_batch_ack` | Batch persisted, or rejected |

Batches are acknowledged, so a replay interrupted halfway resumes rather than duplicating.

## Self-update

| Message | Purpose |
|---|---|
| `agent_update_request` | Update to the given version |
| `agent_update_response` | Accepted or failed, written **before** the process goes away, so the server learns the outcome |

Inside a container the agent pulls the new image and starts a replacement, passing
`MCHARBOR_RETIRE_CONTAINER_ID` so the replacement removes its predecessor once it is running.
As a bare binary it downloads the release binary, swaps it in place and re-execs. Target
versions must look like semver, and a downloaded binary under about a megabyte is rejected as
not being a real one.

## Versions have to line up

The agent speaks a protocol version, and features have floors: interactive exec needs
v1.1.0. Keeping agents current is
[Settings → Updates → Agents](../../settings/updates/agents/README.md), which reports each
sweep as "*4 started, 9 already current, 1 unsupported, 2 excluded*".

## Related

- [Configuration](../configuration/README.md): the variables this behaviour reads
- [Operations](../operations/README.md): updating, tokens, diagnostics
- [Use cases](../use-cases/README.md): what this machinery is for
