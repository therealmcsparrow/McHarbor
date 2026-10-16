# Integrations

What McHarbor is wired up to. Every section in this group is backed by plugins, and each one says so plainly when none is installed.

| Section | Covers | Gated by |
|---|---|---|
| [AI](./ai/README.md) | Provider connections for AI-backed features | `settings.view` or `ai_assistant.manage` |
| [Communications](./communications/README.md) | Email and chat channels alerts and events deliver to | `communications.view` / `.manage` |
| [Storage](./storage/README.md) | Backup and export destinations, and the backup encryption key | `settings.*` or `storage_locations.*` |
| [Snapshots](./snapshots/README.md) | Filesystem snapshot drivers per environment | `settings.manage` |
| [Logging](./logging/README.md) | Forwarding audit and application logs outward | `audit.view` **and** `settings.manage` |
| [Catalogs](./catalogs/README.md) | Where the App Store reads app, plugin and theme definitions from | `settings.manage` |

## The shape they share

The host owns the screen; the plugin owns the work. In every one of these sections you
create a row, fill in the fields the plugin declares, and press **Test**, which performs a
real operation against the real system rather than validating the form.

Credentials are encrypted at rest with AES-256-GCM and are never returned by the API once
saved. Editing an existing entry shows "stored — leave empty to keep" rather than asking
you to retype a secret.

## Order of setup

1. [Catalogs](./catalogs/README.md), if the plugins you need are not in the shipped catalog.
2. Install the plugins from [Store](../../screens/store/README.md).
3. Configure the connection here.
4. Point something at it: a backup plan, an alert rule, the assistant.

## Related

- [Settings](../README.md): the other five groups
