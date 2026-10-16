# General: Reference

| Setting | Default | What it does |
|---|---|---|
| **Application name** | "McHarbor" | The name shown in the interface |
| **Time format** | 24-hour | `24-hour (13:30)` or `12-hour (1:30 PM)` |
| **Date format** | `DD/MM/YYYY` | `DD/MM/YYYY (31/12/2025)` or `MM/DD/YYYY (12/31/2025)` |
| **Language** | English | Your preferred language, for your account |
| **Default application language** | English | "Language used by the McHarbor UI before a user signs in or when the user has not picked a preferred language" |
| **Auto-refresh interval (seconds)** | — | "How often list and detail views re-fetch data from the server" |
| **Time zone** | UTC | "Time zone used by schedules, retention windows, and timestamps rendered by the server". Changing it restarts McHarbor |

## What the refresh interval costs

Lower means fresher lists and more load on the Docker daemon: every refresh is API calls
per environment, and on a host with hundreds of containers that is not free. Live streams
(logs, terminal, events) are WebSockets and are not affected by this at all.

## Six locales

English, Dutch, German, Spanish, French and Portuguese. English is the source language; the
others fall back to it for any string not yet translated, so a partially translated locale
shows English rather than a missing key.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| Schedules run at the wrong hour | The time zone does not match the one you think in | Set it here; cron is read in this zone, not UTC and not the browser’s |
| The time zone cannot be changed here on a native install | McHarbor follows the host's time zone | `sudo timedatectl set-timezone <zone>` on the host, then `sudo systemctl restart mcharbor` |
| Changing the time zone disconnected everyone | It restarts McHarbor, by design | Expected; the dialog says so before it acts |
| The login page is in the wrong language | That is the default application language, not your personal one | Set the default separately |
| Lists feel stale | The auto-refresh interval is long | Lower it, knowing the Docker API cost per environment |

## Related

- [General](./README.md): what the section is for
- [Settings](../../README.md): the other groups
