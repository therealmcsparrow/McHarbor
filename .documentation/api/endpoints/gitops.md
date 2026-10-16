# Git, GitOps and the reconciler

48 endpoints: git repositories and their sync, promotion pipelines with approval gates, and
the desired-state reconciler with its plans, enforcement and Git sources.

| Key | Covers |
|---|---|
| `git.view` / `git.manage` | Repositories, deployments, manual sync |
| `gitops.view` | Pipelines, promotions, approvals, PR previews |
| `gitops.manage` | Create and edit pipelines, execute a promotion |
| `gitops.delete` | Delete a pipeline |
| `gitops.approve` | Approve or reject a promotion gate |
| `reconciler.view` / `reconciler.manage` | Desired states, drift, reconcile, plans and their approval, Git sources |
| `reconciler.enforce` | Put a desired state in enforce mode, change the enforcement limits, reset the breaker |

## Things worth knowing first

**Approving is its own permission.** `gitops.approve` is deliberately separate from
`gitops.manage`: the person who can edit a pipeline is not automatically the person who may
wave a promotion through it.

**The PR webhook is public and signature-verified.**
`POST /api/webhooks/gitops/pr` accepts GitHub, GitLab and Gitea pull-request events,
authenticating on `X-Hub-Signature-256` or `X-Gitlab-Token` rather than on a session. That is
the only unauthenticated route in this area.

**Repository credentials are referenced, not stored inline.** `credentialRef` points at a
stored credential; the repository row never carries a key or a token in a response.

**Drift and reconcile are separate calls.** `GET /api/reconciler/{id}/drift` reports the
difference between desired and actual state without touching anything;
`POST /api/reconciler/{id}/reconcile` acts on it. When the environment cannot be reached,
the drift call fails with `err.reconciler.docker_failed` rather than reporting the
container as missing.

**A background loop keeps drift current.** Every `MCHARBOR_RECONCILE_INTERVAL` (default
`60s`) the elected leader checks every desired state and updates `lastReconcile` and
`driftDetected`. Detecting is read-only; what happens next depends on the desired state's
`mode` (see *Answering drift*). Three rules keep it honest:

- **Unreachable is unknown, not drifted.** A desired state on an environment the loop
  cannot reach is left untouched.
- **Drift is confirmed before it is recorded.** It must be seen on consecutive passes (two
  by default, `confirmPasses` in the reconciler settings), so a container caught mid-restart
  does not flip the flag.
- **Every change of the flag is audited** as `reconciler.drift.detected` (warning) or
  `reconciler.drift.resolved` (info), with no user attached and the diff in the details.

The loop's health is in the `mcharbor_reconciler_*` metrics; see
[Observability](../../observability/README.md).

### What a desired state can watch

| `kind` | Needs | Drift when |
|---|---|---|
| `container` (default) | `containerName`, `imageRef` | missing or not in the desired status; another image; the restart policy or anything in `config` differs |
| `stack` | `stackName` (a McHarbor stack) | a service the compose file starts by default is missing or not running; the project runs a container for a service the file does not declare |
| `environment` | `envId` | a container runs on the environment that no `container` or `stack` state **in the same environment** accounts for, such as the one started by hand during an incident and never removed |

An environment watch leaves out McHarbor's own container and anything labelled
`mcharbor.reconciler.ignore=true`. The direct `POST /api/reconciler/{id}/reconcile` acts on
container watches only and refuses the others with `409 err.reconciler.failed` and
`data.reason: "observe-only"`; a stack or environment watch is answered with a plan instead.

**Compose is read, not guessed.** The expected services come from a YAML parse of the
stack's `services:` mapping. A service with `profiles` is not started by a plain `up`, so it
is not expected. It is declared, though,, so if it runs it is not reported as undeclared either.
A compose file that uses `include:` is reported as **unknown** rather than drifted: the
services it pulls in cannot be seen without running compose, and a wrong expected set would
produce drift that never goes away.

### `config`

A typed object, checked strictly when written. An unknown key is refused with
`400 err.invalid_body` instead of being silently never compared:

