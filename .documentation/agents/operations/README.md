# Operating Agents

Day-to-day: tokens, versions, seats, diagnostics.

## The API surface

| Route | Gated by |
|---|---|
| `GET /api/agent/ws` | The agent's own authenticated WebSocket (token, not session) |
| `GET /api/agent/install/{token}` | The one-time install script. Single-use, 24-hour expiry |
| `GET /api/agents` | Session |
| `GET /api/agents/{id}/status` | Session |
| `GET /api/agents/{id}/logs` | `environments.view` |
| `GET /api/agents/{id}/health/history` | `environments.view` |
| `POST /api/agents/{id}/regenerate-token` | `environments.manage` |
| `POST /api/agents/{id}/deploy` | `environments.manage` |
| `POST /api/agents/{id}/install-token` | `environments.manage` |
| `POST /api/agents/direct-transfer-test` | `environments.manage` |

Everything an operator does to an agent is `environments.manage`. There is no separate agent
permission; an agent *is* an environment as far as RBAC is concerned.

## Health and uptime

An agent has two separate observables, and conflating them is how a sick link goes unnoticed:

- **Status**: `connected` or `disconnected`. Whether the outbound WebSocket exists.
- **Health**: `healthy`, `degraded`, `unhealthy` or `unknown`. Whether it is *working*.

An open socket proves nothing on its own, so health is derived from the keepalive rather than
from the socket:

| Health | Meaning |
|---|---|
| `healthy` | Connected, answered its last ping, round trip under 2000 ms |
| `degraded` | Connected, but no pong for two ping intervals **or** a round trip over 2000 ms |
| `unhealthy` | Not connected, having connected at some point before |
| `unknown` | An agent environment that has never connected at all |

The 2000 ms threshold is not arbitrary: a one-shot container stats call through an agent
already costs roughly two seconds on a *healthy* link (the cost an agent that cannot
[push its stats](../stats/README.md) still pays), so a keepalive round trip past that
means ordinary Docker operations are into timeout territory. The ping interval itself is
[Settings → Agents](../configuration/README.md).

`degraded` is the state that did not exist before, and the one worth watching. An agent can be
connected and degraded at the same time, which is what a slow or saturated link looks
like from McHarbor's side, and what a connected/disconnected badge could never show.

### The graphs

Environment detail → **Overview** carries two cards for agent environments:

- **Agent health**: one block per 30-second sample of the keepalive, plus a chart of the
  ping/pong round trip underneath. A strip that is entirely green while the latency line
  climbs is an agent that is about to start timing out.
- **Agent uptime**: one block per sample of the connection, with an uptime percentage and an
  **outage count**. Both are shown because neither is enough alone: an agent that dropped once
  for an hour and one that flapped sixty times produce the same percentage, and only the
  second is a networking problem.

Samples are written every 30 seconds and kept for **7 days**. Collection costs no agent
traffic at all: every fact recorded is already stamped on the pooled connection by the
keepalive loop, so the sampler reads local memory rather than the remote host. In an HA
cluster only the leader samples, because an agent holds its socket to exactly one node and a
follower would record a fabricated outage.

## Tokens

- Issued when the environment is created, and **shown once**.
- Format `agt_` followed by 64 hex characters.
- **Regenerating invalidates the previous token immediately.** The running agent will be
  rejected on its next reconnect, so plan to reinstall it in the same sitting.
- Install tokens are a separate, shorter-lived thing: single-use, 24-hour expiry, and they
  exist only to fetch the install script.

Treat the token like a credential for that host, because within its environment it is one.
It appears in the systemd unit file and in the container's environment, so both inherit the
same care.

## Updating agents

An agent speaks a protocol version, and some features have hard floors. Keeping them current
is [Settings → Updates → Agents](../../settings/updates/agents/README.md):

- **Update remote agents automatically**: after McHarbor updates, push the matching binary
  to every connected, non-excluded agent.
- **Update agents now**: the same sweep on demand, reporting
  "*4 started, 9 already current, 1 unsupported, 2 excluded*".
- **Exceptions**: per agent, skipping both the automatic and the manual sweep. Useful for a
  host with its own change window, or one whose agent is managed by configuration management.

What an agent moves to is the **target release** in McHarbor's own
[release store](../releases/README.md), or, with no target, the agent this McHarbor was
built with. McHarbor no longer asks GitHub which version is newest when it sweeps: a version
it has not stored is one it could not deliver.

