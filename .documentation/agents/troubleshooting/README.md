# Agent Troubleshooting

Symptoms after an agent is installed. For problems getting it installed in the first place,
see [Installation → Troubleshooting](../installation/troubleshooting.md).

## Connection

| Symptom | Usual cause | Fix |
|---|---|---|
| The environment flips between connected and disconnected | An idle connection being dropped by NAT or a proxy | Lower the ping interval in Settings → System → Agent |
| It reconnects, but slowly after an outage | Backoff is exponential, 1s up to 60s with jitter | Expected. A long outage means up to a minute before the next attempt |
| The agent is rejected after working for months | The environment's token was regenerated | Reinstall with the current token |
| `x509` errors after a certificate renewal | The host trusted the old certificate specifically | Trust the new one, or the issuing CA |
| Connected, but Docker lists are empty | The agent cannot reach its own Docker socket | Check `DOCKER_HOST` and socket permissions on the host |

## Performance

| Symptom | Usual cause | Fix |
|---|---|---|
| The whole interface is slow while one agent is busy | Event collection is set to **Stream**, which monopolises that agent's WebSocket | Switch to Poll. One agent has one connection, and everything else queues behind a stream |
| Inspect and stats time out on one host | The same, or a genuinely slow link | Poll mode first; then raise the request timeout |
| Charts are empty for an agent environment | Agent metrics collection is turned off | Turn it on centrally in Settings → System → Agent |
| An agent environment's history has gaps, or none at all | The agent is on the **pull** path and has more containers than one 60 s sweep covers at ~2 s each | Update the agent so it pushes. The Overview's **Container stats** card names the build it needs ([Container stats](../stats/README.md)) |
| Metrics make one agent's host slow | An older agent being pulled: ~2 s per container, eight at a time | Update it so it pushes; the per-container requests stop |

## Terminal and consoles

| Symptom | Usual cause | Fix |
|---|---|---|
| The container terminal will not open, everything else works | Agent older than v1.1.0 | Interactive exec rides dedicated agent messages; update the agent |
| Shells are listed but the session never starts | The same: detection uses a detached exec, which older agents do support | Update the agent |
| A guest console connects and hangs | The RFB handshake never completed | Usually the stream-ordering hazard; check the agent version, then report it with the agent log at `debug` |
| The host terminal is refused | Missing `environments.manage` | It is the most privileged surface in the product and is gated accordingly |

## Transfers

| Symptom | Usual cause | Fix |
|---|---|---|
| A cross-host move is much slower than expected | It is relaying through McHarbor rather than going direct | Run the agent-to-agent transfer test; allow TCP 8788 from source to target |
| The transfer test reports an unreachable probe URL | Auto-detected advertise address is wrong (NAT, several interfaces, a proxy) | Set `MCHARBOR_TRANSFER_ADVERTISE_URL` on the target agent |
| The responder marker does not match | Something other than the expected agent answered on that address | Check what is listening on 8788 on the target host |
| Transfers never go direct on one host | `MCHARBOR_TRANSFER_LISTEN` is empty, so the receiver is disabled | Intended, if you set it. Otherwise restore the default |

## Offline and replay

| Symptom | Usual cause | Fix |
|---|---|---|
| History has gaps after an outage | The buffer is bounded and drops the oldest rows first | 20,000 logs, 50,000 events, 200,000 metric samples. Long outages keep the most recent history |
| Nothing was buffered at all | `MCHARBOR_OFFLINE_BUFFER=false`, or no writable data directory | Check the variable and that `MCHARBOR_DATA_DIR` is mounted |
| Buffered data disappeared after a container recreate | The data directory was not a volume | Mount it. The instance id lives there too |
| Replay seems to stall | Batches are acknowledged one at a time | Watch the agent log; an interrupted replay resumes rather than duplicating |

## Disk

