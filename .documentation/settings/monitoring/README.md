# Monitoring

What McHarbor tells you about, and how long it remembers.

| Section | Covers | Gated by |
|---|---|---|
| [Alerts](./alerts/README.md) | Rules that watch metrics and container state, and where they deliver | `alerts.view`, `.manage`, `.delete` |
| [Data retention](./data-retention/README.md) | How long audit, activity, lifecycle, backup-log and scan history are kept | `settings.view` / `settings.manage` |

## How they relate

Alerts are about the present: something crossed a threshold, tell someone now. Retention is
about the past: how far back you can answer a question.

The two meet in an incident. An alert tells you a container is down; the history is what
lets you find out it has been restarting nightly for three weeks. Retention set for database
size alone tends to delete exactly the window an investigation needs.

## Related

- [Settings](../README.md): the other five groups
