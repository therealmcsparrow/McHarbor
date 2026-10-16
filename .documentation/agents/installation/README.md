# Installing an Agent

Every install starts in McHarbor: create the environment, take the token, run the agent.

## 1. Create the environment

[Environments](../../screens/environments/README.md) → **Add environment** → connection type
**Remote Agent**. Needs `environments.manage`.

McHarbor issues a token and shows it **once**:

> Save this token now — it will not be shown again.

Alongside it you get three ways to use it: a one-line install script, a `docker run`
command, and a binary command. The install URL is **single-use and expires after 24 hours**.

Lose the token and you regenerate it from the environment's detail page, which invalidates
the old one.

## 2. Run the agent

### The install script

```bash
curl -fsSL https://mcharbor.example.com/api/agent/install/<token> | sh
```

The script is generated per environment and already carries the URL, the token and the
`MCHARBOR_FORCE_TLS` value for how McHarbor is served. It then:

1. Detects the OS and architecture, mapping `x86_64`→`amd64`, `aarch64`→`arm64`,
   `armv7l`→`arm`.
2. Works out an advertise URL for direct transfers, from `ip route get 1.1.1.1` or
   `hostname -I`. If it cannot, it says so and explains that the agent will advertise its own
   addresses and McHarbor will add the address it sees the agent connect from.
3. **If Docker is present**, installs as a container: it loads the target release's agent
   image from McHarbor (`/api/agent/image/<os>/<arch>`, authenticated with the agent token,
   digest checked), and failing that builds a local image from the binary McHarbor serves.
4. **Otherwise**, downloads the binary McHarbor serves to `/usr/local/bin/mcharbor-agent`,
   checks its size and `X-Checksum-Sha256` digest, and, where
   `systemctl` exists, installs a systemd unit and starts it. It also links
   `/usr/local/bin/mcharbor` to the agent, which carries the command-line client, unless a
   separately installed `mcharbor` is already there, which it leaves alone.

It is fetched over an authenticated one-time URL, so treat the command as a secret while it
is valid.

Every install takes the agent **from McHarbor and nowhere else**. There is no fallback to
GitHub or to a registry: when McHarbor has nothing for the host's platform, the script stops
with

> Error: this McHarbor has no agent for linux/arm64.

and names the fix: download an agent release under **Settings → Updates → Agents** and make
it the target, or build one on the McHarbor host with `scripts/development/build-agent.sh`.
See [Releases](../releases/README.md). The one exception is an image you pin yourself with
`MCHARBOR_AGENT_IMAGE`, which the script pulls as asked.

#### The container runs on the host's network

A container install uses `--network host --cap-add NET_ADMIN`, and publishes no port.

That is not a tuning choice. On Docker's own network the agent's namespace contains loopback
and one veth and nothing else, so it cannot see a single interface belonging to the machine
it is managing: guest networks offer no bridge under **Bridged** and no outbound interface
under **Managed**, because none is visible from there. Host networking is what makes the
agent's view of the host the host's own.

`NET_ADMIN` is the other half: creating bridges, taps and a managed network's NAT rules all
need it.

There is no `-p 8788:8788`. With host networking the transfer listener binds the host's port
directly, and a published mapping alongside it is rejected rather than ignored.

**What you give up**: the agent has no network isolation from the host. It can reach anything
the host can reach, on any port. An agent already holds the Docker socket, so it can start a
privileged container on that host anyway, but the boundary is genuinely different, and if
this host will never run guests you can keep the old one:

```bash
curl -fsSL https://mcharbor.example.com/api/agent/install/<token> | sh -s -- --isolated
# or, when piping leaves no room for a flag:
curl -fsSL https://mcharbor.example.com/api/agent/install/<token> | MCHARBOR_AGENT_ISOLATED=1 sh
```

`--isolated` puts the agent back on Docker's bridge network with `-p 8788:8788`. Docker,
Compose, images, logs, terminals, transfers and the offline buffer all work exactly as
before; only guest hosting is unavailable, and both the installer and the **Host network
access** item in the capability panel say so rather than leaving you to infer it from an
empty picker.

Neither form applies to a binary install, which already runs on the host's network.

### As a container, by hand

Load the image from McHarbor first. The agent token is the credential, and the tarball is the
target release's image (see [Releases](../releases/README.md)); it is tagged here as
`mcharbor-agent:current` so the `docker run` below does not depend on the version:

```bash
IMG=$(curl -fsSL -H "Authorization: Bearer agt_…" \
  "https://mcharbor.example.com/api/agent/image/linux/$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" \
  | docker load | sed -n 's/^Loaded image: //p' | tail -1)
docker tag "$IMG" mcharbor-agent:current
```