| Key | Compared with |
|---|---|
| `env` | each listed variable must be set to that value; others are ignored |
| `labels` | each listed label must be set to that value; others are ignored |
| `ports` | `"8080:80"`, `"8080:80/udp"` or `"127.0.0.1:8080:80"` must be published |
| `mounts` | `"source:/target"` (a `:ro` suffix is accepted): the volume name or host path mounted at the target |
| `memoryBytes`, `nanoCpus` | the container's limits |
| `networks` | each listed network must be attached |
| `capAdd`, `capDrop` | each listed capability must be present; `NET_ADMIN` and `CAP_NET_ADMIN` are the same |
| `healthcheck` | `{test, interval?, timeout?, retries?}`: `test` exactly, the others only when given (`"30s"`) |
| `checkImageDigest` | when `true`, report **image** drift if `imageRef` now names a different image than the container runs. Off by default: `latest` moves all the time, and a report people mute helps nobody |

### The report

Each drift carries a `class` (`configuration`, `state`, `existence` or `image`) with the
`field`, the `expected` and `actual` values, and `firstSeenAt`, which survives between
passes so week-old drift does not look new. The last report is stored on the desired state
(`lastReport`) and shown under **Drift** on the Reconciler page.

### Following drift

- **Drift report**: `GET /api/reconciler/report` (`reconciler.view`; `?env=`, `?class=`) returns one
  row per confirmed drift across every watch, what has been wrong longest first. Drift seen on
  one pass but not yet confirmed is left out. The **Drift report** page (from the Reconciler
  page, `/reconciler/report`) shows it with a class filter and CSV export.
- **Alerts**: the built-in *Platform* alert source has a **Desired state drifted** condition.
  A rule on it fires when the reconciler confirms drift on a watch and resolves when it clears,
  through whichever channels the rule names, including push. It reads what the loop recorded,
  so an alert can never disagree with the page, and an unreachable host never alerts.
- **Dashboard**: the **Drift** widget lists drifted watches with their classes and how long
  they have been wrong; it needs `reconciler.view` and asks nothing without it.

### Credentials in a desired state

A desired state's `config` and a plugin kind's `spec` are encrypted at rest, because both can pin
container environment values that include credentials. Responses never carry them: every
credential-looking env value in `config` (the `env` object) or in `spec` comes back as the mask
`••••••••` (eight bullets), and the same masking applies to the steps of a plan. Send the mask back
unchanged in a `PUT /api/reconciler/{id}` and the stored value is kept; a masked key with nothing
stored behind it is dropped. Rows written before this change are encrypted at the first start that
has an encryption key. Without the encryption service a save is refused rather than stored in
plaintext.

### Answering drift

A desired state has a `mode`:

| `mode` | When drift is confirmed |
|---|---|
| `observe` (default) | Recorded, audited, alerted on. Nothing else |
| `propose` | Also writes a **plan**: the exact steps that would restore the desired state, for a person to approve |
| `enforce` | Applies the plan without asking, within the enforcement limits; anything it cannot or may not do becomes a plan instead |

`POST` and `PUT /api/reconciler` accept `observe` and `propose` only; `enforce` is refused with
`data.reason: "enforce_route"`. Enforcement is granted through `POST /api/reconciler/{id}/enforce`,
under its own key, `reconciler.enforce`. Being able to edit intent is not the same grant as
letting McHarbor act on it unattended. Lowering the mode back is a plain edit.

**Plans.** A plan's steps are `start`, `stop`, `remove`, `recreate` (with the image, env,
labels, restart policy, limits, healthcheck and capabilities it restores, merged into what the
container already has), `stack_up`, or `manual` with a `reason`:
`missing`, `compose_managed`, `ports`, `mounts`, `networks`, `protected`, `stack_remove`,
`stack_stop`, `git_compose`. Undeclared containers on an environment watch are **stopped,
never removed**. One pending plan exists per desired state; a newer drift supersedes it, and
drift that clears closes it.

