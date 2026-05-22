# Docker and Stacks

These routes target Docker environments and generally expect `?env=<environmentId>`.

## Docker Info

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/docker/info` | Returns Docker daemon system information. |

## Containers

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/containers/` | Lists containers. |
| POST | `/api/containers/` | Creates a container. |
| GET | `/api/containers/stats/summary` | Returns summarized metrics for many containers. |
| POST | `/api/containers/check-updates` | Checks whether running containers have newer image digests available. |
| GET | `/api/containers/{id}` | Inspects a container. |
| DELETE | `/api/containers/{id}` | Removes a container with the basic delete flow. |
| POST | `/api/containers/{id}/remove` | Extended removal with extra options. |
| POST | `/api/containers/{id}/start` | Starts a container. |
| POST | `/api/containers/{id}/stop` | Stops a container. |
| POST | `/api/containers/{id}/restart` | Restarts a container. |
| POST | `/api/containers/{id}/pause` | Pauses a container. |
| POST | `/api/containers/{id}/unpause` | Unpauses a container. |
| POST | `/api/containers/{id}/kill` | Sends a kill signal to a container. |
| POST | `/api/containers/{id}/update` | Updates container runtime limits. |
| POST | `/api/containers/{id}/recreate` | Recreates a container from a richer declarative payload. |
| POST | `/api/containers/{id}/network/connect` | Connects a container to a network. |
| POST | `/api/containers/{id}/network/disconnect` | Disconnects a container from a network. |
| GET | `/api/containers/{id}/logs` | Returns logs for a container. |
| GET | `/api/containers/{id}/stats` | Returns current stats for a container. |
| GET | `/api/containers/{id}/top` | Returns process information for a container. |
| GET | `/api/containers/{id}/files` | Lists files within a container path. |
| GET | `/api/containers/{id}/files/content` | Reads a file from the container. |
| PUT | `/api/containers/{id}/files/content` | Writes file content into the container. |
| DELETE | `/api/containers/{id}/files/content` | Deletes a file inside the container. |
| POST | `/api/containers/{id}/files/upload` | Uploads a file into the container. |
| POST | `/api/containers/{id}/files/directory` | Creates a directory inside the container. |
| POST | `/api/containers/{id}/files/rename` | Renames or moves a file path. |
| POST | `/api/containers/{id}/files/chmod` | Changes file permissions inside the container. |
| GET | `/api/containers/{id}/services` | Detects OS-level services inside the container. |
| POST | `/api/containers/{id}/shells` | Detects interactive shells for terminal access. |

Representative container create body:

```json
{
  "name": "nginx-demo",
  "image": "nginx:stable",
  "env": [
    "TZ=UTC"
  ],
  "cmd": [],
  "labels": {
    "com.example.managed-by": "mcharbor"
  },
  "tty": false,
  "openStdin": false
}
```

## Images

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/images/` | Lists images. |
| POST | `/api/images/` | Pulls an image. |
| POST | `/api/images/prune` | Prunes unused images. |
| POST | `/api/images/import` | Imports an image archive. |
| GET | `/api/images/{id}` | Inspects an image. |
| DELETE | `/api/images/{id}` | Deletes an image. |
| POST | `/api/images/{id}/tag` | Adds a tag to an image. |
| GET | `/api/images/{id}/history` | Returns layer history. |
| GET | `/api/images/{id}/export` | Exports an image archive. |

## Volumes

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/volumes/` | Lists volumes. |
| POST | `/api/volumes/` | Creates a volume. |
| POST | `/api/volumes/prune` | Prunes unused volumes. |
| GET | `/api/volumes/{name}` | Inspects a volume. |
| DELETE | `/api/volumes/{name}` | Deletes a volume. |

## Networks

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/networks/` | Lists networks. |
| POST | `/api/networks/` | Creates a network. |
| GET | `/api/networks/{id}` | Inspects a network. |
| DELETE | `/api/networks/{id}` | Deletes a network. |
| POST | `/api/networks/{id}/connect` | Connects a container to a network. |
| POST | `/api/networks/{id}/disconnect` | Disconnects a container from a network. |

## Compose Stacks

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/stacks/` | Lists stacks. |
| POST | `/api/stacks/` | Creates a managed stack. |
| POST | `/api/stacks/check-updates` | Checks image updates across stacks. |
| POST | `/api/stacks/adopt/preview` | Generates a compose preview for adoption. |
| POST | `/api/stacks/adopt` | Adopts an existing workload into stack management. |
| GET | `/api/stacks/{name}` | Returns stack detail. |
| PUT | `/api/stacks/{name}` | Updates a stack definition. |
| DELETE | `/api/stacks/{name}` | Deletes a stack definition. |
| POST | `/api/stacks/{name}/up` | Starts or applies the stack. |
| POST | `/api/stacks/{name}/stop` | Stops the stack. |
| POST | `/api/stacks/{name}/down` | Brings the stack down. |
| POST | `/api/stacks/{name}/restart` | Restarts the stack. |
| POST | `/api/stacks/{name}/update` | Runs the managed update flow. |
| POST | `/api/stacks/{name}/reinstall` | Reinstalls the stack. |
| GET | `/api/stacks/{name}/compose` | Returns the compose source. |
| GET | `/api/stacks/{name}/logs` | Returns stack logs. |
| GET | `/api/stacks/{name}/containers` | Lists stack containers. |
| GET | `/api/stacks/{name}/env-vars` | Returns stack environment variables. |
| PUT | `/api/stacks/{name}/env-vars` | Updates stack environment variables. |
| POST | `/api/stacks/{name}/prune` | Prunes orphaned stack resources. |
| GET | `/api/stacks/{name}/webhooks` | Lists stack-scoped webhooks. |
| POST | `/api/stacks/{name}/webhooks` | Creates a stack-scoped webhook. |
| PUT | `/api/stacks/{name}/webhooks/{id}` | Updates a stack-scoped webhook. |
| DELETE | `/api/stacks/{name}/webhooks/{id}` | Deletes a stack-scoped webhook. |
| POST | `/api/stacks/{name}/webhooks/{id}/test` | Tests a stack-scoped webhook. |

Representative stack create body:

```json
{
  "name": "whoami",
  "compose": "services:\n  whoami:\n    image: traefik/whoami\n    ports:\n      - \"8080:80\"\n",
  "envVars": {
    "TZ": "UTC"
  },
  "description": "Sample managed stack",
  "environmentId": "env_123",
  "autoStart": true
}
```
