# Store

`/store`

Install applications, workflow nodes, dashboard widgets and plugins from a catalog.

## Permissions

| Key | What it unlocks |
|---|---|
| `store_apps.view` / `store_apps.manage` | Browse and install applications |
| `store_nodes.view` / `store_nodes.manage` | Browse workflow node types |
| `store_widgets.view` / `store_widgets.manage` | Browse dashboard widget types |
| `dashboard_widgets.install` | Install, update or remove a downloaded widget from the **Widget catalog** section of the Widgets tab (global, administrator only) |
| `plugins.view` / `plugins.manage` / `plugins.delete` | Install, update and remove plugins |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- Four tabs: **Apps**, **Nodes**, **Widgets** and **Plugins**.
- App cards showing the stack deployed, the image, ports, volumes, environment variables, minimum memory, and links to the project website and documentation.
- Node and widget catalogs, each entry badged **Built-in**, **Downloaded** or **Plugin**; a plugin-provided one names its source.
- On the Widgets tab, a **Widget catalog** section lists signed widget bundles from the local plugin catalog, each with **Install** or **Update** and, once installed, **Remove**. An entry that is not signed by a trusted key reads **Not signed by a trusted key** and cannot be installed. An installed widget runs in every dashboard viewer's browser, which is why installing one takes its own permission. See [Ship a downloaded widget](../../how-to/widgets/downloaded/README.md).

## Installing an app

Two paths, chosen on the first step:

- **Install with defaults**: one click, using the catalog’s values.
- **Customize**: a six-step wizard: Quick install, Ports, Volumes, Network, Environment,
  Review. Each step shows what the app declares, and says plainly when there is nothing to
  set ("No ports configured for this app").

Installing deploys a Compose stack, so the result appears under
[Stacks](../stacks/README.md) and its containers under
[Containers](../containers/README.md).

The same app can be installed into several environments. **View installations** lists each
one with its environment, owner and version, and **Uninstall** asks which: "Uninstall
*Nextcloud* from *production*?"

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Stacks](../stacks/README.md): what an app install produces
- [Plugins](../../plugins/README.md): the plugin system in full
- [Settings → Integrations → Catalogs](../../settings/integrations/catalogs/README.md): where the store reads from
