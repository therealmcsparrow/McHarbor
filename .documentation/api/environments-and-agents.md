# Environments and Agents

McHarbor stores Docker, Kubernetes, and agent-backed targets as environments.
Most runtime APIs operate against one of these targets through `?env=`.

## Environments

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/environments/` | Lists configured environments. |
| POST | `/api/environments/` | Creates an environment definition. |
| GET | `/api/environments/detect-socket` | Auto-detects Docker or Podman socket candidates. |
| GET | `/api/environments/{id}` | Returns one environment definition. |
| PUT | `/api/environments/{id}` | Updates an environment definition. |
| DELETE | `/api/environments/{id}` | Deletes an environment definition. |
| POST | `/api/environments/{id}/test` | Tests connectivity and returns version details. |

Representative create body:

```json
{
  "name": "local-docker",
  "orchestratorType": "docker",
  "connectionType": "socket",
  "socketPath": "/var/run/docker.sock",
  "isDefault": true
}
```

Representative Kubernetes environment body:

```json
{
  "name": "remote-k3s",
  "orchestratorType": "kubernetes",
  "connectionType": "kubeconfig",
  "kubeconfig": "base64-or-raw-kubeconfig",
  "k8sNamespace": "default",
  "isDefault": false
}
```

## Agent Routes

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/agent/ws?token=...` | WebSocket endpoint used by the remote agent binary. |
| GET | `/api/agent/install/{token}` | Returns an installation script for agent bootstrap. |
| GET | `/api/agents/` | Lists environments that use the agent transport. |
| GET | `/api/agents/{envId}/status` | Returns status, connectivity, and version information for one agent environment. |
| POST | `/api/agents/{envId}/regenerate-token` | Rotates the long-lived agent token. |
| POST | `/api/agents/direct-transfer-test` | Runs a source-agent to target-agent direct transfer reachability probe. |
| POST | `/api/agents/{envId}/deploy` | Starts deployment-oriented agent setup flow. |
| POST | `/api/agents/{envId}/install-token` | Creates an install token for scripted bootstrap. |

Agent bootstrap and deploy flows pin the generated Docker image reference to the
current agent release, for example:

```text
ghcr.io/therealmcsparrow/mcharbor-agent:1.3.6
```

The WebSocket auth payload includes hostname, OS, architecture, agent version,
Docker API version, and optionally a direct-transfer advertise URL.

## Notes

- Environment-scoped runtime routes usually require `?env=<environmentId>`.
- Agent transport is used for Docker hosts that cannot expose a daemon socket directly.
- Direct agent-to-agent container move transfers require `mcharbor-agent` `1.3.5+` on both agents.
- The direct transfer connection test requires `mcharbor-agent` `1.3.6+` on both agents.
- Direct transfer is advertised only when the target agent has `MCHARBOR_TRANSFER_LISTEN` and `MCHARBOR_TRANSFER_ADVERTISE_URL` configured.
- If direct transfer is not advertised or either agent is too old, container moves use the McHarbor relay path.
