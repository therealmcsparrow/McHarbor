# Notifications: Actions

Everything you can do to an in-app notification (read it, open what it is about, delete
it, or clear the count in bulk), all of it behind one permission,
`in_app_notifications.view`.

| Action | Gated by | What it does |
|---|---|---|
| **Mark as read** | `in_app_notifications.view` | Clears one from the unread count |
| **Mark all read** | `in_app_notifications.view` | Clears the count |
| **Open item** | `in_app_notifications.view` | Jumps to whatever the notification is about |
| **Delete** | `in_app_notifications.view` | "Remove *Backup failed* from your inbox?" |

## Bulk actions

Select rows and the bulk bar reports "N of M selected" with the actions that apply.
Progress is reported as it runs ("12 of 20 complete (11 succeeded, 1 failed, 0 running)"),
so a partial failure stays visible instead of being rolled into one toast.

| Action | What it does |
|---|---|
| **Mark as read** | Applied across the selection |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Notifications](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
