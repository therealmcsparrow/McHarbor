# First Run: Troubleshooting

What goes wrong on the first run, and the order that avoids most of it. `ALLOWED_ORIGINS`
and the time zone are the two worth getting right before anything else: the first because
a wrong value locks you out of the screen that would fix it, the second because every
schedule written before it silently means something else.

| Symptom | Usual cause | Fix |
|---|---|---|
| The login form submits and nothing happens | The browser's origin is not in `ALLOWED_ORIGINS`, so the preflight fails before the request reaches the handler | Add the exact URL you are typing, no trailing slash, and recreate the container |
| The installer says the setup token is missing or out of date | The token changes every time McHarbor restarts, and applying a different database restarts it | Run `docker logs mcharbor` (container) or `journalctl -u mcharbor` (native install) and copy the newest `first-run setup token` line into the **Setup token** field |
| The installer refuses a SQLite path | A SQLite path must be inside the data directory | Use a path under `DATA_DIR`, or leave the field empty for the default file |
| Setup asks for a database when you expected SQLite | Neither a configured database nor `MCHARBOR_DB_DRIVER` | Choose SQLite on the form; it is the default and needs no input |
| The database test fails at "host reachable" | The database is not reachable from inside the container | Container networking, not credentials. The step that failed tells you which |
| The admin form rejects the password | Under eight characters | Minimum is eight |
| No environment appears after login | The Docker socket is not mounted, or not readable by `PUID:PGID` | Check the mount, and the socket group id in the start-up log |
| Every API call 401s | Cookies are being dropped | A proxy stripping them, or `Secure` cookies over plain HTTP. Set `FORCE_SECURE_COOKIES` to match reality |
| The browser will not open the page at all | The certificate is signed by a root the browser does not know | Install the root (see [Installing the root CA](../../screens/security/install-root-ca.md)), or use the plain-HTTP listener, or upload a publicly-trusted certificate |
| Schedules created on day one run at the wrong hour | The time zone was set after they were written | Set the time zone first; cron is read in it |
| Password reset produces no email | No email channel is selected under Security → Password | Links are issued but never sent until one is picked |
| An agent environment stays "Waiting for agent…" | The install command was never run, or the one-time URL expired | The install URL is single-use and expires after 24 hours |

## The order that avoids most of this

1. Set `ALLOWED_ORIGINS` before the first login.
2. Read the setup token from `docker logs mcharbor` (or `journalctl -u mcharbor`, or the native installer's output), choose the database, then create the admin account.
3. Set the time zone.
4. Deal with the certificate.
5. Then create environments, schedules and backup plans.

Steps 1 and 3 are the two that are annoying to fix afterwards: the first because you cannot
log in to change it, the third because every schedule written before it silently means
something else.

## Related

- [First run](./README.md): the setup flow and what to do next
- [Installation](../installation/README.md): ports, volumes and TLS
- [Configuration](../configuration/README.md): every environment variable
