# Notifications

`/notifications`

The durable inbox for alerts, automation results and system activity. Toasts are transient;
this is not.

## Permissions

| Key | What it unlocks |
|---|---|
| `in_app_notifications.view` | Read your inbox |
| `notifications.view` / `notifications.manage` | The notification configuration in Settings |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- One row per notification: level, title, message, type, status and date.
- **Search title and message**: contains, exact or regex (RE2, 256 characters at most).
  Search, the **All / Unread / Read** chips and the level filter run on the server, so they
  cover every page, not only the one on screen; changing any of them returns to page 1. A
  regex search reads the most recent 1000 matching notifications.
- An unread count on the bell in the top bar, with a short preview and **View all notifications**.

## Where these come from

| Type | Produced by |
|---|---|
| **Alert** | Alert rules in Settings → Monitoring → Alerts |
| **Container** | Container lifecycle events you asked to be told about |
| **System** | McHarbor itself: update available, backup failed, licence state, schedule error |

An in-app notification is one delivery target among several. The same alert can also go to
a chat channel or an inbox through a communication channel. In-app costs nothing to enable
and survives a missed toast.

## Toasts are a different thing

Toasts report what *you* just did, and disappear. If something matters after the tab is
closed, it needs to be a notification or an alert delivery, not a toast.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Settings → Monitoring → Alerts](../../settings/monitoring/alerts/README.md): the rules that produce alerts
- [Settings → General → Notifications](../../settings/general/notifications/README.md): toast defaults
- [Communication channel plugins](../../plugins/communication/README.md): delivering outside McHarbor
