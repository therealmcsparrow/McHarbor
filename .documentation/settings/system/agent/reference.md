# Agent: Reference

| Setting | Default | What it does |
|---|---|---|
| **Event collection mode** | Poll (recommended) | Poll, or Stream, which monopolises the agent WebSocket |
| **Event poll interval (seconds)** | — | "Lower values mean more frequent updates but higher load" |
| **Ping interval (seconds)** | — | Keepalive pings to connected agents |
| **Metrics collection** | On | Container stats for agent environments: pushed by current agents, pulled at ~2s per container from older ones |
| **Stats push interval (seconds)** | 10 | Shown while metrics collection is on. "How often agents that support it push container stats. McHarbor stretches the interval for every agent when too many push at once." 5 to 300 |
| **Request timeout (seconds)** | — | Maximum wait for an agent response |

Pings are what keep a NAT or proxy from silently dropping an idle connection. Too long and
an agent looks connected while its socket is dead; too short and you spend round trips for
nothing.

Raising the request timeout helps an agent across a slow link. Lowering it makes a
struggling agent fail faster rather than fixing it.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| The whole interface is slow when one agent is busy | Event mode is set to Stream | Switch to Poll; one agent has one WebSocket |
| Agent charts are empty | Metrics collection is off for agent environments | Turn it on |
| An agent host has gaps in its history | It is on the pull path with too many containers for one sweep | Update that agent so it pushes; see [Container stats](../../../agents/stats/README.md) |
| Saving is refused | Stats push interval outside 5 to 300 | Pick a value in range |
| An agent shows connected but never responds | The socket died and pings are too infrequent | Lower the ping interval |
| Transfers are slow between two agent hosts | They are relaying through McHarbor | Run the agent-to-agent transfer test; it needs at least two connected agents |

## Related

- [Agent](./README.md): what the section is for
- [Settings](../../README.md): the other groups
