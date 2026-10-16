# Security posture for an internet-facing host

McHarbor's defaults are chosen for a host on a LAN: they never lock you out, and they work
on first start with no reverse proxy, no domain and no certificate. Several of them are
wrong for a host the internet can reach. McHarbor does not change them for you; it tells
you, at boot, which ones are still in force.

Keep one fact in view while reading the rest: **McHarbor is root-equivalent control of a
Docker host.** Everything on this page changes who can reach it. None of it changes what it
can do once reached.

## The posture warnings

Every start logs one `warn` line per default that is still in force, each prefixed
`security posture:` and carrying the variable that addresses it in the `env` field, so
`docker logs mcharbor 2>&1 | grep 'security posture'` lists them all.

| `env` field | Logged when | What to do |
|---|---|---|
| `AUTH_DISABLE` | `AUTH_DISABLE=true` | Unset it. Anyone who can reach the port controls the Docker host. Logged first, and in addition to the older `authentication is disabled` line |
| `TLS_ENABLED` | McHarbor is serving TLS itself with a certificate from its own local CA | Terminate TLS at a reverse proxy with a publicly trusted certificate and set `TLS_ENABLED=false`, or upload a certificate under **Settings → Security → HTTPS** |
| `HSTS_MAX_AGE` | McHarbor is serving TLS and `HSTS_MAX_AGE` is `0` | Set `HSTS_MAX_AGE=31536000`, once the certificate is one browsers trust, never before |
| `ALLOWED_ORIGINS` | `ALLOWED_ORIGINS` is empty, so only the built-in `localhost` origins are allowed | Set it to the URL the UI is served from, e.g. `https://mcharbor.example.com` |
| `MCHARBOR_PLUGIN_REQUIRE_SIGNATURE` | The variable is unset or not truthy | Set `MCHARBOR_PLUGIN_REQUIRE_SIGNATURE=true`; see [Plugin signing](#plugin-signing) |

The certificate and HSTS checks read the TLS state the server actually settled on, after the
**Settings → Security → HTTPS** setting in the database overrides the environment. A host with
`TLS_ENABLED=false` behind a proxy gets neither.

### The forwarded-header warning

One more warning is logged at runtime rather than at boot, and **at most once per process**:
`security posture: a request arrived through a proxy, but no proxy is trusted`, with
`env` set to `MCHARBOR_TRUSTED_PROXY_CIDRS` and `header` naming the header that tripped it.

It fires the first time a request carries `X-Forwarded-For`, `Forwarded` or `X-Real-IP`
while `MCHARBOR_TRUSTED_PROXY_CIDRS` is empty. That is the signature of a reverse proxy
nobody declared: McHarbor ignores its headers (correctly, since it cannot tell your proxy
from a client lying about its address), so every login attempt, audit row and API-key use is
attributed to the proxy. Set `MCHARBOR_TRUSTED_PROXY_CIDRS` to the proxy's address or
network and restart. Never set it to `0.0.0.0/0`, which believes every caller's claim about
itself. See [Configuration](../../configuration/README.md) for how the header chain is read.

### What the warnings do not do

They change no default and block nothing. A host that logs all five still starts and serves.
`HOST=0.0.0.0` in particular is not warned about: a container that binds loopback is
unreachable, so it is correct inside a container; on a bare-metal install behind a local
proxy, set `HOST=127.0.0.1` yourself.

## Recommended settings

For McHarbor behind a reverse proxy that terminates a real certificate:

```bash
MCHARBOR_TRUSTED_PROXY_CIDRS=<the proxy's address or CIDR>
ALLOWED_ORIGINS=https://mcharbor.example.com
HSTS_MAX_AGE=31536000
FORCE_SECURE_COOKIES=true
MCHARBOR_PLUGIN_REQUIRE_SIGNATURE=true
MCHARBOR_PLUGIN_TRUST_KEYS=/path/to/trusted.pem
TLS_ENABLED=false
```

`TLS_ENABLED=false` belongs only where the proxy terminates TLS and reaches McHarbor over a
network nobody else is on. `AUTH_DISABLE` must not be set at all. On a bare-metal install
add `HOST=127.0.0.1` so the only way in is through the proxy.

Proxy configurations for Caddy, Traefik and NGINX are in [Reverse proxy](../reverse-proxy.md).

## Plugin signing

A plugin runs as native Go code inside the McHarbor process, with the same access to the
Docker host as McHarbor itself. Whether a plugin was signed by a key you trust is therefore
the only provenance check between a tarball and root on the host.

**Default behaviour.** With `MCHARBOR_PLUGIN_REQUIRE_SIGNATURE` unset, an unsigned plugin
(no signature, or one no configured key verifies) installs only after an explicit
confirmation:

- In **App Store → Plugins**, the install dialog names the plugin, says it runs with the
  same host access as McHarbor, and keeps the install button disabled until the risk
  checkbox is ticked. When the catalog claimed a signature the server could not verify, the
  dialog reopens in the same state with the button labelled **Install unsigned plugin**.
- The server enforces it. `POST /api/appstore/plugins/install/stream` and
  `POST /api/appstore/plugins/{slug}/update/stream` refuse an unsigned tarball unless the
  body carries `"allow_unsigned": true`. That code, `err.plugin.unsigned_confirmation_required`,
  arrives as the `error_code` of the error event. The
  update body is optional; without one, only a signed release is accepted.
  `POST /api/appstore/plugins/{slug}/reconcile` takes the same field and, without it, fails
  with `409` `err.plugin.update_failed`; `data.slug` names the plugin and the reason is in the
  server log, never in the response.
- An update or a reconcile that moves an installed plugin checks the signature **before** it
  uninstalls the current version, so a refusal leaves the plugin as it was. A boot-time
  repair reinstalls an unsigned plugin you already accepted, but will not quietly replace a
  signed install with an artifact that no longer verifies.
- From the CLI: `mcharbor plugin install <slug> --allow-unsigned`, and the same flag on
  `plugin update`.
- An installed unsigned plugin carries an **Unsigned** badge on its card in the App Store,
  and its shield icon is the installed state, not the catalog's.

All of these routes stay gated by `plugins.manage`; the confirmation is in addition to it.

**Strict behaviour.** With `MCHARBOR_PLUGIN_REQUIRE_SIGNATURE=true`, an unsigned tarball is
refused outright with `err.plugin.signature_invalid`, whatever the request says. Set it on
any internet-facing host. With it set and no trust key configured, every install fails, and
the host logs that combination as a misconfiguration.

**Trusting a key.** `MCHARBOR_PLUGIN_TRUST_KEYS` names a PEM file holding one or more
ed25519 public keys (`PUBLIC KEY` or `ED25519 PUBLIC KEY` blocks). The shipped image points
it at the project's own key, so first-party plugins verify out of the box. To trust your own
publisher key as well, generate a pair and add the public half to that file:

```bash
openssl genpkey -algorithm ed25519 -out plugin-signing.key
openssl pkey -in plugin-signing.key -pubout -out plugin-signing.pub
```

Keys are read once at start, so adding one means a restart. Signing a tarball is covered in
[Packaging and publishing a plugin](../../../how-to/plugin/packaging.md#sign); the trust
model in [Plugin architecture](../../../development/plugins/architecture.md#trust-and-signing).

## The Docker socket

The shipped Compose files mount `/var/run/docker.sock` with `:ro`. That flag applies to the
bind mount, not to the Docker API: it stops the socket file being replaced and restricts
nothing a client can ask the daemon to do. It is not a hardening measure and should not be
counted as one. See [Compose](../compose.md#the-docker-socket).

## Beyond configuration

- **Put an authentication layer in front.** An identity-aware proxy or a VPN is the single
  highest-leverage control for an internet-facing McHarbor.
- **Enrol MFA on every administrator.**
- **Rate-limit `/api/auth/login` at the proxy** as defence in depth, alongside McHarbor's own
  throttling and lockout.
- **Keep the agent transfer listener (`:8788`) off the public interface.** It runs in the
  agent container and has no reason to be routable.

## Related

- [Deployment](../README.md)
- [Reverse proxy](../reverse-proxy.md)
- [Configuration](../../configuration/README.md): every environment variable
- [Plugin architecture: trust and signing](../../../development/plugins/architecture.md#trust-and-signing)
- [CLI](../../../cli/README.md)
