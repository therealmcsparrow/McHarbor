# Change an agent's environment variables

Moving an agent to a new URL, turning debug logging on, switching a token, pinning a
certificate, disabling the direct-transfer receiver: all of it is one variable and one
restart. What differs is how the process gets replaced, and that depends on how the agent
was installed.

The variables themselves, with their defaults and what each one does, are in
[Agent configuration](../../agents/configuration/README.md). This page is the operation.

## The one rule

**Every variable is read once, at start-up.** The agent has no reload signal: `SIGHUP` does
nothing and `SIGTERM` is a shutdown. Editing a unit file, an env file or a Compose file
changes nothing until the process behind it has been replaced, and for a container,
replaced means *recreated*, not restarted.

Three things are not agent variables at all. Event collection mode, the poll and ping
intervals, metrics collection and the request timeout are set centrally in
[Settings → System → Agent](../../settings/system/agent/README.md) and apply to every
agent-connected environment at once, with no host access and no restart. Check there first;
the change you want may not need any of what follows.

## Which install do I have?

```bash
systemctl cat mcharbor-agent >/dev/null 2>&1 && echo "native, systemd"
docker inspect mcharbor-agent >/dev/null 2>&1 && echo "container"
pgrep -af /usr/local/bin/mcharbor-agent
```

A native install with no unit shows only the `pgrep` line.

## A native agent

### Under systemd: a drop-in (prefer this)

The installer wrote the variables as `Environment=` lines into
`/etc/systemd/system/mcharbor-agent.service`. A reinstall or an SSH redeploy overwrites that
file, and would take an edit made there with it. A drop-in survives both.

```bash
systemctl edit mcharbor-agent
```

The editor opens empty. Add only what you are changing, under a `[Service]` header:

```ini
[Service]
Environment=LOG_LEVEL=debug
Environment=MCHARBOR_TRANSFER_LISTEN=
```

Saving writes `/etc/systemd/system/mcharbor-agent.service.d/override.conf` and reloads the
daemon for you. Then restart and look:

```bash
systemctl restart mcharbor-agent
systemctl show mcharbor-agent -p Environment     # what it will actually run with
journalctl -u mcharbor-agent -f
```

#### Undoing an override

To go back to the installer's unit, with every drop-in gone and every overridden value back to what
the install wrote:

```bash
sudo systemctl revert mcharbor-agent
sudo systemctl restart mcharbor-agent
```

`revert` deletes `mcharbor-agent.service.d/` and reloads systemd, but the running agent keeps
the environment it was started with; the restart is what drops the override. Confirm with
`systemctl show mcharbor-agent -p Environment`.

A variable that shapes how a virtual machine is started, such as `MCHARBOR_VM_VHOST`, only reaches
a guest when QEMU is started again. Stop and start the VM from McHarbor afterwards; a reboot
from inside the guest keeps the same QEMU process and the old setting.

Two traps live in that file:

- A later `Environment=` assignment **wins** over an earlier one, which is the whole reason
  a drop-in works. But a drop-in cannot *unset* a variable the base unit sets. To clear
  one, assign it empty (`Environment=MCHARBOR_TRANSFER_ADVERTISE_URL=`) rather than omitting
  it.
- A value containing a space or a `#` needs quoting: `Environment="MCHARBOR_URL=…"`.

### Under systemd: editing the unit

Appropriate when the change belongs to the install itself: a rotated token, a moved
McHarbor.

```bash
$EDITOR /etc/systemd/system/mcharbor-agent.service
systemctl daemon-reload            # without this, systemd runs the old definition
systemctl restart mcharbor-agent
```

`daemon-reload` is the step people skip, and skipping it is silent: the restart reuses the
previous definition and the change simply appears not to have worked.

Keep the unit root-readable only (`chmod 600`); `MCHARBOR_AGENT_TOKEN` is in it.

### Under systemd: an env file

If you would rather the token were not in the unit at all:

```bash
install -m 600 /dev/null /etc/mcharbor-agent.env
cat >> /etc/mcharbor-agent.env <<'EOF'
MCHARBOR_URL=https://mcharbor.example.com
MCHARBOR_AGENT_TOKEN=agt_…
LOG_LEVEL=info
EOF
```

with `EnvironmentFile=/etc/mcharbor-agent.env` in the unit's `[Service]` section. The syntax
is not `Environment=`'s: one `KEY=value` per line, no quoting rules, and `#` starts a
comment. After that a variable change is an edit to the env file plus
`systemctl restart mcharbor-agent`, with no `daemon-reload`, because the unit itself has not
changed.

### No supervisor

Nothing holds the configuration but the command line that started the process, so a change
means stopping it and starting it again with the new environment:

```bash
pkill -f /usr/local/bin/mcharbor-agent
MCHARBOR_URL=… MCHARBOR_AGENT_TOKEN=… LOG_LEVEL=debug \
  nohup /usr/local/bin/mcharbor-agent > /var/log/mcharbor-agent.log 2>&1 &
```

