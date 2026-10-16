# Reverse proxy

Three worked examples under `deploy/proxy/` (Caddy, Traefik and NGINX), each terminating TLS
for `mcharbor.example.com` and forwarding to McHarbor on port `8705`. Each is written for two
nodes, `mcharbor-node-1` and `mcharbor-node-2`, as in `docker-compose.ha.yml`; for a single
node, remove the second upstream.

## What McHarbor needs from a proxy

| Requirement | Why | Where it shows up |
|---|---|---|
| WebSocket upgrades on any path | Terminal, host terminal, the compute console and the agent connection are WebSockets | `/api/terminal/ws`, `/api/system/host-terminal/ws`, `/api/compute/console/{ticket}`, `/api/agent/ws` |
| Read timeouts of an hour or more | A WebSocket or event stream is idle between messages. Agents ping every 30 seconds and give up after 90; a terminal can be quiet for much longer | All of the above, and the streams below |
| No response buffering | Server-sent events have to reach the browser as they are written | `/api/events/stream`, `/api/logs/{id}`, `/api/metrics/containers/{id}/stream`, `/api/workflows/{id}/live`, `/api/workflows/{id}/execute`, `/api/system/os-updates/stream`, and the install, update, uninstall, move and catalog-test `…/stream` routes |
| No request body limit, no request buffering | Uploads are large and streamed. The compute image library accepts up to 64 GiB per upload; image import and container file upload accept up to 4 GiB | Compute image library, `POST /api/images/import`, container file upload, backup upload, instance restore |
| Health checks on `/api/readyz` | Only the active node answers `200`. A standby answers `503` and refuses every change | Every multi-node setup |
| `Host` passed through | Links McHarbor builds for agents and install scripts use the request's host | Agent install command, `/api/agent/ws` |
| `X-Forwarded-Proto` set | Install scripts and redirects choose `https`/`wss` from it | Agent install command, the HTTPS redirect |

Everything is on one port. There is no separate WebSocket or agent port to publish.

## Upstream TLS

McHarbor serves HTTPS by default (`TLS_ENABLED=true`) with a certificate from a root CA it
generates itself, a different one on every node. The examples keep that and **do not verify
the upstream certificate**:

| Proxy | Setting |
|---|---|
| Caddy | `transport http { tls_insecure_skip_verify }` |
| Traefik | `serversTransports.mcharbor-upstream.insecureSkipVerify: true` |
| NGINX | `proxy_ssl_verify off` |

The hop is still encrypted, but a machine that can impersonate the upstream address on your
network can read it. That is acceptable on a host-local Docker network or a private segment you
control, and it is what the examples assume.

The alternatives:

- **Verify the upstream.** Upload a certificate for the node's name under **Settings → HTTPS**,
  or distribute McHarbor's root CA from `DATA_DIR/tls/ca.pem` to the proxy, and turn
  verification on. With several nodes that is one CA per node.
- **Plain HTTP upstream.** Set `TLS_ENABLED=false` on McHarbor and use `http://` upstreams. Do
  this only when McHarbor's port is reachable from the proxy alone: an internal Docker network
  with no published port, or a firewall rule. Set `FORCE_SECURE_COOKIES=true` so session cookies
  stay `Secure`, since McHarbor itself now sees plain HTTP. Once HTTPS has been toggled under
  **Settings → HTTPS**, the stored setting overrides `TLS_ENABLED`; check the log line
  `server listening` after a restart.

Clients negotiate TLS with the proxy, so the proxy's minimum TLS version is the one that
matters to them. `TLS_MIN_VERSION` then governs only the hop to McHarbor.

## Health checks and failover

In active/passive exactly one node answers `200` on `GET /api/readyz`. The proxy should send
traffic to that node and nowhere else:

| | Caddy | Traefik | NGINX (open source) |
|---|---|---|---|
| Active health check on `/api/readyz` | Yes, every 5 s | Yes, every 5 s | **No** |
| Passive: stop using a node that answered `503` | `unhealthy_status 503` for `fail_duration` | — | `max_fails=1 fail_timeout=15s`, with `http_503` in `proxy_next_upstream` |
| Retry a refused request on the other node | Dial failures, for `lb_try_duration` | — | Yes, including `POST`/`PUT`/`DELETE` (`non_idempotent`) |
| Traffic after a failover | Moves within one health interval | Moves within one health interval | See below |

A standby refuses every write with `503` before any handler runs, so retrying a non-idempotent
request on the other node cannot apply it twice. With request buffering off, NGINX cannot
replay a request whose body it has already started sending; that request fails once and the
client retries.

**NGINX open source has no active health checks** and cannot tell a standby from the active node
until a request fails. The example makes `mcharbor-node-2` a `backup` server, so while node 1 is
active every request goes to it. After a failover to node 2:

- writes sent to node 1 get `503`, NGINX marks node 1 failed for `fail_timeout` and retries on
  node 2, so they succeed;
- reads are **served by the standby** until a write marks it failed, because a standby answers
  reads. Environments behind a remote agent are unavailable there (agents attach to the
  active node only), and live views can show errors.

Recover by making node 1 active again (**Enterprise → High availability → Make active**), or by
moving the `backup` flag to node 1 and reloading NGINX. For automatic readiness-based routing
use Caddy, Traefik, HAProxy, NGINX Plus (`health_check uri=/api/readyz`), or a cloud load
balancer with an HTTPS health check on `/api/readyz`.

## Forwarding headers and trusted proxies

Every example sends `X-Forwarded-For`, `X-Forwarded-Proto`, `X-Forwarded-Host` and
`X-Real-IP`. Each one **replaces** whatever the client sent rather than appending to it:
Caddy and Traefik ignore incoming forwarding headers from untrusted clients by default, and
the NGINX example sets `X-Forwarded-For` to `$remote_addr` instead of the usual
`$proxy_add_x_forwarded_for`.

