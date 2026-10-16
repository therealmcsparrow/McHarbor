# Environments, agents and consoles

32 endpoints. An environment is a connection to one Docker or Kubernetes host. Everything
else in the API points at one through [`?env=`](../conventions/parameters.md).

| Key | Covers |
|---|---|
| `environments.view` | List, inspect, topology, socket detection, agent status and logs |
| `environments.manage` | Create, update, delete, test, deploy an agent, mint and rotate tokens |

## Things worth knowing first

**Credentials go in, never out.** Socket paths, TLS material, SSH keys, kubeconfigs and agent
tokens are encrypted at rest and are not returned by any read endpoint. An agent token is
shown once, at creation, and redacted afterwards.

**Connection types.** `socketPath` for a local socket, `host` plus `port` and TLS material
for TCP, `sshHost` / `sshUser` / `sshKey` for SSH, and an agent token for a host behind NAT.
`GET /api/environments/detect-socket` finds the sockets the McHarbor container can actually
see.

**Connection details are for managers only.** `GET /api/environments` and
`GET /api/environments/{id}` return `socketPath`, `host`, `port`, `sshHost`, `sshPort` and
`sshUser` only to a caller holding `environments.manage`, the people who can change them.
A caller with `environments.view` alone gets the environment without them.

**A failed connection test is a 200.** `POST /api/environments/{id}/test` returns a per-step
report so the dialog can show which step broke, with a rolled-up badge.

**Agents connect outbound.** The agent dials `GET /api/agent/ws?token=…`; McHarbor never
opens a connection to the host and no Docker socket is ever exposed. Two routes exist purely
to bootstrap that: `POST /api/agents/{envId}/install-token` mints a one-time token, and
`GET /api/agent/install/{token}` serves the install script against it. Both are reachable
without a session, because the operator on the target host has none.

**`POST /api/agents/{envId}/deploy`** is the other path: McHarbor reaches the host over SSH
and installs the agent itself. It requires a host key fingerprint; there is no
trust-on-first-use.

**Per-environment behaviour lives on the environment row.** Scheduled update checks,
automatic image pruning, event tracking, metric collection, disk-usage notifications and
their threshold, the auto-update window and days, and per-environment log and metric
retention are all fields on `PUT /api/environments/{id}` rather than global settings.

**Console access is ticketed.** `POST /api/compute/console/ticket` is where the permission
check happens; the WebSocket that follows carries only the ticket, because a browser cannot
set headers on a handshake. See [Streaming](../conventions/streaming.md).
## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/agent/binary/{os}/{arch}` | — | Serves the agent binary for one platform. Public, like the install script: the same bytes are on the project's GitHub releases | `{os}`, `{arch}` |
| `GET` | `/api/agent/install/{token}` | — | Serves the install script for a one-time token | `{token}` |
| `GET` | `/api/agent/ws` | — | Handles the WebSocket connection from a remote agent | `?token=` |
| `GET` | `/api/agents` | — | Returns status of all agent-type environments | — |
| `POST` | `/api/agents/direct-transfer-test` | `environments.manage` | Runs a lightweight agent-to-agent connectivity probe | — |
| `POST` | `/api/agents/install-prereqs` | `environments.manage` | Installs exactly the prerequisites the operator ticked and re-probes, without deploying an agent or minting a token; audited | — |
| `POST` | `/api/agents/probe` | `environments.manage` | Connects over SSH, reports what the host has, and disconnects; no environment and no token | — |
| `DELETE` | `/api/agents/{envId}/access` | `environments.manage` | Clears the stored SSH access for an environment | `{envId}` |
| `GET` | `/api/agents/{envId}/access` | `environments.manage` | Returns the saved SSH access: host, port, user, method, fingerprint and the vault secret it uses, never the key or password | `{envId}` |
| `PUT` | `/api/agents/{envId}/access` | `environments.manage` | Stores the SSH access an operator entered | `{envId}` |
| `GET` | `/api/agents/{envId}/access/secrets` | `environments.manage` | Lists vault rows usable as an SSH credential: id, name and kind only | `{envId}` |
| `POST` | `/api/agents/{envId}/deploy` | `environments.manage` | Deploys the agent to a remote host via SSH | `{envId}` |
| `GET` | `/api/agents/{envId}/health/history` | `environments.view` | Returns the sampled health and uptime series behind the Overview tab's agent cards | `{envId}`, `?limit=` |
| `POST` | `/api/agents/{envId}/install-token` | `environments.manage` | Creates a one-time install token for an agent environment | `{envId}` |
| `GET` | `/api/agents/{envId}/logs` | `environments.view` | Returns the agent's own log lines replayed from its offline buffer, newest first, paginated | `{envId}` |
| `POST` | `/api/agents/{envId}/redeploy` | `environments.manage` | Repeats the last deployment from what was remembered: same host, user, method and vault credential | `{envId}` |
| `POST` | `/api/agents/{envId}/regenerate-token` | `environments.manage` | Creates a new token for an agent environment | `{envId}` |
| `GET` | `/api/agents/{envId}/status` | — | Returns agent status for a specific environment | `{envId}` |
| `POST` | `/api/compute/console/ticket` | — | Issues a console ticket | — |
| `GET` | `/api/compute/console/{ticket}` | — | Upgrades a ticketed request and bridges it to the guest | `{ticket}` |
| `GET` | `/api/environments` | `environments.view` | Returns all environments | — |
| `POST` | `/api/environments` | `environments.manage` | Creates a new environment | — |
| `GET` | `/api/environments/detect-socket` | `environments.view` | Auto-detects available Docker sockets | — |
| `GET` | `/api/environments/duplicates` | `environments.view` | Lists environments that reach the same machine | — |
| `DELETE` | `/api/environments/{id}` | `environments.manage` | Removes an environment | `{id}` |
| `GET` | `/api/environments/{id}` | `environments.view` | Returns a single environment by ID | `{id}` |
| `PUT` | `/api/environments/{id}` | `environments.manage` | Updates an existing environment | `{id}` |
| `POST` | `/api/environments/{id}/merge` | `environments.manage` | Folds one environment into another; `?dryRun=true` returns the plan without writing | `{id}`, `?dryRun=` |
| `DELETE` | `/api/environments/{id}/migrate` | `environments.manage` | Disarms a pending connection migration | `{id}` |
| `POST` | `/api/environments/{id}/migrate` | `environments.manage` | Converts an environment between connection types; mints or revokes a credential and can drop a live agent connection | `{id}` |
| `POST` | `/api/environments/{id}/test` | `environments.manage` | Tests the Docker connection for an environment and returns a per-step report | `{id}` |
| `GET` | `/api/environments/{id}/topology` | `environments.view` | Returns the topology graph (containers + networks + volumes + their connections) for a single environment | `{id}` |


