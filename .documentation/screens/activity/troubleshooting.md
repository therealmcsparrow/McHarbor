# Activity: Troubleshooting

When Activity looks empty or stops moving. All three causes are ordinary rather than
faults: a quiet host, a retention window that has passed, or a stream that dropped.

| Symptom | Usual cause | Fix |
|---|---|---|
| "No events recorded yet" | A quiet host, or a fresh install | Literal rather than a fault; act on something and watch it appear |
| Old events are gone | Retention | Activity retention is set in Settings → Monitoring → Data retention |
| Events stop arriving | The stream dropped, or an agent environment is polling rather than streaming | Reload; for agents, event mode and poll interval are in Settings → System → Agent |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Activity](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
