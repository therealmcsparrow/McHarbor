# Store: Actions

Everything you can do in the Store (sync the catalog, install and uninstall an app, and
install, update or remove a plugin) and the permission each one needs. A plugin install
verifies the signature and runs its migrations before it loads.

| Action | Gated by | What it does |
|---|---|---|
| **Sync** | `store_apps.manage` | Refreshes the catalog from its source |
| **Install** | `store_apps.manage` | Defaults, or the six-step wizard |
| **View installations** | `store_apps.view` | Every environment this app is installed in |
| **Uninstall** | `store_apps.manage` | Names the app and the environment before removing it |
| **Install plugin** | `plugins.manage` | Verifies the signature, runs migrations, loads the plugin |
| **Update plugin** | `plugins.manage` | Installs a newer version in place |
| **Reconcile** (installed plugin card) | `plugins.manage` | Pins the plugin to the catalog version shown on its card and keeps its enabled state. The host keeps, moves or reinstalls it until the record, files and binary agree; role grants are kept. The toast says whether it was already at that version, moved to it or installed. The CLI equivalent is `mcharbor plugin reconcile` |
| **Uninstall plugin** | `plugins.delete` | Withdraws its routes, menu, widgets, nodes and permission keys |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Store](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
