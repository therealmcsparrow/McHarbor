# System, updates and licence

40 endpoints: health, version information, the self-update machinery, the licence, the
database wizard, cluster status and high-availability control, the load-balancer probes, the compliance posture, the generated OpenAPI document, and the command-line client downloads.

| Key | Covers |
|---|---|
| `updates.view` / `updates.manage` | Update policies, the self-update check and its settings |
| `environments.view` | List the connected agents an update sweep would touch |
| `environments.manage` | Sweep connected agents onto a new version |
| `system.manage` | Trigger a McHarbor self-update |
| `license.view` / `license.manage` | The licence, its usage samples, activation and release |
| `compliance.view` | The compliance posture and report |
| `cluster.view` | Cluster status, the HA migration plan, readiness, startup health |
| `cluster.manage` | Fail over, drain and resume a node |
| — | Health, liveness and readiness probes, about, versions, the API docs, the database wizard |

## Things worth knowing first

**`GET /api/health`, `GET /api/livez`, `GET /api/readyz` and `GET /api/about` are fully
public** here, with no token of any kind. Use health or livez as a container healthcheck.
Point a load balancer at `readyz`: it answers `503` with the failing checks while a node must be
kept out of rotation, including while it is still starting, and `livez` stays `200` meanwhile.

**`GET /api/docs` returns the OpenAPI document** for the whole API as JSON. It is
authenticated but carries no permission.

**Two different things are called "updates".** Update *policies*
(`/api/updates` with an id) describe how container images get refreshed: a container match,
an image match, a schedule, a strategy. The self-update endpoints
(`/api/updates/check`, `/state`, `/settings`, `/dismiss`, and
`POST /api/versions/self-update`) are about McHarbor upgrading itself.

**The self-update check is cached.** A background checker polls GitHub and writes the result
to settings; `GET /api/updates/state` reads it back, so the frontend can poll for a new
release without hammering the API. `GET /api/updates/check` forces a live check.

**A self-update ends the process.** It schedules a detached helper container which replaces
the image; the response returns first and the operator reconnects once the new image is up.
It only works when McHarbor manages its own local environment.

**Agent updates are a sweep, not a per-agent call.**
`POST /api/updates/agents/update` tells every connected agent older than the target version
to replace itself, bypassing the periodic retry backoff because the operator asked.
Environments can be excluded permanently through the self-update settings.

**Licence status is ungated on purpose.** Every user sees the banner, so every user must be
able to learn why it is there; `GET /api/license/status` and `POST /api/license/notice-ack`
carry counts and state only. The detail, the usage samples, activation and release are behind
`license.view` and `license.manage`.