What happens on the agent: a native binary downloads its replacement **from McHarbor**,
checks the digest McHarbor sent, swaps itself and re-execs; it refuses any URL that does not
point at the McHarbor it is connected to. A container downloads the target's image tarball
from McHarbor with its agent token, loads it through its own Docker API and starts a
replacement that removes its predecessor once running. It never pulls from a registry. The
response is written before the process goes away, so McHarbor learns the outcome either way.

A container agent older than `2026.09.01` build `202609271330` cannot load an image from
McHarbor; the sweep sends it the request it always understood and it still pulls from a
registry itself. Reinstall it once from McHarbor to move it past that.

*Failed* with "McHarbor holds no agent for linux/arm64" means nothing McHarbor serves covers
that platform: download a release that does, or build one. *Unsupported* means an agent too
old to accept the push at all. Both need a person.

## Licence seats

An agent does not claim a seat; its **environment** does, counted by the McHarbor that manages
it, the same as a direct environment.

- Recreating an agent, or moving its data directory, changes nothing about seats.
- The agent reports nothing to the licence server. `MCHARBOR_LICENSE_KEY` is ignored with a
  warning in the agent log, and no path in the agent binary is gated on a licence.

Nothing is ever switched off for being over licence (see
[Settings → System → Licence](../../settings/system/license/README.md)).

## Diagnostics

### From McHarbor

The environment's detail page shows hostname, OS, architecture, agent version, last seen, and
the agent's own log, which is the fastest route when you have no shell on the host. Host
stats, host logs and a host terminal are in the Console and Host groups of the same page.

### The agent-to-agent transfer test

[Settings → System → Agent](../../settings/system/agent/README.md) → pick a source and a
target agent. The result reports both agents and their versions, the **advertised URL** and
the **probe URL** actually tried, the HTTP status and duration, a **responder marker**
confirming the answer came from the expected agent rather than something else on that
address, the prepared receiver and its expiry, and an interpretation under "What this means".

It needs at least two connected agents. A failure is not fatal (transfers fall back to the
relay), but it tells you which path a large move will take.

### On the host

A container install writes to Docker's log driver, so `docker logs` is the whole story:

```bash
docker logs -f mcharbor-agent
```

A **native** (binary) install writes to stdout, and where that lands depends on what started
it.

#### Started by systemd: the journal

This is what the install script sets up whenever the host has `systemctl`. The unit is
`mcharbor-agent.service`, it has no `StandardOutput=` of its own, so everything the agent
prints is in the journal under that unit:

```bash
journalctl -u mcharbor-agent -f                 # follow
journalctl -u mcharbor-agent -n 200 --no-pager  # last 200 lines
journalctl -u mcharbor-agent --since "1 hour ago"
journalctl -u mcharbor-agent -b                 # this boot only
journalctl -u mcharbor-agent -p warning         # warnings and worse
journalctl -u mcharbor-agent --since today -o cat > /tmp/agent.log
```

`systemctl status mcharbor-agent` gives the state, the PID, the uptime and the last handful
of lines in one screen. It is the fastest first look when an agent shows `disconnected` in
McHarbor.

Two things worth knowing before you conclude the log is empty. The journal is root-readable
by default, so a non-root user sees nothing unless they are in `systemd-journal`; use
`sudo`. And if the journal is volatile on that host (`Storage=volatile` in
`journald.conf`, common on minimal images), nothing survives a reboot and `-b -1` will not
find the previous boot either.

#### Started without systemd: the log file

On a host with no `systemctl` the install script runs the binary under `nohup` and redirects
both streams to one file:

```bash
tail -f /var/log/mcharbor-agent.log
grep -i error /var/log/mcharbor-agent.log | tail -50
```

Nothing rotates that file. It is append-only for the life of the process, so on a long-lived
host give it a `logrotate` entry with `copytruncate`. The agent holds the descriptor open
and a plain rename would leave it writing to an unlinked inode.

If the host was wired into OpenRC, runit or an `/etc/rc.local` line by hand instead, the log
is wherever that supervisor was told to put it: `/var/log/mcharbor-agent/current` for a
runit service, `/var/log/messages` for a syslog-forwarding setup.

#### macOS and Windows

McHarbor ships no service installer for either, so the log is wherever the plist or the
service wrapper was pointed. A launchd plist with `StandardOutPath` /
`StandardErrorPath` is read with `tail -f` on those paths, or `log stream --predicate
'process == "mcharbor-agent"'` when it has none. On Windows, `nssm` writes the stdout file
configured with `AppStdout`, and a binary run in a console window simply prints there.

#### What the lines look like

The agent logs through `slog`'s text handler, so each line is `time=… level=… msg=…` with
`key=value` attributes after it, greppable as-is with no JSON parsing needed. A healthy start
reads:

```
starting mcharbor-agent version=…
connecting url=wss://mcharbor.example.com/api/agent/ws
authenticated envId=…
```

The connect line never carries `?token=`; the agent strips the query string before writing
it, because that same log is served into McHarbor's own log viewer.

A disconnect logs at `warn` with `attempt=` and `backoff=` on it. The ladder is exponential
from 1s to a 60s cap with ±50% jitter, and it **resets to the bottom after any session that
authenticated**, so a climbing `attempt=` counter means consecutive failures, not a long
uptime.

#### Turning the detail up

`LOG_LEVEL=debug` exposes the handshake, the reconnect backoff and every proxied call. On a
systemd host, set it without editing the unit the installer wrote:

```bash
systemctl edit mcharbor-agent
# [Service]
# Environment=LOG_LEVEL=debug
systemctl restart mcharbor-agent
journalctl -u mcharbor-agent -f
```

Remove the drop-in with `systemctl revert mcharbor-agent` when you are done. Debug logs
every proxied call, which on a busy host is a great deal of journal.

#### The offline buffer

While McHarbor is unreachable the agent also keeps its own log at info and above in
`$MCHARBOR_DATA_DIR/offline.db` (default `/var/lib/mcharbor-agent/offline.db`), and replays
it on the next connection. That is the record of what happened during an outage, and it is
readable with any `sqlite3` on the host, but the journal or the log file above is the live
view, and the buffer is cleared once it has been replayed.

### Restarting a native agent

A restart is safe at any time: the agent holds no state McHarbor needs, and it reconnects on
its own with the backoff above. Running containers on the host are untouched: the agent
manages Docker, it does not host it.

```bash
systemctl restart mcharbor-agent        # systemd host
systemctl status mcharbor-agent         # confirm it came back
journalctl -u mcharbor-agent -f         # watch it re-authenticate
```

`systemctl stop` and `systemctl start` are the same thing in two steps; use them when you
want the host visibly disconnected in between. `Restart=always` with `RestartSec=5` is in the
unit, so a crash restarts itself within five seconds and only a `stop` keeps it down.

**After editing the unit file** (a new token, a changed URL, a new variable), reload before
restarting, or systemd runs the old definition:

```bash
systemctl daemon-reload
systemctl restart mcharbor-agent
```

On a host without systemd there is nothing supervising it, so a restart is kill and relaunch,
with the same environment the install script used:

```bash
pkill -f /usr/local/bin/mcharbor-agent
# then re-run the nohup line from the install script
```

For the container install it is `docker restart mcharbor-agent`.

Changing *what* the agent runs with, rather than just bouncing it, is
[Configuration → Changing a variable on the host, from the terminal](../configuration/README.md#changing-a-variable-on-the-host-from-the-terminal).

Two restarts that are **not** enough on their own:

- **A regenerated token.** The old one is rejected immediately, so the agent will restart,
  fail to authenticate, and climb the backoff. Put the new token in the unit file (or the
  container's environment) first, then `daemon-reload` and restart.
- **A binary upgrade done by hand.** Replacing `/usr/local/bin/mcharbor-agent` takes effect
  only on restart, and `install -m 0755` over a running binary is the safe way to swap it;
  a plain `cp` into a busy file gives `Text file busy`.

## Routine changes

| Change | How |
|---|---|
| Move an agent to a different McHarbor | Create an environment there, install with its token. An agent serves one McHarbor at a time |
| Rotate a token | **Regenerate token** on the environment, then reinstall the agent |
| Turn off direct transfers on one host | Set `MCHARBOR_TRANSFER_LISTEN=` (empty) and recreate |
| Reduce chatter from a busy host | Raise the event poll interval centrally; it applies to every agent |
| Stop collecting agent metrics | Turn metrics off centrally. Pushing agents are told to stop; pulled ones stop being swept |
| Move an agent from pull to push | Install a current agent binary on the host; see [Container stats](../stats/README.md#updating-an-agent-for-push) |
| Retire a host | Remove the environment, then remove the agent and `/var/lib/mcharbor-agent` |

## What is audited

Agent lifecycle actions are audited like any other mutation: creating and removing
environments, regenerating tokens, deploying over SSH, triggering updates. Host terminal
access is recorded as an event (that a session opened, by whom, against which host), not as
a transcript.

## Related

- [Configuration](../configuration/README.md): every variable these operations touch
- [Protocol](../protocol/README.md): what travels over the connection
- [Troubleshooting](../troubleshooting/README.md): when something is wrong
- [Settings → Updates → Agents](../../settings/updates/agents/README.md): version sweeps