The **Add environment** dialog's **Docker** tab produces exactly these lines with your URL and
token filled in.

```bash
docker run -d \
  --name mcharbor-agent \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/mcharbor-agent/compose:/var/lib/mcharbor-agent/compose \
  -v /var/lib/mcharbor-agent:/var/lib/mcharbor-agent \
  -e MCHARBOR_URL="https://mcharbor.example.com" \
  -e MCHARBOR_AGENT_TOKEN="agt_…" \
  -e MCHARBOR_FORCE_TLS=1 \
  -e DOCKER_HOST=unix:///var/run/docker.sock \
  -e LOG_LEVEL=info \
  -e MCHARBOR_COMPOSE_DIR=/var/lib/mcharbor-agent/compose \
  -e MCHARBOR_DATA_DIR=/var/lib/mcharbor-agent \
  -e MCHARBOR_TRANSFER_LISTEN=0.0.0.0:8788 \
  --network host \
  --cap-add NET_ADMIN \
  mcharbor-agent:current
```

Swap `--network host --cap-add NET_ADMIN` for `-p 8788:8788` if this host will never run
guests. See **The container runs on the host's network** above for what that changes.

Both volumes matter. The compose directory is where staged Compose projects live; the data
directory holds the offline buffer and the instance id.

### With the shipped Compose profile

The repository's `docker-compose.yml` carries an `agent` service under the `agent` profile:

```bash
MCHARBOR_URL=https://mcharbor.example.com \
MCHARBOR_AGENT_TOKEN=agt_… \
docker compose --profile agent up -d
```

That service also runs `network_mode: host` with `NET_ADMIN`, for the reasons above, so it
publishes no port either.

This is mostly useful for testing an agent beside McHarbor. In practice the agent belongs on
the *other* host.

### As a binary with systemd

This is the **native** install, and it is the only one that can host virtual machines or
system containers. A containerised agent cannot, whatever it is given. See
[Host requirements → container or native agent](../../compute-foundation/host-requirements.md).

#### Getting a native install in the first place

The install script chooses for you, and it chooses the container whenever `docker info`
succeeds. There is no flag that forces the binary path. So on a host that already runs
Docker (which is most of them), take one of these instead:

- **Deploy over SSH**, choosing **system binary**. McHarbor writes the unit, enables it and
  starts it, and this is the path to prefer: it installs the unit unconditionally rather
  than probing for `systemctl`.
- **By hand**, below.

#### The binary

