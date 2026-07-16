// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

# Agent Setup

The McHarbor remote agent runs on a host you want McHarbor to manage and
connects outbound to your McHarbor server over a single WebSocket. This
guide covers installing the agent for a single host and the additional
configuration needed to enable direct agent-to-agent transfers (used by
container moves between two agent-managed environments).

## When to use an agent

- The Docker host is behind a NAT or firewall that you do not want to
  expose a Docker daemon socket to.
- You run Docker on a NAS, Raspberry Pi, Proxmox VM, or another
  remote machine that should be managed alongside your local
  McHarbor.
- You want McHarbor to manage remote hosts without a VPN or
  port-forwarding.
- You want to move containers between two remote hosts and would
  rather stream the image archive directly between them than relay
  it through McHarbor.

The agent is **not** required to use McHarbor with a local Docker
daemon — the local socket path works without it.

## Architecture

```
                                    ┌──────────────────┐
   McHarbor server  ◀── WebSocket ── │   mcharbor-agent  │
   (port 8705)                     │   (per Docker host) │
                                    │                    │
   - API                            │   - Docker socket  │
   - Web UI                         │   - HTTP proxy     │
   - WebSocket endpoint             │   - exec sessions  │
                                    │   - direct transfer│
   ◀─────────────── direct transfer (optional) ──────────▶
   (between two agents, via the source agent's transfer listen
    address and the target agent's transfer advertise URL)
```

One agent per Docker host. Multiple agents can connect to one McHarbor
server. The agent holds an outbound WebSocket open at all times and
reconnects with exponential backoff on failure.

## Prerequisites

On the agent host:

- Linux with Docker Engine (17.09 or newer) — the agent only runs on
  Linux. macOS works for development; Windows requires WSL2.
- The agent needs read/write access to the local Docker socket.
- A reachable Docker daemon: the agent defaults to
  `unix:///var/run/docker.sock` and supports TCP and SSH-based
  Docker daemons too.
- For direct agent-to-agent transfer: a TCP port the **source** agent
  can reach on the **target** agent. Usually a port the host
  firewall already allows, or one you expose on demand.

On the McHarbor server:

- The McHarbor container must be reachable from the agent. The
  default is `http://<your-mcharbor>:8705`.
- The server must allow the agent URL's host header. The default
  `ALLOWED_ORIGINS` setting covers the typical local-network case;
  for remote agents use the McHarbor server's public hostname or
  a TLS reverse proxy.

## Step 1 — Create an environment of type "Agent" in the McHarbor UI

The agent token is generated when you create an agent-type
environment. The token is shown **once** at creation time and is
required to bring the agent up.

1. Sign in to McHarbor.
2. Go to **Settings → Environments → New environment**.
3. Pick **Connection type → Agent**.
4. Fill in the environment name and any optional defaults (default
   environment flag, label, etc.).
5. Click **Create**. The UI shows a one-time token in the format
   `agt_<32 hex characters>`. Copy it immediately and store it
   somewhere safe. Anyone with the token can manage Docker on the
   host, so treat it like a password.

If you lose the token, sign in as an admin and use
**Regenerate token** on the environment's settings page. The old
token stops working the moment the new one is saved.

## Step 2 — Install the agent

The agent is published as a Docker image and as a standalone binary.
Pick whichever fits your host.

### Option A — Docker (recommended)

Run the agent container on the Docker host it should manage. The
container needs `/var/run/docker.sock` mounted read-write and the
agent token in the environment:

```bash
docker pull ghcr.io/therealmcsparrow/mcharbor-agent:latest
docker rm -f mcharbor-agent 2>/dev/null || true
mkdir -p /var/lib/mcharbor-agent/compose

docker run -d \
  --name mcharbor-agent \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/mcharbor-agent/compose:/var/lib/mcharbor-agent/compose \
  -e MCHARBOR_URL=http://mcharbor.example.com:8705 \
  -e MCHARBOR_AGENT_TOKEN=agt_your_token_here \
  -e DOCKER_HOST=unix:///var/run/docker.sock \
  ghcr.io/therealmcsparrow/mcharbor-agent:latest
```

