# Contracts

A **contract** is an agreement between the host and a plugin: a Go interface, a registry, a
declaration type or a browser-side export that both sides build against. The host calls a plugin
only through these contracts, and a plugin reaches the host only through them. Neither imports
the other's internals. All of them live under `src/backend/core/plugin/` (Go) and
`src/frontend/core/plugins/` (browser).

Every contract is a compiled boundary. An installed plugin's `.so` is linked against these exact
types, so a contract changes by **adding a new interface beside it**, never by editing an
existing one.

## How to use the contracts

### 1. Import one package

```go
import "github.com/therealmcsparrow/mcharbor/core/plugin"
```

`core/plugin` re-exports every type from `contract`, `services`, `types` and the ten adapter
families (`plugin.Plugin`, `plugin.HostContext`, `plugin.NotificationAdapterSpec`,
`plugin.CLICommand`, …). Import a `specs/<name>` package directly only for the contracts it does
not re-export: `specs/abac`, `specs/desiredstate`, `specs/incidentsource`, and `specs/incident`. Never import anything else from the host, and never import another plugin.

### 2. Export a `Plugin`

Implement the seven methods of [`Plugin`](./plugin.md#plugin-required) and export the value as
a package-level variable named `Plugin`. Methods with nothing to register return `nil`.

```go
var Plugin plugin.Plugin = &MyPlugin{manifest: plugin.Manifest{Slug: "comm-example" /* … */}}
```

### 3. Register what you contribute

In the `Register*` methods, hand each registry what the plugin brings: routes, menu entries,
widgets, nodes and permissions. Everything registered in Go is **also declared** in
`plugin.yaml`. The manifest is what the host mounts, gates and audits, and a route that exists
only in Go has no permission check.

### 4. Opt into more by implementing an interface

Tabs, adapters, CLI commands, ABAC resources, desired state and incident sources are
**optional interfaces**. Implement the method and the loader finds it by type assertion. There
is no flag to set and nothing to register:

```go
func (p *MyPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(spec, client)
}
```

### 5. Use the host through `HostContext`

`Init` receives the host's services. Keep them on the plugin struct. Check a field that can be
`nil` before using it, check `Available()` where a handle has it, and reach newer capabilities
by type assertion with a fallback. See [Host services](./host-services.md#three-rules-for-every-handle).

### 6. Mount the interface through the browser contract

The UMD bundle exports `mount(el, ctx)` for its page, plus `widgets[key]`, `tabs[key]` and form
mounts as `{ mount, unmount?, update? }`. It reads host state from `window.McharborHost`. See
[Hooks → Frontend hook points](../hooks/README.md#frontend-hook-points).

### 7. Prove it holds

Run the [contract tests](./tests.md). The plugin tests check your manifest against your Go code:
routes, audit declarations and version agreement.

## Every contract

### The plugin: [plugin.md](./plugin.md)

| Contract | Kind | For |
|---|---|---|
| `Plugin` | interface, required | The value a plugin exports: manifest, five registrations, `Init`, `Shutdown` |
| `Registry` | registry | HTTP routes in the `Public`, `Auth` or `Protected` band |
| `MenuRegistry` / `MenuItem` | registry | Sidebar entries and sections |
| `WidgetRegistry` | registry | Dashboard widgets |
| `NodeRegistry` | registry | Workflow nodes, with an optional server-side `Execute` |
| `AccessRegistry` | registry | Permission keys, ABAC rules, SCIM schemas |
| `TabProvider` / `TabRegistry` / `TabItem` | optional interface | A tab on a host page |
| `AdapterContributor` / `AdapterRegistry` | optional interface | Registering in an adapter family |
| `CLIProvider` / `CLIRegistry` / `CLIRoot` / `CLICommand` / `CLIFlag` / `CLIColumn` | optional interface | A command group for `mcharbor` |
| `SCIMHandler` | optional interface | SCIM operations on a plugin's resources. Declared, not yet dispatched. |
| `ErrScopeViolation`, `ErrPermissionExists`, `ErrSchemaExists` | errors | What the host refuses |

### Adapter families: [adapters.md](./adapters.md#adapter-families)

| Registry | Spec | Client | For |
|---|---|---|---|
| `Storage()` | `StorageAdapterSpec` | `StorageAdapterClient` (+ `RicherStorageTester`, `StorageQuotaReporter`) | Backup destinations |
| `Notification()` | `NotificationAdapterSpec` | `NotificationAdapterClient` | Communication channels |
| `Identity()` | `IdentityAdapterSpec` | `IdentityAdapterClient` | Identity providers |
| `Snapshot()` | `SnapshotAdapterSpec` | `SnapshotAdapterClient` | Snapshot drivers |
| `AI()` | `AIAdapterSpec` | `AIAdapterClient` | AI providers |
| `Logging()` | `LogForwarderAdapterSpec` | `LogForwarderAdapterClient` | Audit log forwarders |
| `Alerting()` | `AlertSourceAdapterSpec` | `AlertSourceAdapterClient` | Alert condition sources |
| `Reporting()` | `ReportProviderSpec` | `ReportProviderClient` | Report providers, consumed through `ReportingHandle` |
| `Scanner()` | `ImageScannerAdapterSpec` | `ImageScannerAdapterClient` (+ `RicherScannerTester`) | Image scanners |
| `Backup()` | `BackupProviderSpec` | `BackupProviderClient` | Backup sources beside container backups |

### Providers found by type assertion: [adapters.md](./adapters.md#providers-found-by-type-assertion)

| Contract | Package | For |
|---|---|---|
| `abac.Provider` | `specs/abac` | Resource types ABAC policies can target |
| `desiredstate.Provider`, `desiredstate.ScopeProvider` | `specs/desiredstate` | Letting the reconciler detect and fix drift |
| `incidentsource.Provider` | `specs/incidentsource` | Raising incidents and supplying evidence |
| `incident.LifecycleClient`, `incident.WebhookParser`, `incident.Poller` | `specs/incident` | Two-way ITSM tickets on a notification client |

### Host services: [host-services.md](./host-services.md)

| Contract | `HostContext` field |
|---|---|
| inline logger | `Logger` |
| `DBHandle`, `Rows`, `KVStore` | `DB` |
| `EncryptionHandle` | `Encryption` |
| `PoolHandle`, `DockerPoolAccessor` | `DockerPool`, `AgentPool` |
| `I18nHandle` | `I18n` |
| `AuditHandle` (+ `AuditActorHandle`) | `Audit` |
| `EnvironmentHandle` | `Environments` |
| `BackupLogHandle` | `BackupLog` |
| `LifecycleHandle` | `Lifecycle` |
| `RBACHandle` | `RBAC` |
| `IdentityProvisioner` | `Identity` |
| `SessionIssuer` | `Sessions` |
| `AIHandle` | `AI` |
| `SnapshotRPCHandle` | `SnapshotRPC` |
| none: a local interface asserted against the `any` field | `AgentRPC` |
| `StorageHandle` (+ `StorageStreamHandle`) | `Storage` |
| `ReportingHandle` | `Reporting` |
| `CommsHandle` | `Comms` |
| `CatalogHandle` | `Catalogs` |
| `WorkflowHandle` | `Workflows` |

### Declarations: [types.md](./types.md)

| Contract | For |
|---|---|
| `Manifest`, `Contributes`, `Capabilities` | `plugin.yaml` |
| `RouteDecl`, `MenuItem`, `ThemeDecl`, `CatalogDecl`, `FormDecl` | Entries under `contributes` |
| `PermissionDecl` | A permission key and the role presets that receive it |
| `ABACRuleDecl` | An ABAC rule |
| `ConfigFieldDecl` | A settings field, and every adapter's `ConfigSchema` |
| `OAuthDefaults` | A zero-config OAuth device flow |
| `SCIMSchema`, `SCIMAttributeDecl` | A SCIM resource type |
| `WidgetDescriptor` | A dashboard widget |
| `NodeDescriptor`, `NodeExecFunc`, `NodeExecInput`, `NodeExecOutput` | A workflow node and its executor |

### Capability declarations: [capabilities.md](./capabilities.md)

| Contract | For |
|---|---|
| `contributes.capabilities` (`runbook-provider`, `backup-row-actions`, `estate-source`, `lifecycle-subject`, `document-export`, `login-endpoint`, `picker-source`, `workflow-node-alias`, `email-channel`, `console-subject`, `agent-rpc`, `reboot-drain`, `search-source`, `search-action`) | Facts a host page needs from whichever plugin provides them, looked up without a slug. Manifest-only, so they never touch the ABI. |

### Agent messages: [agent-rpc](./agent-rpc/README.md)

| Contract | For |
|---|---|
| `agent.rpc` token, `agent-rpc` capability, `HostContext.AgentRPC` | Sending declared messages to a remote agent and observing its unsolicited frames |

### Host reboot: [reboot-drain](./reboot-drain/README.md)

| Contract | For |
|---|---|
| `reboot-drain` capability | Two plugin routes the host calls to list and stop running guests before a host reboot |

### Search palette: [search](./search/README.md)

| Contract | For |
|---|---|
| `search-source`, `search-action` capabilities | Plugin lists the Ctrl+K palette searches, and navigation-only palette actions into the plugin's own pages |

### The browser: [Hooks](../hooks/README.md#frontend-hook-points)

| Contract | For |
|---|---|
| `mount(el, ctx)` / `unmount` / `onNavigate` with `PluginNavContext` | A plugin page under `/plugins/:slug/*` |
| `tabs[key] = { mount, unmount? }` with `PluginTabContext` | A tab on a host page |
| `widgets[key] = { mount, unmount?, update? }` | A dashboard widget |
| Form `mount(el, ctx)` with `value` / `onChange` / `mode` | A `(resource, kind)` slot in a host dialog |
| `window.McharborHost`: `i18n`, `permissions`, `environment`, `toast` | Host state inside the bundle |
| `/api/plugins/loader/*` tables | What the host read from the manifests |

## Changing a contract

- **Never add a method to an existing interface.** Every built `.so` would then fail to load.
  Add a new interface and assert for it, the way `TabProvider`, `CLIProvider`, `AuditActorHandle`,
  `StorageStreamHandle` were added.
- **Any edit under `src/backend/core/plugin/**`, even whitespace, changes build IDs.** Every
  installed plugin must then be rebuilt with `scripts/development/catalog.sh build --all` and
  reinstalled.
- **A new message a plugin needs to send to the agent** is a reviewed addition to the ceiling in
  `core/agent/pluginroutes.go`. The plugin then lists it under `agent-rpc` `send`. No handle or
  spec struct widens. See [Agent RPC](./agent-rpc/README.md#adding-a-message-type).
