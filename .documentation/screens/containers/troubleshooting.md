# Containers: Troubleshooting

What goes wrong on the Containers screen. Several of these are the product working as
designed rather than failing: editing is locked until you confirm, removal leaves volumes
behind, and a container in a Compose stack reverts to what the stack file says.

| Symptom | Usual cause | Fix |
|---|---|---|
| A container disappears from the list | It was removed, or the environment switched | Check the environment picker first; then the Lifecycle log for a `destroy` event |
| **Edit** is greyed out | Editing is locked by default | Unlock editing and accept the confirmation; saving recreates the container |
| An edit reverts after a deploy | The container belongs to a Compose stack | Change the stack file instead; the container page warns about this |
| Logs are empty on a running container | A non-default Docker log driver ships logs elsewhere | Nothing local exists to read; check the daemon's logging driver |
| The terminal connects and drops | A proxy is not allowing WebSocket upgrades | Allow upgrades, or reach McHarbor directly |
| The terminal will not open through an agent | Agent older than v1.1.0 | Interactive exec needs dedicated agent messages; update the agent |
| The terminal says `stat /bin/sh: no such file or directory` | A distroless or `scratch` image has no shell | Use **Debug shell** on the Terminal tab (`containers.debug_shell`). See [Debug shell](./debug-shell/README.md) |
| Removing a container leaves its data | The volume checkbox was left off | Intended default. Remove the volume separately once you are sure |
| Stats and history are empty on an agent host | Metrics collection for agent environments is turned off, or the agent is being pulled and has too many containers for one sweep | Turn it on in Settings → System → Agent; if it is on, check the environment's **Container stats** card and update the agent so it pushes |
| Live CPU and memory columns are blank for a minute after McHarbor starts | An environment on the pull path is still taking its first sweep | Wait; the next poll after it finishes fills them |
| The Devices Host Path is a text box with a disabled **Choose from list** | You lack `host.view`, or the host's devices could not be listed | Ask for `host.view`, or type the path. See [Devices](./devices/README.md) |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Containers](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
