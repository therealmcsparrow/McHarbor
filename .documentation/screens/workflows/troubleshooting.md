# Workflows: Troubleshooting

What goes wrong on the Workflows screen. Inactive is the first thing to check (it
disables automatic triggers only), and after that it is usually a node dependency that is
not configured, or a path that does not match what the previous node emitted.

| Symptom | Usual cause | Fix |
|---|---|---|
| A workflow never fires on its own | It is Inactive | Activate it; inactive disables auto-triggers only |
| A node is greyed out in the palette | Its dependency is not configured | The node states which: an email server, a communication channel, or an environment |
| "No runs yet" on a scheduled workflow | The trigger never fired | Check Active, then the schedule trigger’s cron expression and the configured time zone |
| A branch never executes | The node before it is disabled, or the port is blocked | Disable stops the chain; Skip passes through |
| A value is missing downstream | The path does not match what the previous node emitted | Read the Debug tab on the receiving node to see the real `msg` |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Workflows](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
