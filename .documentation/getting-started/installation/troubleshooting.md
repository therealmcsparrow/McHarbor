# Installation: Troubleshooting

What goes wrong bringing the container up, and where it is named. Almost every line in
this table also appears in the start-up log, which prints the entrypoint's own checks
(data directory, session secret, Docker socket and its group id, TLS certificate) before
the listeners open.

| Symptom | Usual cause | Fix |
|---|---|---|
| The login form submits and nothing happens | The browser's origin is not in `ALLOWED_ORIGINS`, so the preflight `OPTIONS` fails before the request reaches the handler | Add the exact URL you type, no trailing slash, and recreate the container |
| The browser refuses to connect over HTTPS | The certificate is signed by McHarbor's local root, which the browser does not know yet | Install the root CA on the machine you browse from, use the plain-HTTP listener on 8705, or upload a publicly-trusted certificate |
| The container will not start, with a CIDR error | A malformed `MCHARBOR_TRUSTED_PROXY_CIDRS` | A bad value fails start-up deliberately rather than degrading silently |
| Everything 401s after login | Cookies are being dropped: a proxy stripping them, or `Secure` cookies over plain HTTP | Set `FORCE_SECURE_COOKIES` to match how the browser actually reaches McHarbor |
| No environment appears | The Docker socket is not mounted, or is not readable by `PUID:PGID` | Check the mount and the socket's group id, which the start-up log prints |
| The terminal and live logs do nothing, everything else works | A proxy is not allowing WebSocket upgrades | Allow upgrades on the proxy |
| Port already in use | Something else holds 8705 | Change the host side of the mapping; the container port stays as it is |
| Data disappears after `docker compose down -v` | `-v` removes named volumes | The data volume is the install; back it up before experimenting |
| Health check flaps | The container is still applying migrations at start-up | Migrations run before the listeners open; wait out the start period and read the log |
| A rebuilt in-tree plugin still shows the old behaviour | Only the `.so` was replaced, or the image was not rebuilt | `docker compose build && docker compose up -d`; the entrypoint then refreshes the whole plugin payload |

## Reading the start-up log

```bash
docker compose logs mcharbor | head -50
```

The first fifty lines cover the entrypoint's own checks (data directory, session secret,
Docker socket and its group id, TLS certificate presence), then migrations, the listener
addresses, and the plugin loader. A failure in any of those is named there rather than
surfacing later as odd behaviour in the interface.

## Related

- [Installation](./README.md): the Compose file, ports, volumes and TLS
- [First run](../first-run/README.md): what happens after it starts
- [Configuration](../configuration/README.md): every environment variable
