# Agent Configuration

Everything the agent reads, and everything McHarbor decides on its behalf.

Agent configuration is split in two, deliberately:

| Where | What lives there | Scope |
|---|---|---|
| **Environment variables on the agent** | Identity, transport, state, licence | That one agent |
| **[Settings → System → Agent](../../settings/system/agent/README.md)** | Event mode, poll and ping intervals, metrics, request timeout | **All** agent-connected environments at once |

The variables are read at start-up, so changing one means recreating the container or
restarting the unit.

## Required

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_URL` | **required** | The McHarbor instance to dial. `http://` and `https://` are both accepted, and are promoted to `ws://`/`wss://` internally |
| `MCHARBOR_AGENT_TOKEN` | **required** | The environment's agent token, in the form `agt_` + 64 hex characters. Shown once, at creation |

The agent refuses to start without both.

## Docker and logging

| Variable | Default | What it does |
|---|---|---|
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | The daemon this agent manages. A `tcp://` endpoint works, though the usual answer is the local socket |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. `debug` shows the handshake, the reconnect backoff and every proxied call |

## Transport and TLS

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_FORCE_TLS` | `false` | Promotes `ws://` to `wss://` and `http://` to `https://` in the dialled URL. The generated install script sets it to `1` when McHarbor is served over HTTPS |
| `MCHARBOR_INSECURE` | `false` | Skips TLS verification when dialling McHarbor |
| `MCHARBOR_PIN` | unset | The SHA-256 fingerprint of McHarbor's **root CA**. The agent then accepts only certificates chaining to it. Colon-separated or bare hex |

`MCHARBOR_PIN` is the right answer to a privately-issued McHarbor certificate, and every
install method McHarbor offers sets it for you while the dialog's self-signed switch is on:
the one-line script, the Docker and Compose snippets, the binary and its systemd unit, and
the SSH deploy. It names the **root CA**, so it survives the 90-day certificate renewal; a
pin on the server certificate would drop the agent at each one, and is rejected with a
message saying so. The agent
verifies that one certificate and rejects everything else, which is stronger than public
CA verification with no pin. Copy the fingerprint from Settings -> Security -> HTTPS, or
read it from the server log at startup. It takes precedence over `MCHARBOR_INSECURE` when
both are set, and it covers the WebSocket control channel and the agent's HTTP calls.
Re-pin after the server certificate is regenerated.

`MCHARBOR_INSECURE=true` accepts *any* certificate and is a last resort on a network you
fully control. It disables the check that McHarbor is who it says it is, so prefer
`MCHARBOR_PIN`, or trust the root certificate on the host instead.

## Direct transfers

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_TRANSFER_LISTEN` | `0.0.0.0:8788` | Where the direct agent-to-agent transfer receiver binds. **Empty disables the receiver entirely** and forces every transfer through the McHarbor relay |
| `MCHARBOR_TRANSFER_ADVERTISE_URL` | _(empty)_ | Overrides address discovery. Leave it unset unless the receiver is reached through port forwarding or a reverse proxy |

Address discovery is a chain: the install script tries `ip route get 1.1.1.1`, then
`hostname -I`; failing that, the agent advertises its own addresses and McHarbor adds the
address it sees the agent connect from. Setting the variable is the escape hatch when none of
those produce a reachable address.

Disabling the receiver is a legitimate choice on a host where opening 8788 is unwelcome. The
cost is speed, not function.

## State

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_DATA_DIR` | `/var/lib/mcharbor-agent` | Agent state: the offline buffer (`offline.db`) and the instance id. **Mount it as a volume** or the buffer survives restarts only, not recreates |
| `MCHARBOR_COMPOSE_DIR` | `/var/lib/mcharbor-agent/compose` | Where the agent stages Compose project files before running them |

The instance id living in the data directory is why a recreated agent without a volume claims
a fresh licence seat.

## Offline buffering

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_OFFLINE_BUFFER` | `true` | Buffer Docker events, container stats and the agent's own log while McHarbor is unreachable, and replay them on reconnect |
| `MCHARBOR_OFFLINE_METRICS_INTERVAL` | `30` | Stats sampling cadence in seconds while offline. 30 matches the server-side collector |

The buffer is a bounded ring in local SQLite. The oldest rows are dropped once a table
reaches its cap, so a host that stays offline for weeks keeps the most recent history rather
than filling the disk:

| Buffered | Cap |
|---|---|
| Logs | 20,000 rows |
| Events | 50,000 rows |
| Metrics | 200,000 rows |

At roughly 200 bytes per row the whole file stays well under 100 MB.

## Virtual machines

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_VM_VHOST` | _(on)_ | `off`, `0`, `false` or `no` starts every VM's NICs with `vhost=off`, moving the receive and transmit path from the kernel's vhost-net back into QEMU. Slower; it exists as a diagnostic |

