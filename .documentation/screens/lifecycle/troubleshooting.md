# Lifecycle Log: Troubleshooting

When the Lifecycle Log looks thinner than it should. Two of the causes are the date range
and the retention window rather than missing data, one is where an OOM kill is recorded,
and one is a plugin that is not installed.

| Symptom | Usual cause | Fix |
|---|---|---|
| "No lifecycle events match these filters" | The date range is narrower than you think | Widen the range before concluding nothing happened |
| A container’s history stops partway back | Retention purged the older rows | Lifecycle retention is in Settings → Monitoring → Data retention |
| An OOM kill is not obvious in the container | It is recorded here as `killed (OOM)` | Filter severity to Error and subject type to Container |
| Virtual machine or Storage pool is missing from the subject filter | The plugin that declares that subject type is not installed or not loaded | Install or enable Virtual Machines or Compute Foundation. Older rows are still listed under All subjects |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Lifecycle Log](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
