# Notifications: Troubleshooting

When the inbox stays empty. In-app delivery is a per-rule destination rather than a global
setting, so an alert can reach chat and never reach here.

| Symptom | Usual cause | Fix |
|---|---|---|
| Nothing ever arrives | No alert rule delivers in-app, and nothing system-level has happened | Check that a rule has **In-app notifications** among its destinations |
| An alert reached chat but not the inbox | In-app was not selected as a destination | Destinations are per rule; in-app is one of them |
| The unread count will not clear | Notifications are still arriving | **Mark all read** clears what exists at that moment |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Notifications](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