`MCHARBOR_TRUSTED_PROXY_CIDRS` lists the proxies whose forwarding headers McHarbor believes when
it resolves a client address: login throttling, the request log, audit rows, password-reset
records, API-key usage history, a policy's `request.ip`, and the address an agent appears to
connect from. Without it, every failed login behind the proxy counts against the proxy's address. Set it to the proxy's address or network: the Docker network's subnet, the
ingress controller's pod CIDR:

```
MCHARBOR_TRUSTED_PROXY_CIDRS=172.20.0.0/16
```

Empty is the default and means the socket peer (the proxy) is recorded as the client. Never
`0.0.0.0/0`, which lets any caller claim any address. A malformed value stops McHarbor from
starting.

The sign-in handler, the audit log and the request log read `X-Forwarded-For` without
consulting that list. That is why the proxy has to overwrite the header, and why McHarbor's
port should not be reachable except through the proxy: a client that can reach McHarbor
directly can put any address in those records.

Add the external URL to `ALLOWED_ORIGINS` (`https://mcharbor.example.com`), or the sign-in form
does nothing.

## Caddy

`deploy/proxy/caddy/Caddyfile`. Caddy obtains the certificate for `mcharbor.example.com` itself.

| Directive | Why |
|---|---|
| `lb_policy first` | All traffic to the first healthy upstream |
| `health_uri /api/readyz`, `health_interval 5s`, `health_status 200` | Active check; a standby is taken out of rotation |
| `unhealthy_status 503`, `fail_duration 10s` | Passive check, for the seconds between a failover and the next active check |
| `lb_try_duration 10s` | Retry another upstream while one is unreachable |
| `flush_interval -1` | Write every byte immediately, for server-sent events |
| `stream_close_delay 5m` | Keep WebSockets open across a Caddy config reload instead of cutting every terminal |
| `header_up X-Real-IP {remote_host}` | Caddy sets the `X-Forwarded-*` headers itself |
| `tls_insecure_skip_verify` | See [Upstream TLS](#upstream-tls) |

Caddy upgrades WebSockets, streams request bodies and applies no body limit by default, so none
of that needs configuring. Validate with:

```bash
caddy validate --config Caddyfile --adapter caddyfile
```

## Traefik

Two files, for the two Traefik providers. Use either.

**Docker provider**: `deploy/proxy/traefik/docker-compose.labels.yml` is a Compose overlay that
puts the labels on the `mcharbor` service and joins it to an external network named `traefik`:

```bash
docker network create traefik     # once, if your Traefik does not already use one
docker compose -f docker-compose.yml -f deploy/proxy/traefik/docker-compose.labels.yml up -d
```

For `docker-compose.ha.yml`, copy the same labels onto both `mcharbor-node-1` and
`mcharbor-node-2`: identical labels make Traefik merge both containers into one service with a
health check per server.

**File provider**: `deploy/proxy/traefik/dynamic.yml` defines the router, the service with both
nodes, and the `mcharbor-upstream` servers transport. The Docker labels refer to that transport
as `mcharbor-upstream@file`, because a servers transport cannot be declared in labels, so the
file provider is needed in both cases.

Both assume an entry point named `websecure` and a certificate resolver named `letsencrypt` in
Traefik's static configuration. Traefik 3 also has one static setting McHarbor needs: the entry
point's read timeout defaults to 60 seconds, which cuts off any upload that takes longer.

```yaml
entryPoints:
  websecure:
    address: ":443"
    transport:
      respondingTimeouts:
        readTimeout: 0
```

(`--entrypoints.websecure.transport.respondingTimeouts.readTimeout=0` on the command line.)

Traefik upgrades WebSockets, streams request bodies and sets `X-Forwarded-*` and `X-Real-Ip`
without configuration. `flushInterval: 100ms` bounds how long a streamed response can sit in
Traefik; responses with `Content-Type: text/event-stream` are flushed immediately.

## NGINX

`deploy/proxy/nginx/mcharbor.conf` goes into `conf.d/`: it holds a `map`, an `upstream` and two
`server` blocks, all in the `http` context. Certificates are expected at
`/etc/nginx/certs/fullchain.pem` and `privkey.pem`.

| Directive | Why |
|---|---|
| `map $http_upgrade $mcharbor_connection` | Sends `Connection: upgrade` only for WebSocket requests, so ordinary requests keep using upstream keep-alive |
| `proxy_http_version 1.1`, `Upgrade`, `Connection` | WebSocket upgrades |
| `client_max_body_size 0` | No body limit |
| `proxy_request_buffering off` | Stream uploads to McHarbor instead of spooling them to disk first |
| `proxy_buffering off` | Server-sent events and downloads reach the client as written |
| `proxy_read_timeout 3600s`, `proxy_send_timeout 3600s`, `client_body_timeout 3600s` | Long-lived WebSockets and streams, slow uploads |
| `proxy_next_upstream error timeout http_502 http_503 non_idempotent` | Move a refused request to the other node; see [Health checks and failover](#health-checks-and-failover) |
| `backup` on node 2 | Keep every request on node 1 while it is active |
| `proxy_ssl_verify off` | See [Upstream TLS](#upstream-tls) |

NGINX resolves the upstream names when it loads the configuration, so `nginx -t` fails with
`host not found in upstream` until they resolve.

## Related

- [Deployment](./README.md)
- [Kubernetes](./kubernetes.md#ingress): the same requirements as Ingress annotations
- [High availability](../../settings/system/high-availability/README.md)
- [Configuration](../configuration/README.md#network-trust)