**The database wizard runs before any account exists.** `GET /api/database/setup/status`,
`POST /api/database/setup/test` and `POST /api/database/setup/apply` sit in the rate-limited
auth group. The test and the apply need the one-time setup token in the `X-McHarbor-Setup-Token`
header (`403` with `data.reason: "setup_token"` without it), a SQLite `path` must resolve inside the
data directory (`400` `path_outside_data_dir`), and the apply holds the same lock as
`POST /api/auth/setup` and re-checks that no account exists before it writes. The status route
needs no token. Applying writes the encrypted connection secret and restarts into it, including
when the choice is SQLite, because "the operator has chosen" is a fact that cannot live in a
database a later migration would carry along.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/about` | — | Build and product information. Fully public | — |
| `GET` | `/api/cli/download/{os}/{arch}` | signed in | The `mcharbor` archive for one platform (`linux`/`darwin`/`windows`, `amd64`/`arm64`) as built into this image, or the local `cli/` build when mounted; `404` when none | — |
| `GET` | `/api/cli/downloads` | signed in | Every available archive: `os`, `arch`, `file`, `version`, `size`, `sha256` | — |
| `POST` | `/api/cluster/failover` | `cluster.manage` | Makes the named node the preferred active; the current active hands over on its next tick. `409` `not_active_passive` or `split_brain` | — |
| `GET` | `/api/cluster/ha-migration-plan` | `cluster.view` | Returns the plan for migrating this install to HA | — |
| `POST` | `/api/cluster/instances/{id}/drain` | `cluster.manage` | Asks a node to drain; that node applies it on its next tick, so any node can drain any other. `404` `unknown_node` | `{id}` |
| `POST` | `/api/cluster/instances/{id}/resume` | `cluster.manage` | Asks a drained node to resume, on its next tick | `{id}` |
| `GET` | `/api/cluster/readiness` | `cluster.view` | Evaluates the HA gates for `active-active` (the default) or `active-passive`; `ready` means no gate failed | `?target=` |
| `GET` | `/api/cluster/startup-health` | `cluster.view` | Returns what this node found at boot; `404` before a report exists | — |
| `GET` | `/api/cluster/status` | `cluster.view` | Returns high-availability cluster status | — |
| `GET` | `/api/compliance/posture` | `compliance.view` | The measured compliance posture; `export=1` audits the download. See [Compliance](../../screens/enterprise/compliance.md) | `sections`, `redact`, `export` |
| `POST` | `/api/database/setup/apply` | setup token | Records the operator's choice on a fresh install; needs the `X-McHarbor-Setup-Token` header | — |
| `GET` | `/api/database/setup/status` | — | Tells the first-run wizard whether it should show the database step at all | — |
| `POST` | `/api/database/setup/test` | setup token | Is HandleTest without the permission, for the installer who has no account yet; needs the `X-McHarbor-Setup-Token` header and is audited as `database.setup.test` | — |
| `GET` | `/api/docs` | — | Returns the OpenAPI specification as JSON | — |
| `GET` | `/api/health` | — | Liveness probe. Fully public | — |
| `GET` | `/api/license` | `license.view` | Returns the licence detail behind license.view | — |
| `POST` | `/api/license/activate` | `license.manage` | Claims a seat with the key the operator pasted | — |
| `POST` | `/api/license/deactivate` | `license.manage` | Releases the licence | — |
| `POST` | `/api/license/notice-ack` | signed in; API key needs `account.self.manage` | Records that the caller has dismissed a banner. Per user and audited; one user's dismissal does not clear it for others | — |
| `POST` | `/api/license/refresh` | `license.manage` | Forces a heartbeat | — |
| `GET` | `/api/license/status` | — | Returns the banner's payload | — |
| `GET` | `/api/license/usage` | `license.view` | Lists recorded usage samples | — |
| `GET` | `/api/livez` | — | Liveness probe: `200` while the process runs. Public | — |
| `GET` | `/api/readyz` | — | Readiness probe: `200` when this node may take traffic, `503` with the failing checks when not. Public | — |
| `GET` | `/api/updates` | `updates.view` | Returns a paginated list of update policies | — |
| `POST` | `/api/updates` | `updates.manage` | Creates a new update policy | — |
| `GET` | `/api/updates/agents` | `environments.view` | Lists every connected agent with its version, so an operator sees what an agent update would touch | — |
| `POST` | `/api/updates/agents/update` | `environments.manage` | Runs one agent update sweep now: every connected agent older than the target version is told to replace itself | — |
| `GET` | `/api/updates/check` | `updates.view` | Checks GitHub for a newer McHarbor release | — |
| `POST` | `/api/updates/dismiss` | `updates.manage` | Records that the operator has seen the current latest version so the in-app banner stops highlighting it | — |
| `GET` | `/api/updates/settings` | `updates.view` | Returns the operator-facing self-update config | — |
| `PUT` | `/api/updates/settings` | `updates.manage` | Persists the operator-facing self-update configuration | — |
| `GET` | `/api/updates/state` | `updates.view` | Returns the cached self-update check result | — |
| `DELETE` | `/api/updates/{id}` | `updates.manage` | Removes an update policy | `{id}` |
| `GET` | `/api/updates/{id}` | `updates.view` | Returns a single update policy | `{id}` |
| `PUT` | `/api/updates/{id}` | `updates.manage` | Updates an existing policy | `{id}` |
| `GET` | `/api/updates/{id}/history` | `updates.view` | Returns update execution history for a policy | `{id}` |
| `GET` | `/api/versions` | — | Returns McHarbor and agent version information | — |
| `POST` | `/api/versions/self-update` | `system.manage` | Triggers a McHarbor self-update by scheduling the detached helper container (see core/docker.ScheduleDetachedSelfUpdateHelperForImage) | — |


## Request bodies

**`POST` `/api/cluster/failover`**: `targetNodeId`

**`POST` `/api/database/setup/apply`**: `driver`, `host`?, `port`?, `name`?, `user`?, `password`?, `sslMode`?, `params`?, `dsn`?, `path`?

**`POST` `/api/database/setup/test`**: `driver`, `host`?, `port`?, `name`?, `user`?, `password`?, `sslMode`?, `params`?, `dsn`?, `path`?

**`POST` `/api/license/activate`**: `key`

**`POST` `/api/license/notice-ack`**: `kind`

**`POST` `/api/updates`**: `name`, `containerMatch`, `imageMatch`, `schedule`, `strategy`, `autoRestart`

**`GET` `/api/updates/check`**: `tag_name`, `html_url`, `published_at`, `body`

**`PUT` `/api/updates/settings`**: `intervalHours`, `channelIds`, `lastSeenVersion`, `enabled`, `autoInstall`, `autoUpdateAgents`, `agentExcludedEnvIds`

**`PUT` `/api/updates/{id}`**: `name`?, `containerMatch`?, `imageMatch`?, `schedule`?, `strategy`?, `autoRestart`?, `enabled`?

**`POST` `/api/versions/self-update`**: `image`

## Related

- [Settings](../settings/README.md): what the wizard writes
- [Getting started](../../getting-started/README.md): installing and upgrading