Read when a VM **starts**, so a change reaches a guest only after it is stopped and started
from McHarbor. It is a way to rule vhost-net in or out when a guest's inbound traffic
disappears (see
[Put guests on your network → A guest gets no address](../../compute-foundation/guest-networking.md#a-guest-gets-no-address)).
Remove it again once the question is answered.

## Licensing

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_LICENSE_KEY` | _(empty)_ | Ignored, with a warning in the agent log. An agent does not claim a seat; its environment is counted by the McHarbor that manages it |

Leave the key unset and the agent reports nothing outward; no path in the binary is gated on
a licence.

## Internal

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_RETIRE_CONTAINER_ID` | _(empty)_ | Set by the self-update flow on the replacement container so it removes the container it replaced, once it is running |

You never set this by hand.

## The host-side settings

These live in McHarbor and apply to **every** agent-connected environment:

| Setting | Default | Why it matters |
|---|---|---|
| **Event collection mode** | Poll (recommended) | Stream monopolises the agent WebSocket and can block inspect and stats behind it |
| **Event poll interval** | — | Lower is fresher and costs more |
| **Ping interval** | — | Keepalives stop a NAT or proxy silently dropping an idle connection |
| **Metrics collection** | On | Container stats for agent environments. A current agent pushes them; an older one is pulled at about **2s per container** (see [Container stats](../stats/README.md)) |
| **Stats push interval** | 10 s | How often a push-capable agent sends its stats, 5 to 300 s. McHarbor stretches it for every agent when too many push at once |
| **Request timeout** | — | Raise it for an agent across a slow link |

Full detail, including the agent-to-agent transfer test, is in
[Settings → System → Agent](../../settings/system/agent/README.md).

## Changing a variable on the host, from the terminal

Every variable above is read **once, at start-up**. The agent has no reload signal (SIGHUP
does nothing, and SIGTERM is a shutdown), so a change takes effect only once the process has
been replaced. The three installs differ only in how you replace it.

### A native install under systemd

The installer wrote the variables as `Environment=` lines into
`/etc/systemd/system/mcharbor-agent.service`. Two ways to change them, and the second is the
one to prefer.

**A drop-in** leaves the installer's unit alone, which matters because a reinstall or an SSH
redeploy overwrites that file and would take your edit with it:

```bash
systemctl edit mcharbor-agent
```

The editor opens an empty override; add only what you are changing, under a `[Service]`
header:

```ini
[Service]
Environment=LOG_LEVEL=debug
Environment=MCHARBOR_TRANSFER_LISTEN=
```

Saving writes `/etc/systemd/system/mcharbor-agent.service.d/override.conf` and reloads the
daemon for you. Then:

```bash
systemctl restart mcharbor-agent
systemctl show mcharbor-agent -p Environment    # what it will actually run with
journalctl -u mcharbor-agent -f
```

To undo an override and go back to the installer's unit:

```bash
sudo systemctl revert mcharbor-agent
sudo systemctl restart mcharbor-agent
```

`revert` deletes `mcharbor-agent.service.d/` and reloads systemd, but the running agent keeps
the environment it was started with; the restart is what drops the override. A variable that
shapes how a virtual machine is started, such as `MCHARBOR_VM_VHOST`, reaches a guest only on its
next stop and start from McHarbor.

Two traps in that override file. An `Environment=` line for a variable the base unit already
sets **wins** (later assignment overrides earlier), which is what makes the drop-in work at
all; but a drop-in cannot *unset* a variable the base unit sets, so to clear one you assign
it empty (`Environment=MCHARBOR_TRANSFER_ADVERTISE_URL=`) rather than omitting it. And a
value with spaces or a `#` needs quoting: `Environment="MCHARBOR_URL=…"`.

**Editing the unit directly** is the other route, appropriate when the change belongs to the
install itself, such as a new token or a moved McHarbor:

```bash
$EDITOR /etc/systemd/system/mcharbor-agent.service
systemctl daemon-reload            # without this, systemd runs the old definition
systemctl restart mcharbor-agent
```

`daemon-reload` is the step people skip; a restart without it silently reuses the previous
unit and the change appears not to have worked.

Keep that file root-readable only (`chmod 600`), because `MCHARBOR_AGENT_TOKEN` is in it.
If you would rather the token were not in the unit at all, move the variables to a file and
point the unit at it:

```bash
install -m 600 /dev/null /etc/mcharbor-agent.env
cat >> /etc/mcharbor-agent.env <<'EOF'
MCHARBOR_URL=https://mcharbor.example.com
MCHARBOR_AGENT_TOKEN=agt_…
LOG_LEVEL=info
EOF
```

with `EnvironmentFile=/etc/mcharbor-agent.env` in the unit's `[Service]` section. Note the
syntax differs from `Environment=`: no quoting rules, one `KEY=value` per line, and `#`
starts a comment.

### A native install with no supervisor

Nothing holds the configuration but the command line that started it, so changing a variable
means stopping the process and starting it again with the new environment. Edit the `nohup` block
from the install script:

```bash
pkill -f /usr/local/bin/mcharbor-agent
MCHARBOR_URL=… MCHARBOR_AGENT_TOKEN=… LOG_LEVEL=debug   nohup /usr/local/bin/mcharbor-agent > /var/log/mcharbor-agent.log 2>&1 &
```

This is the install that loses its configuration on a reboot. If you are changing variables
on such a host at all, it is worth writing the unit (the [Installation](../installation/README.md)
page has it) rather than doing this twice.

### A container install

A container's environment is fixed at creation, so `docker run -e` changes need the container
recreated. There is no `docker update` for environment variables.

```bash
docker rm -f mcharbor-agent
docker run -d --name mcharbor-agent --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/mcharbor-agent:/var/lib/mcharbor-agent \
  … the full run line with the new value …
```

Recreating is safe **only if `/var/lib/mcharbor-agent` is a volume**, as above. Without it the
instance id is lost with the container and the new one claims a second licence seat, and the
offline buffer goes with it. `docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}'
mcharbor-agent` prints the current environment, which is the easiest way to rebuild a run
line you no longer have. The token is in that output, so mind who is watching.

If the agent was installed from a Compose file, edit the `environment:` block and
`docker compose up -d`. Compose recreates it for you and keeps the named volume.

### Checking it took effect

Whatever the install, the proof is in the log rather than in the file you edited:

```bash
journalctl -u mcharbor-agent -n 30 --no-pager   # or docker logs --tail 30 mcharbor-agent
```

A restart that re-reads the configuration logs `starting mcharbor-agent version=…`, then a
connect line, then `authenticated envId=…`. If the environment is still showing the old
behaviour and those lines never reappeared, the process was never replaced.

### What you cannot change here

Event collection mode, the event poll and ping intervals, metrics collection, the stats push
interval and the request timeout are **not** agent variables; they are set centrally and apply to every agent at once.
Changing them needs no host access and no restart. They are in
[Settings → System → Agent](../../settings/system/agent/README.md).

## A worked example

A host behind NAT, McHarbor's own root CA trusted locally, direct transfers off,
verbose logging while you set it up:

```bash
docker run -d --name mcharbor-agent --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/mcharbor-agent:/var/lib/mcharbor-agent \
  -v /var/lib/mcharbor-agent/compose:/var/lib/mcharbor-agent/compose \
  -e MCHARBOR_URL=https://mcharbor.example.com \
  -e MCHARBOR_AGENT_TOKEN=agt_… \
  -e MCHARBOR_FORCE_TLS=1 \
  -e MCHARBOR_TRANSFER_LISTEN= \
  -e MCHARBOR_DATA_DIR=/var/lib/mcharbor-agent \
  -e MCHARBOR_COMPOSE_DIR=/var/lib/mcharbor-agent/compose \
  -e LOG_LEVEL=debug \
  ghcr.io/therealmcsparrow/mcharbor-agent:latest
```

Drop `LOG_LEVEL=debug` once it is connected.

## Related

- [Change an agent's environment variables](../../how-to/agent/README.md): the operation, per install type, end to end
- [Installation](../installation/README.md): where these variables are set
- [Operations](../operations/README.md): tokens, updates and licence seats
- [Settings → System → Agent](../../settings/system/agent/README.md): the host-side settings