McHarbor serves the target agent release, else the agent it was built with, so an install
taken from your own server matches what it expects: same version, same build stamp. The
precedence is written down in [Releases → What McHarbor serves](../releases/README.md#what-mcharbor-serves):

```bash
curl -fsSLo /usr/local/bin/mcharbor-agent   https://mcharbor.example.com/api/agent/binary/linux/amd64
chmod +x /usr/local/bin/mcharbor-agent
ln -s mcharbor-agent /usr/local/bin/mcharbor     # optional: the bundled CLI as `mcharbor`
```

The agent binary carries the McHarbor command-line client. Run it as `mcharbor-agent cli …`,
or as `mcharbor …` through a link of that name; see [the CLI](../../cli/README.md). A native
agent makes that link itself, beside its own binary, every time it starts and after every
self-update, so the `ln -s` above is only needed to use the CLI before the first start. It
never replaces a `mcharbor` that is anything other than that link.

That endpoint answers `HEAD` as well as `GET`, which both install paths rely on: they probe
it before downloading, and a probe that fails is explained (a 404 is "this McHarbor serves no
binary for that platform", anything else is a connection or TLS failure) and stops the
install. It also sends `X-Checksum-Sha256`, which the script compares after the download.

The generated install script does not take the file in one shot. It retries up to five times
and resumes with `--continue-at -`, restarting from zero when the server refuses ranges, and
then compares what landed against the `Content-Length` the probe reported. A truncated ELF
still carries valid magic bytes, so a header check cannot catch a short read and a size check
can. Doing it by hand with the `curl` above, you get one attempt and no size check, which matters
on a link that crosses a routed boundary.

Neither the script nor the SSH deploy falls back to GitHub any more. When McHarbor has no
binary for the platform, the script stops with the fix and the SSH deploy is refused before
anything runs on the host (`err.agent_release.no_artifact`). Getting a version into McHarbor
is [Releases](../releases/README.md); building from this repository instead is
`scripts/development/build-agent.sh`, whose output lands in `agents/` at the repository root
as `mcharbor-agent-<os>-<arch>`. Take it from there rather than from `src/agent/`, where a
binary compiled without the build flag reports its build as `dev`.

#### The unit

`/etc/systemd/system/mcharbor-agent.service`:

```ini
[Unit]
Description=McHarbor Remote Agent
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mcharbor-agent
Environment=MCHARBOR_URL=https://mcharbor.example.com
Environment=MCHARBOR_AGENT_TOKEN=agt_…
Environment=MCHARBOR_FORCE_TLS=1
Environment=DOCKER_HOST=unix:///var/run/docker.sock
Environment=LOG_LEVEL=info
Environment=MCHARBOR_TRANSFER_LISTEN=0.0.0.0:8788
Environment=MCHARBOR_TRANSFER_ADVERTISE_URL=http://<this host>:8788
Environment=MCHARBOR_DATA_DIR=/var/lib/mcharbor-agent
Environment=MCHARBOR_COMPOSE_DIR=/var/lib/mcharbor-agent/compose
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
chmod 600 /etc/systemd/system/mcharbor-agent.service
mkdir -p /var/lib/mcharbor-agent/compose
systemctl daemon-reload
systemctl enable --now mcharbor-agent
journalctl -u mcharbor-agent -f
```

`MCHARBOR_DATA_DIR` is what holds the offline buffer and the instance id. Leave it out and
both land wherever the default falls, which is how a reinstall claims a second seat.

The token sits in the unit file, so it inherits that file's permissions. Keep it
root-readable only. `MCHARBOR_PIN` belongs there too when McHarbor serves a certificate this
host does not trust; the generated installer writes it for you. Reach for
`MCHARBOR_INSECURE=true` only when there is no pin to use; it accepts any certificate at
all.

A healthy start logs `starting mcharbor-agent version=…`, then a connect line, then
`authenticated envId=…`. The connect line carries no `?token=`: the agent strips the query
string before writing it, because that log is read in McHarbor's own log viewer.

The agent needs the Docker socket it is pointed at. Running the unit as root is the simple
answer; a non-root `User=` has to be in the `docker` group.

#### No systemd, and other platforms

On a host with no `systemctl`, the script runs the binary in the background instead, which
does not survive a reboot. Wire it into whatever init that host uses: OpenRC, runit, or an
`/etc/rc.local` line.

McHarbor ships no service installer for **Windows or macOS**. The binary runs on both; making
it a service is `nssm` or `sc.exe create` on Windows and a launchd plist on macOS, written by
hand with the same environment variables as the unit above.

## 3. Deploy over SSH instead

McHarbor can install the agent for you: **Deploy over SSH**, from the environment wizard or
its detail page. It takes host, port, user, and either a private key or a password, and it
asks for the **SSH host fingerprint** first, telling you how to get it:

```bash
ssh-keyscan <host> | ssh-keygen -lf - -E sha256
```

Password authentication is offered with a warning attached, because it is the weaker option. You
choose whether to deploy as a Docker container or as a system binary.

The wizard can check the host first and install what it lacks; see
[Checking an SSH host before you deploy](../prerequisites/README.md#checking-an-ssh-host-before-you-deploy).

## 4. Verify

In McHarbor, the environment leaves "Waiting for agent…" and reports hostname, OS,
architecture, agent version and last seen.

On the host:

```bash
docker logs mcharbor-agent          # container install
journalctl -u mcharbor-agent -f     # binary install
```

A healthy start ends with an authenticated connection. A rejected token, an unreachable URL
or an untrusted certificate all name themselves in that log.

## Reinstalling and moving

- **Same host, same environment**: rerun the install with the same token. The container
  install removes a previous `mcharbor-agent` container first.
- **Token lost**: regenerate it on the environment; the old one stops working immediately.
- **A different McHarbor**: create an environment there and use its token. An agent serves
  one McHarbor at a time.
- **Reusing an existing token**: the environment wizard's **Use existing agent token**
  accepts a token already issued by another McHarbor host, in the form `agt_` followed by 64
  hex characters, instead of generating a fresh one.

## Uninstalling

```bash
docker rm -f mcharbor-agent                      # container
systemctl disable --now mcharbor-agent           # binary
rm -f /usr/local/bin/mcharbor-agent /etc/systemd/system/mcharbor-agent.service
[ "$(readlink /usr/local/bin/mcharbor)" = mcharbor-agent ] && rm /usr/local/bin/mcharbor   # the bundled CLI's link
rm -rf /var/lib/mcharbor-agent                   # state, buffer and instance id
```

Removing the environment in McHarbor removes its record and its token; containers on the host
are untouched.

## In this folder

| Page | What it covers |
|---|---|
| [Troubleshooting](./troubleshooting.md) | When the agent will not install or will not connect |

## Related

- [Prerequisites](../prerequisites/README.md): what the host needs first
- [Configuration](../configuration/README.md): every environment variable
- [Operations](../operations/README.md): tokens, updates and diagnostics
