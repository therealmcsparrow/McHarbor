# Docker: Troubleshooting

What the Docker screen will not show you, and why. Two of these are platform limits rather
than faults, and one is the difference between a registry that authenticates and an image
reference that points at it.

| Symptom | Usual cause | Fix |
|---|---|---|
| The Registries or Resources section is disabled | Missing `registry.view` or `host.view` | The tooltip names the key |
| A registry tests green but pulls fail | The image reference does not match the registry | An unqualified pull uses the default registry; qualify the reference |
| Hardware shows nothing | The host device inventory is not readable on this platform | The page says so; it is a platform limit, not a fault |
| Disk usage looks wrong after a prune | Figures are a point-in-time snapshot | Reload the page; reclaimable is recalculated per read |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Docker](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
