# Installing an Agent: Troubleshooting

What goes wrong installing an agent, and what each symptom actually means. Most of it is
one of three things: an install token that has already been used or expired, a McHarbor
certificate the host does not trust, or something in the path that will not pass a
WebSocket upgrade.

| Symptom | Usual cause | Fix |
|---|---|---|
| The install URL returns 404 or an error | It is **single-use and expires after 24 hours** | Create a new install token on the environment |
| The environment stays "Waiting for agent…" | The install never ran, or the agent cannot reach McHarbor | Check the agent's own log first; it names the reason |
| `x509: certificate signed by unknown authority` | McHarbor's certificate is signed by its own local root, which the host does not trust yet | Install the root on the host (see [Installing the root CA](../../screens/security/install-root-ca.md)) or pin McHarbor's root CA with `MCHARBOR_PIN`. `MCHARBOR_INSECURE=true` accepts *any* certificate and is the last resort, on a private network only |
| The agent connects, then is rejected | Wrong or regenerated token | Regenerating invalidates the previous token; reinstall with the current one |
| `connection refused` dialling McHarbor | Wrong URL, wrong port, or egress blocked | Only outbound access is needed; test with `curl -k <url>/api/health` from the host |
| `websocket: bad handshake` | Something in the path does not allow WebSocket upgrades | A reverse proxy in front of McHarbor has to allow them |
| The script installs a binary on a host that has Docker | `docker info` failed, so the script took the binary path | Fix Docker access for the user running the script, or install the container by hand |
| `curl: (56) OpenSSL SSL_read: ... decryption failed or bad record mac` part-way through the download | The bytes changed in flight: a tunnel with a broken path MTU, a NIC offload bug, or a middlebox mangling a segment. Large transfers fail, small ones do not | The script retries and resumes, so re-running it usually completes. If it keeps failing, lower the path MTU between the host and McHarbor |
| The install fails naming a downloaded size that does not match the expected one | The download was truncated. A short ELF still has valid magic bytes, so only the length check catches it | Re-run the install; the download resumes. Both numbers are printed so you can see how far it got |
| The one-liner prints the script instead of running it, and the agent never connects | The install URL was not quoted. It carries more than one query parameter now, and an unquoted `&` backgrounds `curl` and splits the command | Quote the URL. The URL the environment dialog gives you is already quoted |
| The binary install does not survive a reboot | No `systemctl`, so the script ran it in the background | Wire it into that host's init system |
| The script says it started the binary in the background, on a host that *does* run systemd | The script's `systemctl` probe never succeeds, so it always takes the background path | Install the unit yourself, or use **Deploy over SSH** → system binary, which writes it unconditionally |
| A binary install connects but every Docker operation fails | The unit runs as a user that cannot open `/var/run/docker.sock` | Run it as root, or put `User=` in the `docker` group |
| Permission denied on `/var/run/docker.sock` | The agent's user is not in the `docker` group | Add it, or run the agent as root; the container install mounts the socket directly |
| Compose operations fail on an agent host | `MCHARBOR_COMPOSE_DIR` is not writable, or not mounted into the container | Both install paths create it; a hand-rolled container needs the volume |
| The offline buffer is empty after a recreate | `MCHARBOR_DATA_DIR` is not a volume | Mount it, or buffered data and the instance id are lost with the container |
| A second seat is claimed after reinstalling | A fresh instance id was minted | The instance id lives in `MCHARBOR_DATA_DIR`; persist it |
| Direct transfers never happen | Port 8788 is unreachable between the hosts, or the advertise URL is wrong | Transfers still work through the relay. Run the agent-to-agent test in Settings → System → Agent |
| The advertise URL is wrong behind NAT | Auto-detection found a private address | Set `MCHARBOR_TRANSFER_ADVERTISE_URL` explicitly |

## Reading the agent log

```bash
docker logs -f mcharbor-agent          # container install
journalctl -u mcharbor-agent -f        # binary install
```

McHarbor also surfaces the agent's log in the environment's detail page, which is the faster
route when you do not have shell access to the host.

Raise detail with `LOG_LEVEL=debug` on the agent, recreate it, and reproduce. The
authentication handshake, the reconnect backoff and every proxied call are visible at that
level.

## Confirming the direction of travel

If you are unsure whether a connectivity problem is inbound or outbound, this settles it.
Run on the **agent host**:

```bash
curl -fsSk https://mcharbor.example.com/api/health
```

Success means the agent can reach McHarbor, which is the only direction that has to work for
Docker management. Failure is a firewall, DNS or TLS problem on that path, not something to
fix in McHarbor.

## Related

- [Installation](./README.md): the three install paths
- [Troubleshooting](../troubleshooting/README.md): problems after the agent is connected
- [Configuration](../configuration/README.md): every environment variable