## Request bodies

**`POST` `/api/agents/direct-transfer-test`**: `sourceEnvId`, `targetEnvId`

**`POST` `/api/agents/install-prereqs`**: `sshHost`, `sshPort`, `sshUser`, `sshAuthType`, `sshKey`?, `sshPassword`?, `rootPassword`?, `hostKeyFingerprint`, `method`, `agentImage`?, `sshSecretId`?, `saveCredentials`?, `installPrereqs`?, `probeOnly`?, `scope`?

**`POST` `/api/agents/probe`**: `sshHost`, `sshPort`, `sshUser`, `sshAuthType`, `sshKey`?, `sshPassword`?, `rootPassword`?, `hostKeyFingerprint`, `method`, `agentImage`?, `sshSecretId`?, `saveCredentials`?, `installPrereqs`?, `probeOnly`?, `scope`?

**`PUT` `/api/agents/{envId}/access`**: `sshHost`, `sshPort`, `sshUser`, `sshAuthType`, `sshKey`?, `sshPassword`?, `sshSecretId`?, `hostKeyFingerprint`, `method`

**`POST` `/api/agents/{envId}/deploy`**: `sshHost`, `sshPort`, `sshUser`, `sshAuthType`, `sshKey`?, `sshPassword`?, `hostKeyFingerprint`, `method`, `agentImage`?

**`POST` `/api/agents/{envId}/redeploy`**: `sshHost`, `sshPort`, `sshUser`, `sshAuthType`, `sshKey`?, `sshPassword`?, `rootPassword`?, `hostKeyFingerprint`, `method`, `agentImage`?, `sshSecretId`?, `saveCredentials`?, `installPrereqs`?, `probeOnly`?, `scope`?

**`POST` `/api/compute/console/ticket`**: `envId`, `guestKind`, `guestId`, `kind`

**`POST` `/api/environments`**: `name`, `connectionType`, `socketPath`?, `host`?, `port`?, `tlsCa`?, `tlsCert`?, `tlsKey`?, `sshHost`?, `sshPort`?, `sshUser`?, `sshKey`?, `isDefault`, `agentToken`?

**`PUT` `/api/environments/{id}`**: `name`?, `connectionType`?, `socketPath`?, `host`?, `port`?, `tlsCa`?, `tlsCert`?, `tlsKey`?, `sshHost`?, `sshPort`?, `sshUser`?, `sshKey`?, `isDefault`?, `isActive`?, `scheduledUpdateCheckEnabled`?, `automaticImagePruningEnabled`?, `trackContainerEventsEnabled`?, `collectContainerMetricsEnabled`?, `highlightContainerChangesEnabled`?, `dockerDiskUsageNotificationsEnabled`?, `dockerDiskUsageThresholdPercent`?, `dockerDiskUsageUseGlobalDefault`?, `timezone`?, `logRetentionDays`?, `containerPruneEnabled`?, `containerPruneStoppedDays`?, `imagePruneDanglingOnly`?, `autoUpdateEnabled`?, `autoUpdateWindowStart`?, `autoUpdateWindowEnd`?, `autoUpdateDays`?, `metricRetentionHours`?

**`POST` `/api/environments/{id}/merge`**: `sourceId`

**`POST` `/api/environments/{id}/migrate`**: `targetType`, `socketPath`?, `host`?, `port`?, `tlsCa`?, `tlsCert`?, `tlsKey`?, `sshHost`?, `sshPort`?, `sshUser`?, `sshKey`?, `agentToken`?, `immediate`?, `skipAgentDeploy`?, `agentImage`?, `insecureTls`?

## Related

- [Agents](../../agents/README.md): installing and operating the remote agent
- [Host](./host.md): what an environment reports about its machine
- [Environments screen](../../screens/environments/README.md)
