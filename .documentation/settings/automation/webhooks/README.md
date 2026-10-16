# Webhooks

**Settings → Automation → Webhooks** · `?tab=webhooks`

Outbound HTTP callbacks fired on McHarbor events.

## Permissions

| Key | What it unlocks |
|---|---|
| `webhooks.view` | See webhooks |
| `webhooks.manage` | Create and change them |
| `webhooks.delete` | Delete one |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- A URL, and the events that POST to it.
- Whether each webhook is active, and a test that delivers a real request.

## Choosing events

Event names follow the `resource.action` shape used throughout McHarbor:
`container.start`, `container.stop`. The audit action browser on the
[Audit](../../../screens/audit/README.md) screen is the most reliable place to see the
canonical names, since it lists the catalogue rather than asking you to guess.

Subscribe narrowly. A webhook on every container event from a busy host is a
denial-of-service attack on your own receiver.

## Webhooks elsewhere

| Surface | Direction | Where |
|---|---|---|
| **Stack webhooks** | Outbound, per stack, with an optional HMAC secret | [Stacks](../../../screens/stacks/README.md) |
| **GitOps PR webhook** | **Inbound**: your forge calls McHarbor | [GitOps](../../../screens/gitops/README.md) |
| **Workflow webhook trigger / response nodes** | Inbound and outbound | [Workflows](../../../screens/workflows/README.md) |

Stack webhooks are the ones with a signing secret today; if the receiver needs to verify
origin, that is the surface that offers it.

## Delivery is fire and forget

McHarbor posts and moves on. There is no retry queue on this surface, so for anything that
must not be missed use an alert rule with a communication channel, which tracks delivery,
or a workflow, which records runs and errors.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Schedules](../schedules/README.md): the time-driven counterpart
- [Stacks](../../../screens/stacks/README.md): per-stack webhooks with a secret
- [Audit](../../../screens/audit/README.md): the canonical action names