The agent connects outbound to the McHarbor server and reports the
host as online within a few seconds. Verify in **Settings →
Environments** that the agent status flips to **Connected**.

### Option B — Standalone binary

Use this when the agent host cannot run Docker (rare) or you prefer
to run the agent on the host directly.

1. Download the binary for your host's architecture from the
   GitHub release page. The asset is named
   `mcharbor-agent_<version>_<os>_<arch>.tar.gz`.
2. Extract and install:

   ```bash
   tar -xzf mcharbor-agent_linux_amd64.tar.gz
   sudo install -m 0755 mcharbor-agent /usr/local/bin/
   ```

3. Create an environment file at `/etc/mcharbor-agent.env`:

   ```env
   MCHARBOR_URL=http://mcharbor.example.com:8705
   MCHARBOR_AGENT_TOKEN=agt_your_token_here
   DOCKER_HOST=unix:///var/run/docker.sock
   MCHARBOR_COMPOSE_DIR=/var/lib/mcharbor-agent/compose
   LOG_LEVEL=info
   ```

4. Install a systemd unit at `/etc/systemd/system/mcharbor-agent.service`:

   ```ini
   [Unit]
   Description=McHarbor Remote Agent
   After=network-online.target docker.service
   Wants=network-online.target

   [Service]
   EnvironmentFile=/etc/mcharbor-agent.env
   ExecStart=/usr/local/bin/mcharbor-agent
   Restart=always
   RestartSec=5
   LimitNOFILE=65536

   [Install]
   WantedBy=multi-user.target
   ```

5. Enable and start:

   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now mcharbor-agent
   sudo systemctl status mcharbor-agent
   ```

## Step 3 — Verify the connection

In the McHarbor UI:

1. **Settings → Environments**. The agent's environment should show
   status **Connected** with the host's hostname, OS, and
   architecture. The version row shows the running agent version.
2. **Containers → All containers**. Containers from the agent host
   should be listed and manageable.

In the agent's logs:

- A `connected` log line followed by periodic `ping` traffic.
- An `auth result` line on first connect. The `host_id` field
  matches the environment id shown in the McHarbor UI.

Common problems:

| Symptom | Cause | Fix |
| --- | --- | --- |
| `disconnected` after auth | Token mismatch or wrong URL | Verify the token matches the one shown in **Settings → Environments**; check `MCHARBOR_URL` includes the right scheme, host, and port. |
| `forbidden: agent token required` | `MCHARBOR_AGENT_TOKEN` not set on the agent | Add the env var and restart. |
| `connection refused` | McHarbor server unreachable from the agent | From the agent host, `curl http://mcharbor.example.com:8705/api/health` should return 200. Check the firewall and the URL. |
| Agent connects, then disconnects repeatedly | Network is NAT'ing the WebSocket idle session | Set a short TCP keepalive on the agent's network path, or ensure the host's firewall doesn't close long-lived TCP connections after a few minutes. |
| `compose dir not writable` | `MCHARBOR_COMPOSE_DIR` missing or not writable | Create the directory and `chown` it to the same user as the agent process. |

## Agent-to-agent direct transfer

When you move a container between two agent-managed environments,
the default path is to relay the image through the McHarbor server.
For large images (1 GB and up), agent-to-agent direct transfer is
faster because the data moves directly between the two hosts and
the McHarbor server only signals the operation.

Direct transfer requires:

1. Both agents to be at version `1.3.5` or newer.
2. The **target** agent to listen on a TCP port reachable by the
   **source** agent.
3. The source agent to know the URL to reach the target.

### Configure the target agent

The target agent needs a TCP listen port and a URL the source can
reach it on. Both are set via env vars on the **target** agent:

```env
MCHARBOR_TRANSFER_LISTEN=0.0.0.0:8788
MCHARBOR_TRANSFER_ADVERTISE_URL=http://target-host:8788
```

- `MCHARBOR_TRANSFER_LISTEN` is the bind address for the agent's
  transfer server. Pick any port that the agent host's firewall
  allows inbound on. Port 8788 is conventional but any free
  port works.