**Approving re-checks reality.** `POST /api/reconciler/proposals/{pid}/approve` detects again
first. If the drift changed since the plan was written the plan is superseded, nothing is
applied, and the response is `200` with `{stale: true, proposal: <the fresh plan>}`. An
unreachable environment refuses with `503` and `data.reason: "unknown"`. Otherwise the plan is
claimed atomically (a second approval gets `409 data.reason: "not_pending"`) and applied
through the same container and stack services a person's click uses, stopping at the first
failed step. The resolver is always the caller.

**Enforcement is off, and bounded.** It acts only when all of these hold: the desired state is
in `enforce` mode; `enforceEnabled` is on in the settings (off by default); the drift is
confirmed, never unknown; every step is applicable; the steps fit what is left of
`maxActionsPerPass` (default 3); and the desired state is outside its `cooldownSeconds`
(default 600) since the last enforced action. After `failureThreshold` (default 3)
consecutive failures the **circuit breaker** opens and nothing is enforced until someone with
`reconciler.enforce` calls `POST /api/reconciler/enforcement/reset`. Every enforced action is a
plan with `origin: "enforce"`, audited as `reconciler.enforce.applied` or
`reconciler.enforce.failed` with the diff, the steps and their results;
`reconciler.enforce.deferred` records why a plan was written instead, and
`reconciler.enforce.breaker_opened` (critical) the trip. The *Platform* alert source has a
**Drift enforcement halted** condition that is active while the breaker is open.

### Desired state from Git

A **source** names a repository from `/api/git`, a branch, a directory and an environment.
Every `.yaml`, `.yml` or `.json` document under the directory becomes a desired state, either
one per file or several under `desiredStates:`:

```yaml
desiredStates:
  - name: web
    kind: container
    containerName: web
    imageRef: nginx:1.27
    restartPolicy: unless-stopped
    mode: propose
    config:
      env: { MODE: prod }
  - name: shop
    kind: stack
    stackName: shop
    composeFile: stacks/shop/compose.yaml
```

- The loop checks each enabled source's branch at the start of a pass (`ls-remote`, no clone)
  and syncs only when it moved; `POST /api/reconciler/sources/{sid}/sync` syncs now.
- A sync is **all or nothing**: one invalid document fails it, leaves every row as it was, and
  records the reason as the source's `lastError`. A document removed from the repository
  deletes its desired state.
- `mode: enforce` is refused in a document; enforcement is granted in McHarbor, not by a
  commit. A row already put in `enforce` keeps it while its document says `propose`; a document
  saying `observe` lowers it.
- A stack's `composeFile` (relative to the repository root) is what its expected services are
  read from, instead of the stack McHarbor holds; such a stack's `stack_up` step is `manual`
  with reason `git_compose`, because its deploy belongs to its pipeline.
- Rows from Git carry `sourceId` and `sourcePath`; `PUT` accepts only a `mode` change for them
  and `DELETE` refuses them (`409 data.reason: "git_managed"`). Deleting the source keeps its
  rows and hands them back to the API.

