# Promotions and deploys

A promotion takes one commit of a pipeline's repository and deploys the stage's Compose
file, as it exists at that commit, as a stack in the stage's target environment. The
promotion is recorded `succeeded` only when the stack came up; otherwise it is `failed`,
with the reason on the row.

Permission keys on this page: `gitops.view`, `gitops.manage`, `gitops.approve`.

## What starts a promotion

| Trigger | Route or source | Key |
|---|---|---|
| **Promote** on the GitOps page | `POST /api/gitops/pipelines/{id}/promote` | `gitops.manage` |
| `mcharbor gitops pipeline promote <pipeline-id> --stage <stage-id>` | same route | `gitops.manage` |
| An approved approval | `POST /api/gitops/approvals/{id}/approve` | `gitops.approve` |
| A new commit on the first stage's branch | the GitOps scheduler (every 60 seconds, leader only) | — |
| A pull-request preview | the pull-request webhook (see [previews](../previews/README.md)) | — |

The request body is `stageId`, `commitSha`, `note`. An empty `commitSha` promotes the
current tip of the stage's branch.

The route answers at once with the promotion in state `pending`, or already `failed` when
it could not even start (no target environment, a branch whose tip cannot be read, an
invalid Compose path). The deploy runs in the
background, because pulling images can take far longer than an HTTP request may stay open;
the deployment history on the GitOps page refreshes every ten seconds and shows the
outcome. From the CLI, `--wait` follows it and exits with code 7 and the recorded reason
when it fails.

## What a promotion does

1. **Fetches the commit.** A shallow fetch of exactly that commit into
   `<DATA_DIR>/gitops/checkouts/<promotion id>/`, authenticated with the repository's own
   stored credential (`token` or `basic`; `none` fetches anonymously). When the forge does
   not allow fetching a commit by hash, the last 50 commits of the stage's branch are
   fetched instead and the commit must be among them.
2. **Reads the Compose file** named by the stage's **Compose path** (default
   `docker-compose.yml`) from that commit's tree. No working copy is checked out.
3. **Deletes the fetch directory.** Nothing of the repository stays on disk after the file
   is read.
4. **Deploys the stack** through the stacks module: the file is written as the stack's
   Compose file and `docker compose up -d --remove-orphans` runs against the stage's target
   environment (locally, over TCP, or through the remote agent) exactly as a stack deployed
   from the Stacks page would. `${secret:NAME}` references resolve the same way.

### The stack it deploys

| Promotion into | Stack name | Owned by |
|---|---|---|
| A pipeline stage | `gitops-<pipeline>-<stage>` | that pipeline and stage |
| A pull-request preview | `preview-<pipeline>-pr-<number>` | that preview |

Names are lower-cased and reduced to letters, digits and hyphens. Each promotion of the same
stage updates the same stack.

**A promotion never overwrites a stack it did not create.** Every stack a promotion creates
records its owner; a stack of the same name that was made by hand, or by another pipeline,
is left alone and the promotion fails saying so. Rename the pipeline or the stage to deploy
under a different name.

### Limits

| Limit | Value |
|---|---|
| Repository size on disk during the fetch | 256 MiB |
| Time for the fetch | 2 minutes |
| Compose file size | 1 MiB |
| Fetches running at once, across all pipelines | 2 |
| Time for `docker compose up` | 10 minutes (the stacks module's own bound) |

The Compose path must be a relative path inside the repository. `..`, an absolute path, a
symlink or a directory is refused, both when the stage is saved and when the promotion
runs.

## What gets recorded

Each row in `GET /api/gitops/pipelines/{id}/promotions` (`gitops.view`) carries:

| Field | Meaning |
|---|---|
| `status` | `pending` while the deploy runs, then `succeeded`, `failed` or `skipped` |
| `commitSha` | The commit deployed |
| `environmentId` | Where it was deployed |
| `stackName` | The stack it deployed |
| `lastError` | Why it failed: the sentence shown behind the error icon beside the status |
| `finishedAt` | When it reached a final state |

A stage that requires approval gets its approval request only **after** the previous
stage's promotion succeeded. A failed deploy requests nothing.

A promotion still `pending` 30 minutes after it started is recorded `failed` with
"interrupted: McHarbor stopped before the deploy finished"; that is a restart in the
middle of a deploy.

The scheduler promotes each new commit once. When that deploy fails, the failure stays in
the history and the scheduler waits for the next commit rather than retrying the same one
every minute; promote again by hand once the cause is fixed.

## What goes wrong

| `lastError` reads | Cause |
|---|---|
| `stage … has no target environment` | The stage has no target environment. A `pr_preview` stage deploys only into previews. |
| `resolving the tip of branch … failed` | No commit was given and the branch could not be listed: wrong branch, or the credential cannot read the repository |
| `compose file … was not found at commit …` | The Compose path does not exist at that commit |
| `commit … is not within the last 50 commits of branch …` | The forge refused a fetch by hash and the commit is older than the fallback depth, or is not on that branch |
| `the repository exceeded the 256 MiB checkout limit` | The repository's history at depth 1 is larger than the cap |
| `fetching the repository did not finish within 2m0s` | The forge is slow or unreachable |
| `… is not managed by this caller` | A stack with the generated name already exists and was not created by this pipeline stage |
| `docker compose up failed: …` | Compose's own message, such as an image that cannot be pulled, a port already in use, an invalid file |
| `the stacks service is not available` | McHarbor was built without the stacks module |

## Known limits

- **Only the Compose file travels.** `build:` contexts, `env_file:` and other files next to
  it in the repository are not deployed with it. This is the same single-file model the remote
  agent uses. Images must be pullable.
- Deleting a pipeline leaves the stacks its stages deployed. They are ordinary stacks and
  can be removed from the Stacks page.
- Editing a pipeline replaces its stages, which also removes their promotion history.
- A self-hosted GitHub Enterprise host is not recognised as GitHub and is addressed with
  the Gitea API shape.
