# Stacks: Actions

Everything you can do to a stack (deploy, up, down, stop, restart, edit, remove, prune
services and wire up a webhook) and the permission each one needs. Prune services is the
one worth knowing: deleting a service from the Compose file leaves its container running.

| Action | Gated by | What it does |
|---|---|---|
| **Deploy stack** | `stacks.manage` | Name, optional description, and the `docker-compose.yml`. Validated before deployment |
| **Up** | `stacks.manage` | Deploy or start the stack |
| **Down** | `stacks.manage` | Stop and remove its containers |
| **Stop** | `stacks.manage` | Stop without removing |
| **Restart** | `stacks.manage` | Restart its services |
| **Edit** | `stacks.manage` | Open and save the Compose file, which redeploys |
| **Remove** | `stacks.delete` | Delete the stack |
| **Prune services** | `stacks.manage` | Remove containers for services no longer in the Compose file |
| **Add webhook** | `stacks.manage` | URL, optional HMAC secret, and the events that fire it |
| **Test webhook** | `stacks.manage` | Delivers a real request and reports success or failure |

## Prune services is the one to know about

Delete a service from the Compose file and redeploy, and its container is left behind:
running, unmanaged, and absent from the file. **Prune services** is what clears those.

## Webhook events

A stack webhook fires on **Up**, **Down**, **Stop**, **Restart**, **Deploy** or
**Update**. This is the one webhook surface in the product with a signing secret, so it is
the one to use when a receiver must verify origin.

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Stacks](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
