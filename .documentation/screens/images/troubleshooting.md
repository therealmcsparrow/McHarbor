# Images: Troubleshooting

What goes wrong with images. Two of these are reference-counting rather than faults
(prune takes what nothing points at, and an image will not go while a container still uses
it), and two are configuration, in Docker → Registries and Security → Scanners.

| Symptom | Usual cause | Fix |
|---|---|---|
| Prune removed an image you wanted | Nothing referenced it at that moment | Pull it again; prune only removes unreferenced images, including dangling ones |
| A pull fails with unauthorized | No registry credential, or the wrong default registry | Add or fix the registry under Docker → Registries |
| An image will not remove | A container still references it | Remove the container first, or remove the image with the container |
| Scan does nothing | No scanner plugin installed or configured | Install one and configure it under Security → Scanners |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Images](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
