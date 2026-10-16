# Networks

`/networks` · `/networks/:id`

Docker networks on the selected environment, which containers are attached, and how they
are addressed.

## Permissions

| Key | What it unlocks |
|---|---|
| `networks.view` | See networks and their detail |
| `networks.manage` | Create, connect and disconnect containers |
| `networks.delete` | Remove a network |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- One row per network with a **Used**, **Unused** or **Built-in** badge.
- Subnet, gateway, driver, scope, whether it is internal, and how many containers are attached.

## Built-in networks are protected

`bridge`, `host` and `none` cannot be removed or modified. The attempt is refused with an
explanation rather than an error.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Containers](../containers/README.md): the network tab, ports and DNS
- [Stacks](../stacks/README.md): networks a Compose file creates and owns
