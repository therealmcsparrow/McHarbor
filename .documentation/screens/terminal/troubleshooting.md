# Terminal: Troubleshooting

When a terminal will not open or will not stay open. Two of these are outside McHarbor (a
proxy that blocks WebSocket upgrades, and an image with no shell in it), and two are
state: a stopped container, or an agent too old for interactive exec.

| Symptom | Usual cause | Fix |
|---|---|---|
| Connects, then immediately disconnects | A proxy is not allowing WebSocket upgrades | Allow upgrades on the proxy, or reach McHarbor directly |
| The shell list is empty | A distroless or scratch image has no shell | A property of the image, not of the terminal. Use the Exec tab with an explicit command if one exists |
| Nothing happens on an agent environment | Agent older than v1.1.0 | Interactive exec needs the dedicated agent messages; update the agent |
| The container is not in the list | It is not running | Only running containers can be attached |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Terminal](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