This is the install that also loses its configuration on a reboot. If you are editing
variables on such a host at all, write the unit instead (it is in
[Installation](../../agents/installation/README.md)) rather than doing this twice.

## A Docker agent

A container's environment is fixed at creation. There is no `docker update` for environment
variables, and `docker restart` re-runs the *same* environment, so the container has to be
recreated.

### Started with `docker run`

Read the current environment back first. It is the easiest way to rebuild a run line you no
longer have:

```bash
docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' mcharbor-agent
```

The token is in that output, so mind who is watching. Then recreate with the value changed:

```bash
docker rm -f mcharbor-agent
docker run -d --name mcharbor-agent --restart unless-stopped \
  --network host --cap-add NET_ADMIN \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/mcharbor-agent:/var/lib/mcharbor-agent \
  -v /var/lib/mcharbor-agent/compose:/var/lib/mcharbor-agent/compose \
  -e MCHARBOR_URL=https://mcharbor.example.com \
  -e MCHARBOR_AGENT_TOKEN=agt_… \
  -e MCHARBOR_DATA_DIR=/var/lib/mcharbor-agent \
  -e MCHARBOR_COMPOSE_DIR=/var/lib/mcharbor-agent/compose \
  -e LOG_LEVEL=debug \
  ghcr.io/therealmcsparrow/mcharbor-agent:latest
```

**Recreating is only safe while `/var/lib/mcharbor-agent` is a volume.** The instance id
lives there; lose it and the new container claims a second licence seat and starts with an
empty offline buffer.

### Started from a Compose file

Edit the `environment:` block and bring it up. Compose recreates the container for you and
keeps the named volume:

```bash
$EDITOR docker-compose.yml
docker compose up -d agent
```

The repository's own `agent` service takes almost every value from the environment around
it, so the file usually does not need editing at all: change `.env` beside the Compose file
and bring it up again:

```ini
MCHARBOR_URL=https://mcharbor.example.com
MCHARBOR_AGENT_TOKEN=agt_…
MCHARBOR_AGENT_LOG_LEVEL=debug
MCHARBOR_AGENT_OFFLINE_BUFFER=true
```

```bash
docker compose --profile agent up -d
```

Note the indirection: the service maps `MCHARBOR_AGENT_LOG_LEVEL` onto the agent's
`LOG_LEVEL`, and `MCHARBOR_AGENT_INSECURE` onto `MCHARBOR_INSECURE`. Setting `LOG_LEVEL` in
`.env` does nothing, because nothing reads it. Read the `environment:` block for the mapping
before inventing a name.

A variable the repository's service does not pass through (`MCHARBOR_PIN`,
`MCHARBOR_FORCE_TLS`, `MCHARBOR_TRANSFER_LISTEN`) has to be added to the `environment:`
block itself.

`docker compose up -d` recreates only the services whose definition changed, so a value that
did not actually change leaves the container alone. That is correct behaviour, not a failure
to apply.

## Confirm it took effect

Whatever the install, the proof is in the log, not in the file you edited:

```bash
journalctl -u mcharbor-agent -n 30 --no-pager     # native
docker logs --tail 30 mcharbor-agent              # container
```

A process that re-read its configuration logs `starting mcharbor-agent version=…`, then a
connect line, then `authenticated envId=…`. If the environment is still behaving the old way
and those three lines never reappeared, the process was never replaced; go back a step.

For a container, `docker inspect -f '{{.Created}}' mcharbor-agent` settles the question of
whether it was recreated or merely restarted.

The environment's card in McHarbor should return to **Connected** within a few seconds. If it
does not, [Agent troubleshooting](../../agents/troubleshooting/README.md) works through the
usual causes: a mistyped URL, an expired token, a certificate the agent will not accept.

## Changes that need more than a restart

| Change | Also needed |
|---|---|
| A new `MCHARBOR_AGENT_TOKEN` | The token has to have been issued for **this** environment; a token from another environment authenticates as that one |
| A new `MCHARBOR_URL` | Reachability from the agent host, and a matching `MCHARBOR_PIN` if the certificate differs |
| A regenerated server certificate | Re-pin; the old `MCHARBOR_PIN` now rejects every connection |
| `MCHARBOR_DATA_DIR` moved | Move `offline.db` with it, or the buffered offline metrics are lost |
| `MCHARBOR_TRANSFER_LISTEN` changed | Open the new port on the host firewall; transfers fall back to the McHarbor relay until it is |

## Related

- [Agent configuration](../../agents/configuration/README.md): every variable, and what it does
- [Agent installation](../../agents/installation/README.md): where the variables get set in the first place
- [Agent operations](../../agents/operations/README.md): tokens, updates and licence seats
- [Settings → System → Agent](../../settings/system/agent/README.md): the settings that are not agent variables
