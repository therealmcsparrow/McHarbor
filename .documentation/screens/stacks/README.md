# Stacks

`/stacks` · `/stacks/:name`

Docker Compose stacks: the ones McHarbor deploys, and the ones it finds already running
on the host.

## Permissions

| Key | What it unlocks |
|---|---|
| `stacks.view` | See stacks and their services |
| `stacks.manage` | Deploy, edit, up, down, stop, restart, prune, and reveal masked environment and compose values |
| `host_self.manage` | On top of `stacks.manage`, for any of those on a stack that runs McHarbor itself, and for bringing up a compose file on the environment McHarbor runs on that mounts the Docker socket or the host, runs privileged, or is named after McHarbor's own compose project. See [McHarbor's own containers](../../api/endpoints/containers.md#mcharbors-own-containers) |
| `stacks.delete` | Remove a stack |

Every `/api/stacks/{name}` route decides on the environment the stack belongs to, whatever
`?env=` the request names: a grant on one environment does not reach a stack in another. A
stack with no recorded environment needs a global grant. Creating a stack puts it in the
`?env=` environment; a body naming another `environmentId` is refused with `403`.

Adopting runs on `stacks.manage`, but the preview reconstructs running containers,
credentials included, so it arrives with credential-looking values masked.
`POST /api/stacks/adopt/preview?reveal=true` shows them to a caller with `containers.manage` on
the environment and writes a `reveal_adopt_preview` audit entry. Adopting a masked preview puts
the withheld values back from the containers, which also needs `containers.manage`; the
`adopt` audit entry then says so, at warning severity. McHarbor's own keys are never shown or
written.

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- One row per stack: name, status, services, images, description and created.
- A **managed** or **discovered** badge. Managed means McHarbor holds the Compose file; discovered means Compose labels were found on containers it did not deploy.
- The card view adds "3/4 services running" and live CPU, memory and network RX/TX for the stack as a whole.

## Managed and discovered

| Badge | What it is | Editing |
|---|---|---|
| **managed** | McHarbor holds the Compose file and deployed it | Full: edit the file, save, redeploy |
| **discovered** | Compose labels found on containers McHarbor did not deploy | Read-only: "Read-only (discovered stack)" |

A discovered stack is still operable: up, down, restart and logs all act on containers.
What McHarbor will not do is pretend to own a file it never had.

## The detail tabs

| Tab | Contents |
|---|---|
| **Overview** | Name, status, type, description, created, last updated, running-of-total services |
| **Services** | Per service: container id, image, ports, mounts, live CPU and memory, link to the container |
| **Logs** | Aggregated across services, with a filter |
| **Compose** | The file, editable on a managed stack. Credential-looking values under a service's `environment:` (map or list form) arrive as `••••••••`; `env_file:` references and `${VAR}` interpolations are shown as written. **Reveal values** calls `GET /api/stacks/{name}/compose?reveal=true`, which needs `stacks.manage` on the stack and writes a `reveal_compose` audit entry; McHarbor's own keys, and any value carrying the keys McHarbor runs with, stay masked. Masking also covers `build.args` and `labels` values, credentials inside `command`, `entrypoint` and `healthcheck.test` (`--password=••••••••`), top-level `configs.*.content`, and every document of a multi-document file; a `${VAR:-default}` counts as its default. A service is treated as McHarbor when its running container is, or by its name (`mcharbor`, `mcharbor-node-N`), image or compose identity. Saving a file that still shows `••••••••` keeps the stored value; a marker McHarbor cannot place (a new or renamed variable or service, an edited command around a masked credential, a value anywhere else) is refused with "The compose file still contains a masked value" and the file is left as it was. Keeping a value nobody may reveal while changing that service's `image`, `build`, `command`, `entrypoint`, `labels` or `healthcheck`, or reaching a restored value through a YAML alias outside `environment:`, is refused with "A masked value (••••••••) was kept for a service whose image, command, entrypoint or labels also changed" |
| **Environment** | Stack variables, values hidden until revealed. A credential-looking value (by the stored secret flag or by the shared classifier described in [Containers → Environment values](../containers/fields.md#environment-values)) arrives as `••••••••`; showing it calls `GET /api/stacks/{name}/env-vars?reveal=true`, which needs `stacks.manage` on the stack and writes a `reveal_env_vars` audit entry. A value that contains McHarbor's own secret, encryption key, database DSN or backup key stays masked for everyone. Saving a row that still shows `••••••••` keeps its stored value |
| **Layers** | Image layers across the stack, and the total size |
| **Webhooks** | Per-stack webhooks |

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Containers](../containers/README.md): the containers a stack creates
- [Git](../git/README.md) and [GitOps](../gitops/README.md): stacks that follow a repository
- [Store](../store/README.md): installing an app deploys a stack
