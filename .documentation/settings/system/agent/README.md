# Agent

**Settings → System → Agent** · `?tab=agent`

Defaults and diagnostics for the remote agent transport. "These settings affect event
collection, heartbeat pings, metrics, and request timeouts for **all** agent-connected
environments."

## Permissions

| Key | What it unlocks |
|---|---|
| `settings.manage` | Change transport defaults and run diagnostics; this section has no dedicated key of its own |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- How Docker events are collected from agent environments, and how often.
- Keepalive pings, request timeouts, whether agent metrics are collected at all, and how
  often agents that can push their stats do so.
- An agent-to-agent direct transfer test.

## The stream warning is the important part

> Streaming events monopolizes the agent WebSocket connection, which can block other
> requests like container inspect and stats. Only use this if you need real-time events and
> have few containers.

One agent has one WebSocket. A stream that holds it means everything else queues behind it,
which shows up as a slow, apparently unrelated interface rather than as an event problem.
Poll unless you have a specific reason and a small host.

## Metrics: pushed by current agents, pulled from older ones

**Metrics collection** is on by default. An agent at 2026.09.01 build 202609251653 or later
pushes its container stats once per **Stats push interval** (default 10 s, 5 to 300 s), which
costs one message per interval however many containers the host runs. An older agent is
pulled instead, and there the page's own description still holds: "One-shot stats take ~2s
per container through the agent transport." The switch stays because it is the one way to
stop both at once: with it off, pushing agents are told to stop.

Each environment says which path it is on; see
[Agents → Container stats](../../../agents/stats/README.md).

## The transfer test

Moving a container between two agent hosts is fastest when the source agent pushes straight
to the target’s listener on port 8788 rather than relaying through McHarbor. Whether that
works depends on the network between the two hosts, which McHarbor cannot know without
trying.

The result reports both agents and their versions, the **advertised URL** and the **probe
URL**, the HTTP status and duration, a **responder marker** confirming the answer came from
the expected agent, the prepared receiver and its expiry, and an interpretation under "What
this means".

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Environments](../../../screens/environments/README.md): creating agent environments and their tokens
- [Updates → Agents](../../updates/agents/README.md): keeping agent versions current
- [Getting started → Configuration](../../../getting-started/configuration/README.md): the agent’s own environment variables