**Drift as a pull request.** `POST /api/reconciler/{id}/pull-request` rewrites a Git-managed
desired state's document to match what is running (image, restart policy, env and label
values, desired status, limits; an environment watch's undeclared containers are declared in a
new file), commits it to a new branch `mcharbor/drift-<id>-<time>`, pushes it with the
repository's credential, and opens a pull request on GitHub, GitLab or Gitea/Forgejo. What
cannot be written as intent (a digest, a mount, a port, a network) is listed as `skipped`. On
a host it does not recognise, the branch is still pushed and `providerError` says why no
request was opened. A stack watch is not expressible (`409 data.reason: "not_expressible"`).

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/git` | `git.view` | Returns a paginated list of git repositories | — |
| `POST` | `/api/git` | `git.manage` | Adds a new git repository | — |
| `DELETE` | `/api/git/{id}` | `git.manage` | Removes a git repo | `{id}` |
| `GET` | `/api/git/{id}` | `git.view` | Returns a single git repo | `{id}` |
| `PUT` | `/api/git/{id}` | `git.manage` | Updates an existing git repo | `{id}` |
| `GET` | `/api/git/{id}/deployments` | `git.view` | Returns deployments for a repo | `{id}` |
| `POST` | `/api/git/{id}/sync` | `git.manage` | Triggers a git sync for a repository | `{id}` |
| `GET` | `/api/gitops/approvals` | `gitops.view` | Returns pending approval requests | `?status=` |
| `POST` | `/api/gitops/approvals/{id}/approve` | `gitops.approve` | Approves a pending promotion gate; body `{note}`. The resolver recorded is always the caller | — |
| `POST` | `/api/gitops/approvals/{id}/reject` | `gitops.approve` | Rejects a pending promotion gate; body `{note}`. The resolver recorded is always the caller | — |
| `GET` | `/api/gitops/pipelines` | `gitops.view` | Returns all gitops pipelines | — |
| `POST` | `/api/gitops/pipelines` | `gitops.manage` | Creates a new pipeline | — |
| `DELETE` | `/api/gitops/pipelines/{id}` | `gitops.delete` | Removes a pipeline | `{id}` |
| `GET` | `/api/gitops/pipelines/{id}` | `gitops.view` | Returns a single pipeline with stages | `{id}` |
| `PUT` | `/api/gitops/pipelines/{id}` | `gitops.manage` | Updates a pipeline | `{id}` |
| `POST` | `/api/gitops/pipelines/{id}/promote` | `gitops.manage` | Executes a promotion to a stage | `{id}` |
| `GET` | `/api/gitops/pipelines/{id}/promotions` | `gitops.view` | Returns recent promotions for a pipeline | `{id}` |
| `GET` | `/api/gitops/previews` | `gitops.view` | Returns recent PR preview environments | `?pipelineId=` |
| `GET` | `/api/reconciler` | `reconciler.view` | Returns a paginated list of desired states | — |
| `POST` | `/api/reconciler` | `reconciler.manage` | Creates a new desired state | — |
| `DELETE` | `/api/reconciler/{id}` | `reconciler.manage` | Removes a desired state | `{id}` |
| `GET` | `/api/reconciler/kinds` | `reconciler.view` | The kinds installed plugins offer, with their properties | — |
| `GET` | `/api/reconciler/kinds/{source}/{kind}/scopes` | `reconciler.manage` | A plugin's own scopes (Kubernetes clusters); empty when it works within environments | `{source}`, `{kind}` |
| `GET` | `/api/reconciler/kinds/{source}/{kind}/subjects` | `reconciler.manage` | What can be picked for a kind in one scope | `?env=` |
| `GET` | `/api/reconciler/kinds/{source}/{kind}/options` | `reconciler.manage` | Choices for a property whose options depend on the scope | `?env=`, `?field=` |
| `GET` | `/api/reconciler/kinds/{source}/{kind}/current` | `reconciler.manage` | A subject's current values, for "use current values" | `?env=`, `?subject=` |
| `GET` | `/api/reconciler/report` | `reconciler.view` | One row per confirmed drift across every watch, oldest first | `?env=`, `?class=` |
| `GET` | `/api/reconciler/{id}` | `reconciler.view` | Returns a single desired state | `{id}` |
| `PUT` | `/api/reconciler/{id}` | `reconciler.manage` | Updates an existing desired state | `{id}` |
| `GET` | `/api/reconciler/{id}/drift` | `reconciler.view` | Checks for drift between desired and actual state | `{id}` |
| `POST` | `/api/reconciler/{id}/reconcile` | `reconciler.manage` | Triggers reconciliation for a desired state | `{id}` |
| `POST` | `/api/reconciler/{id}/propose` | `reconciler.manage` | Checks now and writes a plan for the current drift (or returns the pending one for the same drift) | `{id}` |
| `POST` | `/api/reconciler/{id}/enforce` | `reconciler.enforce` | Puts the desired state in enforce mode | `{id}` |
| `POST` | `/api/reconciler/{id}/pull-request` | `reconciler.manage` | Writes a Git-managed desired state's drift back to its repository as a pull request | `{id}` |
| `GET` | `/api/reconciler/proposals` | `reconciler.view` | Plans, newest first | `?status=`, `?env=`, pagination |
| `GET` | `/api/reconciler/proposals/{pid}` | `reconciler.view` | One plan with its drift, steps and results | `{pid}` |
| `POST` | `/api/reconciler/proposals/{pid}/approve` | `reconciler.manage` | Re-checks, then applies the plan; returns `{proposal, stale}` | `{pid}` |
| `POST` | `/api/reconciler/proposals/{pid}/reject` | `reconciler.manage` | Closes the plan without acting; body `{note}` | `{pid}` |
| `GET` | `/api/reconciler/settings` | `reconciler.view` | Debounce, enforcement limits and breaker state | — |
| `PUT` | `/api/reconciler/settings` | `reconciler.manage` | Sets `confirmPasses` (1 to 10) | — |
| `PUT` | `/api/reconciler/enforcement` | `reconciler.enforce` | Sets `enforceEnabled`, `maxActionsPerPass` (1 to 25), `cooldownSeconds` (0 to 86400), `failureThreshold` (1 to 20) | — |
| `POST` | `/api/reconciler/enforcement/reset` | `reconciler.enforce` | Closes the circuit breaker | — |
| `GET` | `/api/reconciler/sources` | `reconciler.view` | Git sources of desired state | — |
| `POST` | `/api/reconciler/sources` | `reconciler.manage` | Adds a source and syncs it once; body `{name, repoId, branch, path, envId, enabled}` | — |
| `PUT` | `/api/reconciler/sources/{sid}` | `reconciler.manage` | Replaces a source's settings | `{sid}` |
| `DELETE` | `/api/reconciler/sources/{sid}` | `reconciler.manage` | Removes a source; its desired states stay | `{sid}` |
| `POST` | `/api/reconciler/sources/{sid}/sync` | `reconciler.manage` | Syncs now; a failure answers `502 data.reason: "sync_failed"` and the reason is on the source | `{sid}` |
| `GET` | `/api/gitops/repos/{repoId}/webhook-secret` | `gitops.view` | Whether the repository has a webhook secret; never the secret | `{repoId}` |
| `POST` | `/api/gitops/repos/{repoId}/webhook-secret` | `gitops.manage` | Generates or rotates the secret; the response carries it once | `{repoId}` |
| `DELETE` | `/api/gitops/repos/{repoId}/webhook-secret` | `gitops.manage` | Removes the secret; deliveries are refused until a new one exists | `{repoId}` |
| `POST` | `/api/webhooks/gitops/pr` | — | Receives GitHub/GitLab/Gitea PR events | — |


## Request bodies

**`POST` `/api/git`**: `name`, `url`, `branch`, `path`, `authType`, `credentialRef`, `autoSync`, `syncInterval`

**`PUT` `/api/git/{id}`**: `name`?, `url`?, `branch`?, `path`?, `authType`?, `credentialRef`?, `autoSync`?, `syncInterval`?

**`POST` `/api/gitops/pipelines`**: `name`, `repoId`, `description`, `triggerType`, `stages`

**`PUT` `/api/gitops/pipelines/{id}`**: `name`?, `description`?, `enabled`?, `triggerType`?, `stages`?

**`POST` `/api/gitops/pipelines/{id}/promote`**: `stageId`, `commitSha`, `note`, `triggeredBy`, `prNumber`

**`POST` `/api/reconciler`**: `name`, `description`, `envId`, `containerName`, `imageRef`, `desiredStatus`, `restartPolicy`, `config`

A plugin kind takes `kind`, `source` (the plugin slug), `subjectId`, `envId` (the environment, or the cluster for Kubernetes) and `spec`: the pinned properties as strings; an empty value is not managed. The container fields are ignored for it.

**`PUT` `/api/reconciler/{id}`**: `name`?, `description`?, `envId`?, `containerName`?, `imageRef`?, `desiredStatus`?, `restartPolicy`?, `config`?

**`POST` `/api/webhooks/gitops/pr`**: `action`, `prNumber`, `prTitle`, `sourceBranch`, `targetBranch`, `commitSha`, `repoId`

## Related

- [Stacks](./stacks.md): what a pipeline deploys
- [GitOps screen](../../screens/gitops/README.md)
