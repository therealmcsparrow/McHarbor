# Stacks

29 endpoints. A stack is a Compose project McHarbor manages: the file, the deployment, the
containers it produced, and the webhooks that redeploy it.

Stacks are addressed by **name**, not by id. That is also why they get their own permission
scope: `RequireStackPermission` reads the stack name straight out of the path, so a role can
be granted on one stack rather than the whole environment.

| Key | Covers |
|---|---|
| `stacks.view` | List, inspect, compose file, containers, logs, env vars, webhooks |
| `stacks.manage` | Create, update, deploy, stop, restart, prune, adopt, link, webhooks |
| `stacks.delete` | Remove the stack entirely |
| `users.manage` | Reassign a stack's owner. This is deliberately an admin key, not a stack key |

## Things worth knowing first

**Managed and discovered stacks are both visible.** A discovered stack is one Docker already
runs that McHarbor did not create. `POST /api/stacks/adopt` takes it over, and
`POST /api/stacks/adopt/preview` generates the `docker-compose.yml` it would write, so the
operator can read it before agreeing.

**Deploy, stop and down are three different things.** `up` deploys, `stop` leaves the
containers in place as exited, `down` removes them, and `?volumes=true` on `down` takes
their volumes too. `prune` removes containers for services that are no longer in the file.

**`reinstall` versus `update`.** `reinstall` force-recreates at the current images;
`update` pulls the latest images first and then redeploys.

**The compose file is the source of truth.** Editing a container that belongs to a stack is
overwritten by the next deploy. The interface warns about this, and the API does not stop
you.

**A container can be linked to a stack by hand** when the link was lost (a recreate outside
McHarbor, for instance). `GET`/`POST /api/stacks/links` and
`DELETE /api/stacks/links/{containerId}` manage that mapping.

**Stack webhooks are per-stack**, separate from the global webhook endpoints in
[Workflows](./workflows.md). They live under `/api/stacks/{name}/webhooks`.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/stacks` | `stacks.view` | Returns all stacks with live status | — |
| `POST` | `/api/stacks` | `stacks.manage` | Deploys a new Compose stack | — |
| `POST` | `/api/stacks/adopt` | `stacks.manage` | Takes over management of a discovered stack or standalone container | — |
| `POST` | `/api/stacks/adopt/preview` | `stacks.manage` | Generates a docker-compose.yml preview from a discovered stack or standalone container | — |
| `POST` | `/api/stacks/check-updates` | `stacks.view` | Checks for available image updates across stack services | — |
| `GET` | `/api/stacks/links` | `stacks.view` | Returns the manual stack link for a container | `?containerId=` |
| `POST` | `/api/stacks/links` | `stacks.manage` | Creates or replaces a manual container-to-stack link | — |
| `DELETE` | `/api/stacks/links/{containerId}` | `stacks.manage` | Removes a manual container-to-stack link | `{containerId}` |
| `DELETE` | `/api/stacks/{name}` | `stacks.delete` | Removes a stack entirely (down containers + DB + files) | `{name}` |
| `GET` | `/api/stacks/{name}` | `stacks.view` | Returns a single stack by name, supporting both managed and discovered | `{name}` |
| `PUT` | `/api/stacks/{name}` | `stacks.manage` | Updates the compose content or description of a stack | `{name}` |
| `GET` | `/api/stacks/{name}/compose` | `stacks.view` | Returns the raw compose file content | `{name}` |
| `GET` | `/api/stacks/{name}/containers` | `stacks.view` | Returns container details for all containers in a stack | `{name}` |
| `POST` | `/api/stacks/{name}/down` | `stacks.manage` | Stops and removes all containers in a stack | `{name}` |
| `GET` | `/api/stacks/{name}/env-vars` | `stacks.view` | Returns stored environment variables for a managed stack | `{name}` |
| `PUT` | `/api/stacks/{name}/env-vars` | `stacks.manage` | Updates environment variables for a managed stack | `{name}` |
| `GET` | `/api/stacks/{name}/logs` | `stacks.view` | Returns combined logs from all containers in a stack | `{name}`; `?tail=` |
| `PATCH` | `/api/stacks/{name}/owner` | `users.manage` | Updates the owner of a stack | `{name}` |
| `POST` | `/api/stacks/{name}/prune` | `stacks.manage` | Removes containers for services no longer in the compose file | `{name}` |
| `POST` | `/api/stacks/{name}/reinstall` | `stacks.manage` | Force recreates a managed stack without pulling new images | `{name}` |
| `POST` | `/api/stacks/{name}/restart` | `stacks.manage` | Restarts all containers in a stack | `{name}` |
| `POST` | `/api/stacks/{name}/stop` | `stacks.manage` | Stops all containers in a stack (containers remain as "exited") | `{name}` |
| `POST` | `/api/stacks/{name}/up` | `stacks.manage` | Starts the stack | `{name}` |
| `POST` | `/api/stacks/{name}/update` | `stacks.manage` | Pulls latest images and redeploys a managed stack | `{name}` |
| `GET` | `/api/stacks/{name}/webhooks` | `stacks.view` | Returns all webhooks for a stack | `{name}` |
| `POST` | `/api/stacks/{name}/webhooks` | `stacks.manage` | Creates a new webhook for a stack | `{name}` |
| `DELETE` | `/api/stacks/{name}/webhooks/{id}` | `stacks.manage` | Removes a webhook | `{id}`, `{name}` |
| `PUT` | `/api/stacks/{name}/webhooks/{id}` | `stacks.manage` | Updates an existing webhook | `{id}`, `{name}` |
| `POST` | `/api/stacks/{name}/webhooks/{id}/test` | `stacks.manage` | Sends a test payload to a webhook | `{id}`, `{name}` |


## Request bodies

**`POST` `/api/stacks`**: `name`, `compose`, `envVars`?, `description`?, `environmentId`?, `autoStart`

**`POST` `/api/stacks/adopt`**: `name`, `compose`, `description`?, `containerId`?

**`POST` `/api/stacks/adopt/preview`**: `stackName`, `containerId`?

**`POST` `/api/stacks/links`**: `containerId`, `stackName`, `serviceName`?

**`PUT` `/api/stacks/{name}`**: `compose`?, `description`?, `envVars`?, `name`?

**`POST` `/api/stacks/{name}/webhooks`**: `url`, `secret`?, `events`

**`PUT` `/api/stacks/{name}/webhooks/{id}`**: `url`?, `secret`?, `events`?, `isActive`?

## Related

- [Containers](./containers.md): the containers a stack produced
- [GitOps](./gitops.md): deploying a stack from a repository
- [Stacks screen](../../screens/stacks/README.md)
