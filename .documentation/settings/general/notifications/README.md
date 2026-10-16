# Notifications

**Settings → General → Notifications** · `?tab=notifications`

Where toasts appear, how long they stay, and what users may override.

## Permissions

| Key | What it unlocks |
|---|---|
| `notifications.view` | Read the defaults |
| `settings.manage` | Change them or reset overrides |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- Toast position, duration, stacking limit, colouring and close button.
- Whether individual users may override those defaults.
- A preview that raises sample toasts with the current settings.
- A **Web push** card with the instance's VAPID key pair (below).

## This is about toasts

Toasts are the transient confirmations of what you just did. The durable inbox is
[Notifications](../../../screens/notifications/README.md), and alert delivery is
[Monitoring → Alerts](../../monitoring/alerts/README.md).

## Web push

The **Web push** card holds the one administrative push action. Browser and mobile push
notifications share a single VAPID key pair for the whole instance, and **Regenerate VAPID key**
replaces it. It needs `settings.manage` and is disabled with a tooltip naming the key without it.

The confirmation says what it costs before anything happens: regenerating **removes every device
subscription on this instance**, each device stops receiving pushes until its owner turns push on
again, and it cannot be undone. On success the toast reports how many subscriptions were removed.
The action is audited at warning severity. Its route is `POST /api/push/vapid/regenerate`; see
[Push endpoints](../../../api/endpoints/push.md) and [On-call push](../../../oncall/push.md).

## Choosing a duration

Toasts report outcomes, and the outcomes that matter most are failures. Two seconds is fine
for "Container started" and useless for "Backup failed: destination unreachable", which is
the one someone needs to read.

Long durations with a low maximum-visible is usually the better trade: fewer on screen,
each for long enough to read. Anything that must survive being missed belongs in the
notification inbox or an alert delivery, not in a toast.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Notifications](../../../screens/notifications/README.md): the durable inbox
- [Monitoring → Alerts](../../monitoring/alerts/README.md): rules and their delivery
- [Themes](../themes/README.md): the other section with an override lock
