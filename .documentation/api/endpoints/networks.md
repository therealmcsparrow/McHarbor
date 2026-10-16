# Networks

7 endpoints.

| Key | Covers |
|---|---|
| `networks.view` | List and inspect |
| `networks.manage` | Create, connect, disconnect; list host interfaces for macvlan and ipvlan |
| `networks.delete` | Remove |

## Things worth knowing first

**Docker's built-in networks cannot be removed.** `bridge`, `host` and `none` return
`err.network.built_in`.

**There are two ways to attach a container to a network**, and they differ in what they can
express. `POST /api/networks/{id}/connect` takes a container name and nothing else.
`POST /api/containers/{id}/network/connect` additionally takes aliases, a fixed IPv4 or IPv6
address, a MAC address, and a `reconnect` flag. Use that one when the attachment needs
detail.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/networks` | `networks.view` | Returns all networks | — |
| `POST` | `/api/networks` | `networks.manage` | Creates a new network | — |
| `GET` | `/api/networks/host-interfaces` | `networks.manage` | Returns the links that can parent a macvlan or ipvlan network on the environment; an environment that cannot be enumerated is an empty list with `agentAvailable: false` | — |
| `DELETE` | `/api/networks/{id}` | `networks.delete` | Removes a network | `{id}` |
| `GET` | `/api/networks/{id}` | `networks.view` | Returns detailed network information | `{id}` |
| `POST` | `/api/networks/{id}/connect` | `networks.manage` | Connects a container to a network | `{id}` |
| `POST` | `/api/networks/{id}/disconnect` | `networks.manage` | Disconnects a container from a network | `{id}` |


## Request bodies

**`POST` `/api/networks`**: `name`, `driver`?, `internal`?, `attachable`?, `ipam`?, `options`?, `labels`?

**`POST` `/api/networks/{id}/connect`**: `container`

**`POST` `/api/networks/{id}/disconnect`**: `container`, `force`?

## Related

- [Containers](./containers.md): the richer per-container attach
- [Networks screen](../../screens/networks/README.md)