- `MCHARBOR_TRANSFER_ADVERTISE_URL` is what the **source** agent
  uses to reach the target. It must be reachable from the source
  agent's host. In a homelab this is often
  `http://<target-hostname>.local:8788`. In a routed network it's
  a private IP. Across the public internet it's a public
  hostname (TLS recommended).

The McHarbor server picks up the advertise URL from the target
agent's auth handshake and remembers it. The target agent's
settings page in the McHarbor UI also shows the URL.

If you change the advertise URL later, the agent sends the new
value on the next auth refresh (every few minutes). You can force
a refresh by restarting the agent.

### Source agent does not need extra configuration

The source agent uses whatever transport it already has (WebSocket
to McHarbor, then HTTP to the target's advertise URL) to perform
the direct transfer. The source agent does not need to expose
anything inbound.

### Verify the route

Before relying on direct transfer for a real move, validate the
route from the source agent to the target's advertise URL.
McHarbor has a built-in test:

1. In the McHarbor UI, open **Settings → Agents**.
2. Click the target agent's row.
3. Click **Test direct transfer reachability**.

The test makes a single HTTP request from the McHarbor server to
the target's advertise URL. A 200 from the agent's transfer
endpoint means the source agent will be able to reach the same URL
for a real move.

If the test fails:

| Symptom | Cause | Fix |
| --- | --- | --- |
| `connection refused` | `MCHARBOR_TRANSFER_LISTEN` not set on the target, or wrong port | Set the env var and restart the target agent. |
| `connection timeout` | A firewall between source and target blocks the port | Open the port on the source's outgoing rules and the target's incoming rules. |
| `404` from the agent's transfer endpoint | Agent is older than `1.3.5` | Upgrade the target agent. |
| `bad gateway` / `503` | The target host's reverse proxy is in the way | Use a direct port, not a proxy, for the transfer URL. |

### Behavior on failure

If direct transfer fails for any reason, McHarbor automatically
falls back to the relay path through the server. The container
move still completes; it just takes longer for large images. You
can see which path was used in the move's progress log.

## Updating the agent

The agent is updated by replacing the running image. McHarbor
versions and agent versions are tracked separately; the agent
itself reports its version on every auth refresh.

To update the Docker-based agent:

```bash
docker pull ghcr.io/therealmcsparrow/mcharbor-agent:latest
docker rm -f mcharbor-agent
docker run -d \
  --name mcharbor-agent \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/mcharbor-agent/compose:/var/lib/mcharbor-agent/compose \
  -e MCHARBOR_URL=http://mcharbor.example.com:8705 \
  -e MCHARBOR_AGENT_TOKEN=agt_your_token_here \
  -e DOCKER_HOST=unix:///var/run/docker.sock \
  ghcr.io/therealmcsparrow/mcharbor-agent:latest
```

For binary installs, replace the binary and restart the systemd
unit. The agent reconnects automatically with the same token.

After updating, confirm in the McHarbor UI that the agent's
**Version** field updated. The McHarbor server pins certain
features to a minimum agent version; if the version is below the
required threshold, the server logs a warning when the agent
authenticates.

### Minimum agent version by feature

| Feature | Minimum agent |
| --- | --- |
| WebSocket transport + Docker proxy | `1.0.0` |
| Interactive exec protocol | `1.1.0` |
| Staged image-load for remote container moves | `1.3.3` |
| Agent-to-agent direct transfer | `1.3.5` |
| Settings direct-transfer reachability test | `1.3.7` |
| Agent-side Docker Compose stack execution | `1.4.0` |
| Pull-based agent transfers (image + restore) | `1.5.0` |
| SFTP self-test for storage locations | `1.6.0` |

## See also

- `application/remote-agent.md` — protocol details, message types, agent capabilities
- `application/deployment-and-runtime.md` — Docker Compose runtime, TLS, secrets
- `application/cluster-setup.md` — multi-node HA setup with the agent running across nodes
- `application/configuration-and-data.md` — environment variables, encryption keys
