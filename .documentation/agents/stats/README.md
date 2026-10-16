# Container stats: push and pull

How per-container CPU, memory, network and block I/O reach McHarbor from an environment,
what each path costs, and how to tell which one an environment is on.

The same numbers feed the containers table's live columns, the top-consumers and heatmap
widgets, metrics history and every chart built on it.

## The four paths

| Path | Shown as | Applies to | Cost |
|---|---|---|---|
| **Push** | "Pushed by the agent every N s" | An agent at 2026.09.01 build **202609251653** or later | One `stats_push` message per interval per agent, and nothing per container over the link |
| **Pull** | "Pulled one container at a time" | An older agent, or a capable one whose pushes have not started | One stats request per running container, about **2 s each** through the agent, at most 8 at once |
| **Direct** | "Read directly from the Docker connection" | Socket, TCP, TLS and SSH environments | One stats request per running container against the daemon itself, at most 32 at once |
| **Off** | "Not collected" | Agent metrics are turned off, or the agent is not connected | None |

Pull is permanent for an agent that cannot push, not a transitional state. A native agent
updates itself only from a published release, so an agent that nobody updates stays on pull
for good and keeps working.

## Where to see it

**Environments → *an environment* → Overview** carries a **Container stats** card with the
path, and underneath it either when the last batch arrived or why the environment is not on
push:

| Reason | What it means | What to do |
|---|---|---|
| "Agent *version* (build *build*) cannot push stats. Build *build* of *version* or newer can." | The agent predates the pusher | Install a current agent binary on that host; see [Updating agents](#updating-an-agent-for-push) |
| "The agent was asked to push and its first batch has not arrived yet." | Normal for one interval after the agent connects | Wait one interval. If it persists past two minutes, McHarbor falls back to pulling on its own |
| "The agent is not connected." | Nothing reaches McHarbor | [Agent troubleshooting](../troubleshooting/README.md) |
| "Agent metrics are turned off in Settings → System → Agent." | The central switch is off | Turn **Metrics collection** back on |

The card needs `host.view`. The same answer is available as
`GET /api/metrics/stats-path?env=<id>` ([Host endpoints](../../api/endpoints/host.md)).

## How push works

1. When a capable agent connects, McHarbor sends it `stats_subscribe` with the interval.
2. The agent opens Docker's own streaming stats for every running container on its host, and
   reconciles that set against the running containers once per interval.
3. Once per interval it sends one batch (the latest sample of every container) and waits
   for the acknowledgement before it sends the next. Exactly one batch is in flight at a time.
4. McHarbor applies the batch to the live view at once and writes it to metrics history every
   **30 seconds**, the same raw cadence the collector uses for every other environment.

**Nothing lost, nothing counted twice.** A batch that is not acknowledged (the connection
dropped, or McHarbor could not write it) goes into the agent's offline buffer and is replayed
on the next connection through the same acknowledged path the offline buffer always uses. The
replay is deduplicated: a container sample is keyed on its container and timestamp, and a host
row that already exists for that instant is not written again. The buffer needs
`MCHARBOR_OFFLINE_BUFFER` left at its default of `true`
([Configuration](../configuration/README.md)).

**Backpressure.** Every acknowledgement carries the interval the agent should use next.
McHarbor keeps the whole fleet under ten batches a second, so fifty agents at 5 s are
accepted as they are, and a hundred are all told to slow to 10 s. The interval is always
between 5 and 300 seconds.

**Turning metrics off stops it.** With **Metrics collection** off, the next acknowledgement
tells the agent to stop, and its stats streams close.

## The interval

**Settings → System → Agent → Stats push interval (seconds)**, 5 to 300, default **10**, needs
`settings.manage`. It sets how fresh the live view is for pushed environments; history is
written every 30 s regardless. McHarbor may use a longer interval than the one set here when
many agents push at once (see *Backpressure* above).

## Every reader shares one sample

Whatever the path, one sample per environment is shared by every reader: the containers page
in every open tab, the metrics endpoints and the background collector. Twenty tabs on the same
300-container environment cost one sweep, not twenty.

For a pulled environment the page is answered from the last sample at once while a fresh one is
taken in the background. Only the very first read waits, and for at most three seconds. On an
older agent with hundreds of containers that means the live columns can be empty for the first
minute after McHarbor starts, while the first sweep finishes.

## Updating an agent for push

A native agent does not fetch a rebuilt binary from this McHarbor. Copy the current binary to
the host and restart the service:

```bash
curl -fsSLo /usr/local/bin/mcharbor-agent "$MCHARBOR_URL/api/agent/binary/linux/amd64"
chmod +x /usr/local/bin/mcharbor-agent
systemctl restart mcharbor-agent
```

Once it reconnects, the **Container stats** card changes to *Pushed by the agent* within one
interval. [Operations](../operations/README.md#updating-agents) covers updates in general.

## What goes wrong

| Symptom | Cause | Fix |
|---|---|---|
| An agent environment shows gaps in its history, or none at all | It is on pull and has more containers than a 60 s sweep can cover at 2 s each (about 240) | Update the agent so it pushes |
| The card stays on "has not arrived yet" | The pusher on the agent is not delivering | McHarbor pulls after two minutes on its own; check the agent log for `stats push` lines, and restart the agent |
| Live columns on the containers page are empty right after McHarbor starts | A pulled environment's first sweep is still running | Wait for it; the next poll after it finishes fills them |
| Stats stopped for every agent at once | **Metrics collection** was turned off | Turn it back on; agents are subscribed again within one collector tick |

## Related

- [Protocol](../protocol/README.md#container-stats): the three messages
- [Configuration](../configuration/README.md): the offline buffer the push path relies on
- [Settings → System → Agent](../../settings/system/agent/README.md): metrics switch and interval
