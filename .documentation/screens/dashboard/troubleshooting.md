# Dashboard: Troubleshooting

When a widget is blank or disagrees with a screen. Usually one of three things: a
permission or feature the widget's data needs, a layout that belongs to a different user,
or a widget aggregating across environments while you are looking at one.

| Symptom | Usual cause | Fix |
|---|---|---|
| A widget shows nothing | The data it reads needs a permission you lack, or a feature that is not configured | Alerts, vulnerabilities and snapshots all need their feature set up first |
| "Could not be shown" with a retry | That widget’s data call failed | Retry; the rest of the grid is unaffected by design |
| Someone else sees a different dashboard | Layouts are per user | Expected; share a layout by describing it, not by editing theirs |
| Numbers differ from the Containers screen | The widget is aggregating across environments | Check the environment switcher |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Dashboard](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
