# Agents

The McHarbor **agent** is a small binary (shipped as a container image or a single
executable) that runs on a Docker host McHarbor does not own, and **dials outbound** to
McHarbor over a WebSocket.

Because the agent dials out, the remote host needs no inbound port, no exposed Docker
socket and no VPN. Everything McHarbor asks of that host travels back down the connection
the host itself opened.

## When the agent is the answer

| Situation | Why the agent |
|---|---|
| A host behind NAT, CGNAT or a firewall you do not control | Nothing has to be forwarded; the agent connects out |
| A host whose Docker socket you will not expose | A TCP Docker endpoint is root on that host for anyone who can reach the port. The agent is authenticated and scoped |
| Several hosts in one pane | One environment per host, all reporting into one McHarbor |
| Virtual machines or system containers | The compute add-ons run guests **through** the agent; a container cannot be the execution host for QEMU or LXC |
| A host that loses connectivity | The agent buffers events, stats and its own log locally and replays them on reconnect |
| Moving a container between hosts | Two agents can transfer directly to each other instead of relaying everything through McHarbor |

You do **not** need an agent for the host McHarbor itself runs on. That one is reached
through the mounted Docker socket.

## In this section

| Folder | What it covers |
|---|---|
| [Use cases](./use-cases/README.md) | NAT and homelab, fleets, compute hosting, cross-host moves, disconnected sites. Read this first if you are still deciding |
| [Prerequisites](./prerequisites/README.md) | What the host needs, which way the network has to work, and what the agent will be able to do once it is there |
| [Installation](./installation/README.md) | The three install paths (script, container, binary), the systemd unit, and verifying it connected |
| [Configuration](./configuration/README.md) | Every agent environment variable, and the host-side settings that govern all agents at once |
| [Protocol](./protocol/README.md) | How the connection works, every message family the agent serves, and the hazards worth knowing |
| [Operations](./operations/README.md) | Tokens, updates, licence seats, diagnostics and logs |
| [Releases](./releases/README.md) | McHarbor's own store of agent releases: sources, signature and checksum verification, what is served when, container updates without a registry, and the mirror contract |
| [Container stats](./stats/README.md) | Push and pull: how stats reach McHarbor, what each path costs, and how to tell which one a host is on |
| [Troubleshooting](./troubleshooting/README.md) | Symptom, cause and fix across the whole agent surface |

## Common tasks on the host

The things an operator actually does over SSH, and where each is written down:

| Task | Where |
|---|---|
| Install the agent | [Installation](./installation/README.md): script, container or binary |
| Read a native agent's log | [Operations → On the host](./operations/README.md#on-the-host): the journal, the no-systemd log file, macOS and Windows |
| Restart a native agent | [Operations → Restarting a native agent](./operations/README.md#restarting-a-native-agent) |
| Change a variable (URL, token, log level, transfer listener) | [Configuration → Changing a variable on the host](./configuration/README.md#changing-a-variable-on-the-host-from-the-terminal) |
| Turn the detail up while debugging | `LOG_LEVEL=debug` through a drop-in; see [Configuration](./configuration/README.md#changing-a-variable-on-the-host-from-the-terminal) |
| Rotate a token | [Operations → Tokens](./operations/README.md#tokens): regenerate in McHarbor, then reinstall |
| Update the agent | [Operations → Updating agents](./operations/README.md#updating-agents): centrally, not by hand |
| Get a new agent version into McHarbor | [Releases](./releases/README.md): download, verify, make it the target |
| Uninstall | [Installation → Uninstalling](./installation/README.md#uninstalling) |
| Work out why it will not connect | [Troubleshooting](./troubleshooting/README.md) |

Event mode, poll and ping intervals, metrics and the request timeout are **not** on the host.
They are central, and changing one needs no SSH at all.

## The shape of it in one page

1. You create an **environment** of type *Remote Agent* in McHarbor. It issues a token,
   shown **once**.
2. You run the agent on the remote host with that token and the McHarbor URL.
3. The agent opens a WebSocket to McHarbor, authenticates with the token, and reports its
   hostname, architecture and Docker version.
4. McHarbor proxies Docker API calls down that connection, and the agent answers them
   against its local socket. Containers, images, volumes, networks and stacks on that host
   appear in McHarbor like any other environment.
5. Interactive things (terminal, guest consoles, host shell, log streams) ride dedicated
   message families rather than the proxied HTTP path, because hijacked byte streams cannot
   travel that way.
6. If the connection drops, the agent keeps working locally, buffers what it can, and
   reconnects with exponential backoff up to a minute between attempts.

## What it needs from you

Very little, and that is deliberate: a URL, a token, and access to the Docker socket on that
host. Everything else (event collection mode, poll intervals, metrics, timeouts) is set
centrally in [Settings → System → Agent](../settings/system/agent/README.md) and applies to
every agent-connected environment at once.

## Related

- [Environments](../screens/environments/README.md): creating an agent environment and its token
- [Settings → System → Agent](../settings/system/agent/README.md): transport defaults and the agent-to-agent test
- [Settings → Updates → Agents](../settings/updates/agents/README.md): keeping agents current
- [Getting started](../getting-started/README.md): installing McHarbor itself
