# Store: Troubleshooting

What goes wrong installing from the Store. An empty tab means no catalog of that kind is
configured, a refused plugin means a signature or host-range check failed, and a missing
app is usually installed into a different environment.

| Symptom | Usual cause | Fix |
|---|---|---|
| A tab is empty | No catalog of that kind is configured | Settings → Integrations → Catalogs lists sources per kind |
| Install fails on a port | The host port is already bound | Change it on the Ports step of the wizard |
| A plugin will not install | Signature or host-range check failed | The error names the field; see the plugin section for trust keys |
| An installed app does not appear under Stacks | It installed into another environment | Use **View installations** to see which |
| An app using host networking or a device installs and starts, but does not work (a Thread border router no one can reach, a radio that is never found) | McHarbor deploys the container, but does not prepare the host: kernel settings, IPv6 on the LAN, device firmware | Follow the app's **Documentation** link for the host setup, and check the device path in its environment |
| A catalog you just added does not appear | The URL was already configured for that kind, so nothing was added | The refusal names the existing catalog; enable it under Settings → Integrations → Catalogs |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Store](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
