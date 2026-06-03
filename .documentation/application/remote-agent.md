# Remote Agent

The McHarbor remote agent allows Docker hosts behind NAT or firewalls to be managed
without exposing the Docker daemon directly.

## Purpose

The agent runs on a remote machine and connects outbound to the McHarbor server over
WebSocket. This makes remote access practical in environments where inbound socket or
TCP access is not acceptable.

## Main File

- `src/agent/agent.go`
- `src/agent/proxy.go`
- `src/agent/transfer.go`

## Agent Responsibilities

- build the WebSocket URL from the configured McHarbor URL
- connect to `/api/agent/ws`
- authenticate with the agent token
- report host metadata such as hostname, OS, architecture, and versions
- proxy Docker HTTP traffic through the server transport
- handle exec session traffic for terminal access
- run Docker Compose stack commands from a staged project workspace
- optionally receive direct image uploads from peer agents for container moves
- optionally stream Docker image archives directly to another agent
- optionally run a lightweight direct-transfer reachability probe

## Current Embedded Version

The agent currently reports:

- `1.4.0`

This matters because backend behavior checks agent capabilities by version:

- `1.1.0+`: interactive exec protocol support
- `1.3.3+`: streamed and staged Docker image-load request support for remote moves
- `1.3.5+`: optional direct agent-to-agent image transfer for container moves
- `1.3.7+`: Settings > Agent direct transfer reachability test
- `1.4.0+`: agent-side Docker Compose stack execution

## Auth Handshake

On connect, the agent sends:

- token
- hostname
- OS
- architecture
- agent version
- detected Docker version
- optional direct transfer advertise URL

The server replies with an auth result and environment binding information.

## Transport Model

The agent processes message types such as:

- ping / pong
- HTTP request proxy messages
- request cancellation
- exec start
- exec input
- exec resize
- exec end
- compose run
- compose result
- compose cancel
- transfer prepare
- transfer image
- transfer probe
- transfer progress
- transfer result
- transfer cancel

Normal Docker API calls still move through McHarbor over the authenticated
WebSocket. For large container moves between two agent environments, McHarbor can
coordinate an optional direct image transfer so the snapshot archive flows:

```text
source Docker host -> source agent -> target agent -> target Docker host
```

If direct transfer is not available, McHarbor falls back to the existing relay
path:

```text
source Docker host -> source agent -> McHarbor server -> target agent -> target Docker host
```

## Direct Transfer Configuration

Direct agent-to-agent transfer is opt-in. The target agent must expose a
temporary upload receiver that the source agent can reach.

Environment variables:

| Variable | Purpose |
| --- | --- |
| `MCHARBOR_TRANSFER_LISTEN` | TCP listen address for the target agent upload receiver, for example `0.0.0.0:8788`. |
| `MCHARBOR_TRANSFER_ADVERTISE_URL` | URL advertised to McHarbor and sent to source agents, for example `http://target-host:8788`. |

Both variables must be set on the target agent. If either value is missing, the
agent connects normally but does not advertise direct transfer capability.

Example Docker agent command:

```bash
docker run -d \
  --name mcharbor-agent \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/mcharbor-agent/compose:/var/lib/mcharbor-agent/compose \
  -p 8788:8788 \
  -e MCHARBOR_URL=http://mcharbor.example:8705 \
  -e MCHARBOR_AGENT_TOKEN=agt_xxx \
  -e DOCKER_HOST=unix:///var/run/docker.sock \
  -e MCHARBOR_COMPOSE_DIR=/var/lib/mcharbor-agent/compose \
  -e MCHARBOR_TRANSFER_LISTEN=0.0.0.0:8788 \
  -e MCHARBOR_TRANSFER_ADVERTISE_URL=http://target-host:8788 \
  ghcr.io/therealmcsparrow/mcharbor-agent:latest
```

Security notes:

- the upload receiver is opened by the agent only when configured
- McHarbor creates a random transfer ID and bearer token per transfer
- receivers are one-use and expire automatically
- tokens are never logged by the server
- the source agent receives the upload URL only over the authenticated WebSocket

Network notes:

- the source agent must be able to reach the target agent's advertised URL
- NAT, firewall, and DNS rules must be configured outside McHarbor
- use HTTPS or a trusted private network if the transfer crosses untrusted links

You can test this route from the McHarbor UI at Settings > Agent. The test
prepares a one-use probe receiver on the target agent and asks the source agent
to POST directly to it. It validates reachability and bearer-token handling
without loading a Docker image.

## Container Move Behavior

Container moves always create a temporary snapshot image from the source
container filesystem so writable-layer data is preserved. When the move crosses
agent environments:

- target image loading requires `mcharbor-agent` `1.3.3+`
- direct image transfer requires both source and target agents to be `1.3.5+`
- the Settings direct-transfer test requires both source and target agents to be `1.3.7+`
- stack deploys with Docker Compose on agent environments require the target agent to be `1.4.0+`
- direct transfer also requires the target transfer listener configuration above
- if direct transfer is unavailable, the move uses the McHarbor relay path
- cancellation aborts the active move but is not a transactional rollback

## Agent-Side Compose

For agent environments, McHarbor sends the managed stack's Compose file and
environment variables to the connected target agent. The agent stages those files
under `MCHARBOR_COMPOSE_DIR/<project>` and runs `docker compose --project-name
<project>` on the agent host.

Containerized agents should mount the compose directory at the same absolute path
on the host and inside the agent container:

```bash
-v /var/lib/mcharbor-agent/compose:/var/lib/mcharbor-agent/compose
```

This is required so relative bind paths in Compose files resolve to a real host
path visible to the Docker daemon.

## Why the Agent Exists

Without the agent, remote Docker management often depends on:

- exposed TCP Docker daemons
- SSH tunneling
- manually maintained network openings

The agent keeps the control model outbound and centrally managed instead.
