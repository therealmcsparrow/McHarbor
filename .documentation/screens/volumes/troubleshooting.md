# Volumes: Troubleshooting

What goes wrong with volumes. "Not reported" size is expected rather than an error, a
refused removal means something still mounts it, and prune is the one that loses data: an
anonymous volume holding a database is still an unreferenced volume.

| Symptom | Usual cause | Fix |
|---|---|---|
| Size shows "Not reported" | The daemon has not collected usage data for it | Expected. It is not an error and not a zero-byte volume |
| Remove is refused | A container still mounts it, or it is protected | Stop or remove the container; a protected volume cannot be removed at all |
| Prune deleted data | An anonymous volume was unreferenced at that moment | Back up before pruning; the Containers tab is the check to run first |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Volumes](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
