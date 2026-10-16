# Hooks

The **hook points** are where the product can be extended without editing the host. A plugin
attaches to the backend through registries it is handed at load time, in
`src/backend/core/plugin/contract/`. It attaches to the interface through mount points the host
renders, in `src/frontend/core/plugins/`. This page lists every hook point on both sides, what a
plugin hands it, and what the host does with it. The Go types and rules that make up the
agreement, with signatures and examples, are in [Contracts](../contracts/README.md).

## Quick start

1. In Go, implement `Plugin` and register what you contribute in its `Register*` methods (below).
2. Declare the same routes, menu entries, widgets, nodes and permissions in `plugin.yaml`. The
   manifest is what the host mounts and gates.
3. In the UMD bundle, export `mount`, and `widgets[key]` / `tabs[key]` entries where you
   contribute them, each as `{ mount, unmount?, update? }`.
4. Read host services from `window.McharborHost`. Never import a host store.

## Backend hook points

The loader calls these once, in this order, when the plugin loads.

| Method | Registry | What a plugin registers | Where it appears |
|---|---|---|---|
| `RegisterRoutes(Registry)` | `Public` / `Auth` / `Protected(method, path, handler)` | HTTP handlers under `/api/`, conventionally `/api/<slug>/…` (the Reporting add-on uses `/api/reporting/…`), in one of the router's three middleware bands | Reached through the host's `/api` plugin dispatcher, wrapped in `PluginPermissionMiddleware` |
| `RegisterMenu(MenuRegistry)` | `Add(MenuItem)`, `Remove(labelKey)` | Sidebar entries: `Parent` (a section id of the plugin's own), `LabelNS`/`LabelKey`, `Path`, `Icon`, `Required`, `Priority` | The sidebar. Entries sharing a `Parent` form a section labelled `<LabelNS>:nav.section.<Parent>`. An empty `Parent` goes to the generic "Plugins" section. |
| `RegisterWidgets(WidgetRegistry)` | `Add(key, WidgetDescriptor)` | Dashboard tiles: title, category, default size, settings (description, icon, minimum size) | The dashboard widget picker, as `plugin:<slug>:<key>` |
| `RegisterNodes(NodeRegistry)` | `Add(key, NodeDescriptor)` | Workflow nodes: category, display name, icon, config schema, ports, required permission | The workflow editor's palette. A node with no `Execute` is a trigger the plugin fires itself. |
| `RegisterPermissions(AccessRegistry)` | `AddPermission`, `InstallABACRule`, `RegisterSCIMSchema` and their removers | Its own permission keys, ABAC rules and SCIM schemas | The role editor and the permission catalog. Direct writes to `rbac_*` tables are refused at install. |
| `Init(ctx, HostContext)` | n/a | Takes the host services it needs (below) | n/a |
| `Shutdown(ctx)` | n/a | Releases what `Init` took | n/a |

Optional interfaces, which the loader type-asserts for, so a plugin that does not need one
carries no empty method:

| Interface | Method | What it adds |
|---|---|---|
| `TabProvider` | `RegisterTabs(TabRegistry)` | A tab on a host page. `TabItem{PageID, Key, LabelNS, LabelKey, Icon, Required, Priority}`, where `PageID` is `settings`, `security`, `environment` or `container`. |
| `AdapterContributor` | `RegisterAdapters(AdapterRegistry)` | A runtime handler for one of the host's pluggable families (below) |
| `CLIProvider` | `RegisterCLI(CLIRegistry)` | A command group for the `mcharbor` CLI, as declarations that point at the plugin's own routes. See [The plugin contract → CLIProvider](../contracts/plugin.md#cliprovider). |
| `SCIMHandler` | returned from `Init` | Declared in `access.go` for SCIM operations on a plugin's own resource types. Nothing dispatches it yet. The comment names `modules/scim/dispatcher.go`, which does not exist. |

### Adapter families

`AdapterRegistry` hands out one sub-registry per family. A plugin calls only the ones it takes
part in. The spec types live in `src/backend/core/plugin/specs/<family>/`.

| Registry | Family | The host keeps | The plugin supplies |
|---|---|---|---|
| `Storage()` | Backup destinations | Locations, credentials (encrypted), plans | Read, write, list and delete against one kind of store |
| `Notification()` | Communication channels | Channels, routing, retries | Delivering one message |
| `Identity()` | Identity providers | Users, sessions, group mapping | The sign-in exchange with one provider |
| `Snapshot()` | Snapshot drivers | Backup consistency modes | A point-in-time snapshot on one storage system |
| `AI()` | AI providers | The AI router, consent, prompts | One chat completion |
| `Logging()` | Audit log forwarders | The audit trail | Shipping entries to one collector |
| `Alerting()` | Alert condition sources | Scheduling, thresholds, deduplication, delivery | The conditions for a resource the plugin owns |
| `Reporting()` | Report providers | n/a, the consumer is the Reporting add-on | Report types and their builders |
| `Scanner()` | Image scanners | Scan rows, findings, retention, history | Turning an image reference into a CVE list |
| `Backup()` | Backup sources | The Backups page and its log | Plans, runs and restore for what the plugin owns (VMs, system containers) |

### Host services: `HostContext`

What `Init` receives. Every handle type lives in `src/backend/core/plugin/services/`, so host and
plugin builds share one reach set. A field documented as possibly `nil` must be checked, never
dereferenced blind.

| Field | Gives the plugin | May be nil |
|---|---|---|
| `Logger` | `Info` / `Warn` / `Error` / `Debug` into the host log | |
| `DB` | SQL scoped to the plugin's own `plugin_<slug>_` tables. Anything else is `ErrScopeViolation`. | |
| `Encryption` | AES-256-GCM encrypt and decrypt with the host key | |
| `DockerPool`, `AgentPool` | Docker clients per environment, local and agent | |
| `I18n` | Server-side translation | |
| `Audit` | Writing audit entries | |
| `Environments` | An environment id resolved to its display name | |
| `BackupLog` | Lifecycle events into the Backups page's log | yes |
| `Lifecycle` | State changes onto the Logging timeline | yes |
| `RBAC` | Permission checks | |
| `Identity`, `Sessions` | Provisioning an externally authenticated user, and issuing their session | |
| `AI` | The host's AI router, whichever provider plugin serves it | |
| `SnapshotRPC` | Snapshot operations on an agent host | yes, when there is no agent transport |
| `AgentRPC` | The agent message bridge, asserted against a local interface (see [Agent RPC](../contracts/agent-rpc/README.md)) | yes, without the `agent.rpc` token and an `agent-rpc` capability |
| `Storage` | Writing files to a configured storage location, by id. No credential crosses. | yes |
| `Reporting` | The consumer side of the report-provider family | yes |
| `Comms` | Sending a message, with files, to a configured channel, by id | yes |
| `Catalogs` | Reading the enabled catalog sources of a kind the plugin contributes to | yes, unless the manifest declares `catalog.source.register` |
| `Workflows` | Firing the plugin's own trigger nodes | yes |
| `WorkDir` | The host data directory, for a scratch subtree of the plugin's own | |

## Frontend hook points

At boot, `src/frontend/core/plugins/loader.ts` fetches every contribution table from
`/api/plugins/loader/*` once, before the router mounts, and caches them in a Zustand store. The
host then renders each table at its mount point:

| Mount point | Table | Host component | The bundle exports |
|---|---|---|---|
| Plugin page `/plugins/:slug/*` | `routes` | `PluginPageHost.tsx` | `mount(el, ctx)` returning a cleanup, plus optional `unmount(el)` and `onNavigate(ctx)` |
| Sidebar entry | `menu` | the sidebar, via `usePluginMenu()` | nothing: a link to the plugin's `path` |
| Topbar button | `topbar` | `bridge-topbar.tsx`, portalled into `#header-slot` (`left`) or `#header-right-slot` (`right`) | nothing: a button to `path`, default `/plugins/<slug>`. `visibility: "permission:<key>"` hides it. |
| Tab on a host page | `tabs` | `PluginTabPanel.tsx`, listed by `usePluginPageTabs(pageId)` | `tabs[key] = { mount(el, ctx), unmount? }` |
| Dashboard widget | `widgets` | `bridge-widgets.ts` and `PluginWidgetMount.tsx` | `widgets[key] = { mount, unmount?, update? }` |
| Workflow node | `nodes` | the node registry, via `usePluginNodes` | the node's editor component |
| Form slot in a host dialog | `forms` | `PluginFormOutlet.tsx`, `ProviderFormOutlet.tsx` | `mount(el, ctx)` for the `(resource, kind)` pair |
| Theme | `themes` | the appearance settings | a CSS file at `css_path` |
| Translations | `i18n?lang=` | `pluginI18n.ts`, merged once into i18next | the plugin's namespaces |
| Icons and assets | `icon`, `asset` | `asset-url.ts`, `PluginAssetImage.tsx` | files under the plugin's `icons/` |
| Context menu entry | `context-menus` | fetched and counted in the App Store's plugin details. No host page renders them yet. | n/a |

Every mount is **imperative**: the host renders an empty element and calls the bundle's `mount`.
A plugin bundle compiles its own React, so a component it exports cannot be rendered inside the
host tree. Every hook it called would reach the bundle's own dispatcher and fail with "Invalid
hook call".

### The contexts a mount receives

| Mount | Context |
|---|---|
| Page | `PluginNavContext`: `slug`, `path`, `splat` (the path below `/plugins/<slug>/`), `search`, and `navigate(to, { replace?, preventScrollReset? })`. The bundle mounts once for the whole subtree, and `onNavigate` delivers each later location change. |
| Tab | `PluginTabContext`: `pageId`, `tabKey`, and `entityId` when the page is about one entity |
| Widget | the dashboard's `navigate`. `update` is called when the tile is resized. |
| Form | `value` and `onChange` (a `Record<string, string>`), `onError(field, message)`, `readOnlyFields`, `i18nPrefix`, and `mode`. On `edit` the host strips secret values, so a blank secret field keeps the stored value. |

### Form slots

`FormDecl` in the manifest names a `resource` and a `kind`:

| Resource | Kinds |
|---|---|
| `channel`, `email_server`, `identity_provider`, `storage_location`, `ai_provider` | `config_form` (edits the value map), `setup_guide` (read-only help beside the form), `selector` (keyed by `key`) |

A host dialog embeds one outlet per slot. With no plugin for that slot, the outlet renders its
`children` fallback.

### Host services: `window.McharborHost`

What the host publishes for bundles, declared in `hostBridge.ts`. The bundle has its own copies
of React, react-i18next and TanStack Query, so anything that lives in a React context or a module
singleton does not cross the boundary unless it is published here.

| Field | Gives the plugin | Published by |
|---|---|---|
| `i18n` | The host's i18next instance | `core/i18n/i18n.ts` |
| `permissions` | `snapshot()` and `subscribe(listener)` for the current user's permissions, so a grant or revoke reaches plugin UI at the same moment it reaches the host | `core/auth/permissionsBridge.ts` |
| `environment` | `snapshot()` / `subscribe()` for the selected environment id, and `list()` / `subscribeList()` for every environment as `{ id, name, connectionType }` | `environmentBridge.ts` |
| `toast` | `success`, `error`, `info`, `warning` | `toastBridge.ts` |

A plugin must not import `@resources/stores/environment`. It would get a second store that
hydrates from the same `localStorage` key, looks right on first paint, and then keeps answering
for the old environment after the user switches.

## Shared React hooks

`src/frontend/resources/hooks/` holds the host's shared React hooks (about 106 files, listed by area in [Frontend → Data and state](../frontend/data-and-state.md#shared-hooks)), one per server
resource or behaviour. Examples are `useContainers`, `useImages`, `useAlerts`, `usePermissionGate`,
`useFocusTrap` and `useHeaderSlot`. Rules for them:

- A hook two modules need lives here, never copied between modules. A module never imports
  another module's hook.
- Server state is TanStack Query. The `queryFn` ends in `assertSuccess`, so a failed envelope
  reaches `isError`.
- A hook that subscribes returns its cleanup.
- These hooks are for the host. A plugin bundle calling one gets its own copy of the module and
  of every store behind it. That is why `window.McharborHost` exists.

## Where to look in the source

| Side | Path |
|---|---|
| The Go contract | `src/backend/core/plugin/contract/{contract,access,cli,manifest}.go` |
| Adapter family specs | `src/backend/core/plugin/specs/<family>/specs.go` |
| Host handles | `src/backend/core/plugin/services/services.go` |
| The loader and registries | `src/backend/core/pluginruntime/` |
| The `/api/plugins/loader/*` routes | `src/backend/modules/plugin_runtime/` |
| Frontend loader and mount points | `src/frontend/core/plugins/` |