| Symptom | Usual cause | Fix |
|---|---|---|
| Temporary archives fill the host's disk after a crash | An interrupted backup, restore, image load or direct transfer left its archive in the OS temporary directory | The agent deletes its own archives older than ten minutes when it starts, from the 2026.10.02 build. It removes only regular files with its own prefixes and logs the count and bytes. An older agent leaves them in place, so update it, or remove them by hand |
| The environment's disk reads "No disk data" | The agent is offline, or it cannot report host stats | Bring the agent online, or update it. The **Host disk data missing** alert names the cause as `reason` ([Alerts → Host disk conditions](../../settings/monitoring/alerts/reference.md#host-disk-conditions)) |

## Crashes on AVX-512 CPUs

| Symptom | Usual cause | Fix |
|---|---|---|
| The agent restarts every few minutes with `fatal error: found bad pointer in Go heap` or `found pointer to free object` | Go's garbage collector corrupts its heap through an AVX-512 scan routine on some CPUs (seen on AMD Zen 4) | The agent's unit starts with `GODEBUG=cpu.avx512f=off,cpu.avx512vl=off`. An agent installed before that needs a reinstall, or the same line in a drop-in |
| Every container on the host stops at once, with no OOM and no `docker stop` | containerd crashed with the same error and Docker restarted | The agent install writes `mcharbor-go-avx512.conf` drop-ins for `containerd` and `docker` without restarting them; run `sudo systemctl restart containerd docker` when an outage suits you. See [Native Linux → Troubleshooting](../../getting-started/installation/native-linux.md#8-troubleshooting) |

## Compute hosting

| Symptom | Usual cause | Fix |
|---|---|---|
| The host is refused for guest hosting | No systemd as PID 1, or cgroup v1 | Both are hard requirements; the probe names which |
| Virtual machines cannot start | No `/dev/kvm`, virtualization disabled in firmware, or the agent's user is not in the `kvm` group | The probe distinguishes all three and gives the remediation |
| The capability panel reports nothing at all | The probe could not run | Usually a host without systemd; the panel says so rather than showing an empty checklist |
| Guests work, but only as root | Unprivileged containers need idmapped mounts | A kernel without `mount_setattr` is reported as such |
| Guest networks offer no bridge and no outbound interface | The agent runs in a container's own network namespace, where only loopback and its own veth exist: an install from before host networking became the default, or one made with `--isolated` | Reinstall without `--isolated`, or re-run the container with `--network host --cap-add NET_ADMIN` and no published 8788. The **Host network access** capability item names this |
| A managed network's guests get no address | The agent is older than 3.0.0, where no DHCP server existed | Update the agent. Before 3.0.0 the two managed-mode switches were recorded and ignored |
| A managed network's guests get an address but reach nothing | IPv4 forwarding is off, or the masquerade rules could not be installed | The agent reports the `sysctl` line, and needs `iptables` plus `NET_ADMIN` |
| Sharing an adapter reverted itself after 45 seconds | McHarbor could not reach the agent after the change, so the rollback fired | Intended. The reply has to cross the link that was reconfigured; check the adapter carries the agent's route to McHarbor |

## Licence and seats

| Symptom | Usual cause | Fix |
|---|---|---|
| The agent log says `MCHARBOR_LICENSE_KEY is ignored` | The key is still set on the agent | Remove it. Agents do not claim seats; the environment is counted by McHarbor |
| An agent reports nothing to the licence server | Intended | Nothing in the agent is gated on a licence |

## Working out where a problem lives

1. **Does the agent think it is connected?** Read its log on the host, or the environment's
   log panel in McHarbor.
2. **Does McHarbor think so?** The environment's status and last-seen.
3. **Is it one operation or everything?** One operation failing points at that family:
   exec, transfer, snapshot, compute. Everything failing points at the connection.
4. **Is it one host or all of them?** All of them means a central setting: event mode, poll
   interval, timeout. One means that host.
5. **Raise the detail.** `LOG_LEVEL=debug` on the agent shows the handshake, the backoff and
   every proxied call.

## Related

- [Protocol](../protocol/README.md): what each message family does, and the known hazards
- [Configuration](../configuration/README.md): every variable named above
- [Operations](../operations/README.md): tokens, updates, seats and diagnostics
- [Settings → System → Agent](../../settings/system/agent/README.md): the central settings
