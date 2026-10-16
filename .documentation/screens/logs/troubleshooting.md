# Logs: Troubleshooting

What goes wrong reading container logs. Most of it comes down to where the lines actually
live: a non-default log driver ships them elsewhere, Docker rotates them away, and
McHarbor does not store them itself.

| Symptom | Usual cause | Fix |
|---|---|---|
| Nothing appears for a running container | A non-default Docker log driver | There is nothing local to read; check the daemon’s logging driver |
| The view stops updating | Live is off, or the WebSocket dropped | Toggle Live; if it will not attach, check that proxies allow WebSocket upgrades |
| Old lines are missing | Docker rotated them | McHarbor does not store container logs; forward them if they must outlive the container |
| The filter hides everything | It matches nothing in the loaded buffer | "No matching log lines" is literal; clear the filter or load more |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Logs](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
