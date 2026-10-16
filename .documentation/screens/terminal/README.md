# Terminal

`/terminal`

An interactive shell inside a running container.

## Permissions

| Key | What it unlocks |
|---|---|
| `terminal.access` | Open an interactive shell inside a running container |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- A container selector, a shell selector, and the connection state: Connecting, Connected or Disconnected.
- The shell list comes from detection: McHarbor checks what the image actually has rather than assuming `bash`.

## Why it is a separate permission

`terminal.access` is not part of `containers.manage` because a shell is a different thing
from a lifecycle action: inside the container you are whatever user the image runs as, with
that container’s mounts and network.

## Through an agent

For a container on an agent environment the session runs over dedicated WebSocket
messages rather than a proxied Docker call, because the Docker SDK’s non-detached exec bypasses
the agent transport entirely. This needs **agent v1.1.0 or newer**. An older agent can
still list and detect shells, because detection uses a detached exec, but the interactive
session will not start.

## Run one command instead

A container’s **Exec** tab runs a single command with a chosen user and working directory
and shows its output. It is the better tool when you want one answer rather than a
session, and it leaves a cleaner audit trail.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Containers](../containers/README.md): the per-container Terminal and Exec tabs
- [Environments](../environments/README.md): agent connections and their versions
- [Audit](../audit/README.md): the trail terminal access lands in
