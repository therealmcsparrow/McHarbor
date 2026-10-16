# The Plugin Contract

The interfaces a plugin implements and the registries the host hands it, from
`src/backend/core/plugin/contract/`. A plugin reaches all of them through
`github.com/therealmcsparrow/mcharbor/core/plugin`, which re-exports each type under the same name
(`plugin.Plugin`, `plugin.Registry`, `plugin.CLICommand`, …).

## `Plugin`: required

The one value a plugin exports, as a package-level variable named `Plugin`. The loader looks up
that symbol after `plugin.Open`.

```go
type Plugin interface {
	Manifest() Manifest

	RegisterRoutes(Registry) error
	RegisterMenu(MenuRegistry) error
	RegisterWidgets(WidgetRegistry) error
	RegisterNodes(NodeRegistry) error
	RegisterPermissions(AccessRegistry) error

	Init(ctx context.Context, host HostContext) error
	Shutdown(ctx context.Context) error
}
```

| Method | Called | Returns an error when |
|---|---|---|
| `Manifest()` | first, and whenever the host needs the plugin's identity | n/a |
| `RegisterRoutes` / `RegisterMenu` / `RegisterWidgets` / `RegisterNodes` / `RegisterPermissions` | once each, at load | a registration was refused. The load fails and the error reaches the plugin's status. |
| `Init` | after registration, with the host's services | the plugin cannot run. The load fails as `Init: <err>`, and none of its routes ever answer. |
| `Shutdown` | on host shutdown, and when the plugin is unloaded | cleanup failed. Each plugin gets 30 s. |

The order matters. Routes are collected during `RegisterRoutes` but **published only after
`Init` succeeds**, so a plugin that fails to finish loading is never reachable over HTTP. The
type-asserted providers (`abac`, `incidentsource`, `desiredstate`) are registered after `Init`
too. A reload replaces the whole route set, so a route the new version dropped stops
serving.

A method with nothing to register returns `nil`. The Slack channel
(`src/plugins/Plugins/comm-slack/backend/main.go`) implements five of the seven that way:

```go
type CommSlackPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *CommSlackPlugin) Manifest() plugin.Manifest                     { return p.manifest }
func (p *CommSlackPlugin) RegisterRoutes(_ plugin.Registry) error        { return nil }
func (p *CommSlackPlugin) RegisterMenu(_ plugin.MenuRegistry) error      { return nil }
func (p *CommSlackPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }
func (p *CommSlackPlugin) RegisterNodes(_ plugin.NodeRegistry) error     { return nil }

func (p *CommSlackPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	return nil
}

var Plugin plugin.Plugin = &CommSlackPlugin{manifest: plugin.Manifest{Slug: "comm-slack" /* … */}}
```

## `Registry`: routes

```go
type Registry interface {
	Public(method, path string, h http.HandlerFunc)
	Auth(method, path string, h http.HandlerFunc)
	Protected(method, path string, h http.HandlerFunc)
}
```

| Band | Middleware | Use for |
|---|---|---|
| `Public` | none | Endpoints that authenticate some other way: a one-shot grant, a signed webhook, an IdP callback |
| `Auth` | the authentication band | Sign-in flows |
| `Protected` | session or API key, plus `PluginPermissionMiddleware` | Everything else |

```go
func (p *ReportingPlugin) RegisterRoutes(reg plugin.Registry) error {
	reg.Protected(http.MethodGet, "/api/reporting/reports", p.handleList)
	reg.Protected(http.MethodPost, "/api/reporting/reports", p.handleGenerate)
	return nil
}
```

Every route is also declared in `plugin.yaml` under `contributes.routes` (`RouteDecl`). The
permission the middleware checks is the declared `requires_permission`. A route registered here
but not declared gets an empty key, which the middleware passes through to any signed-in user.
The `TestPluginRouteDeclarationDrift` test fails on exactly that.

## `MenuRegistry`: sidebar

```go
type MenuRegistry interface {
	Add(item MenuItem) error
	Remove(labelKey string) error
}
```

`MenuItem{Parent, LabelNS, LabelKey, Path, Icon, Required, Priority}`. The sidebar is built from
what `RegisterMenu` adds, not from `contributes.menu`. Entries sharing a `Parent` form a section.
A higher `Priority` sorts first.

## `WidgetRegistry` and `NodeRegistry`

```go
type WidgetRegistry interface {
	Add(key string, descriptor types.WidgetDescriptor) error
	Remove(key string) error
}

type NodeRegistry interface {
	Add(key string, descriptor types.NodeDescriptor) error
	Remove(key string) error
}
```

The descriptors are in [Shared types](./types.md#widgetdescriptor). A node descriptor with an
`Execute` function runs server-side. Without one it is a trigger the plugin fires through
`HostContext.Workflows`, or a palette-only node.

```go
func (p *MyPlugin) RegisterNodes(reg plugin.NodeRegistry) error {
	return reg.Add("ping-host", plugin.NodeDescriptor{
		Category:    "network",
		DisplayName: "Ping host",
		Inputs:      []string{"input"},
		Outputs:     []string{"output", "error"},
		Fields:      []map[string]any{{"key": "host", "label": "Host", "type": "text", "required": true}},
		Requires:    "myplugin.use",
		Execute: func(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
			out := maps.Clone(in.Msg)
			out["payload"] = ping(ctx, in.Config["host"])
			return plugin.NodeExecOutput{Port: "output", Msg: out}, nil
		},
	})
}
```

## `AccessRegistry`: permissions, ABAC, SCIM

```go
type AccessRegistry interface {
	AddPermission(p types.PermissionDecl) error
	RemovePermission(key string) error
	InstallABACRule(rule types.ABACRuleDecl) error
	RemoveABACRule(id string) error
	RegisterSCIMSchema(schema types.SCIMSchema) error
	UnregisterSCIMSchema(uri string) error
}
```

The usual pattern registers what the manifest declares, so the two cannot drift
(`addon-reporting/backend/main.go`):

```go
func (p *ReportingPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	for _, decl := range p.manifest.AddsPerms {
		if err := reg.AddPermission(decl); err != nil {
			return fmt.Errorf("reporting: add permission %s: %w", decl.Key, err)
		}
	}
	return nil
}
```

A key may contain no hyphens or colons, and it never shadows a host key (`ErrPermissionExists`).
Writing to `rbac_*` tables directly is refused at install.

## Optional interfaces

The loader type-asserts for each of these after the required methods. A plugin that does not need
one leaves it out entirely.

### `TabProvider`

```go
type TabProvider interface{ RegisterTabs(TabRegistry) error }

type TabRegistry interface {
	Add(item TabItem) error
	Remove(pageID, key string) error
}
```

`TabItem{PageID, Key, LabelNS, LabelKey, Icon, Required, Priority}`. `PageID` is `settings`,
`security`, `environment` or `container`. The bundle exports the tab as `tabs[Key]`. See
[Hooks](../hooks/README.md#frontend-hook-points).

### `AdapterContributor`

```go
type AdapterContributor interface{ RegisterAdapters(AdapterRegistry) error }
```

`AdapterRegistry` hands out one sub-registry per family: `Storage()`, `Notification()`,
`Identity()`, `Snapshot()`, `AI()`, `Logging()`, `Alerting()`, `Reporting()`, `Scanner()`,
`Backup()`. Each family is listed in [Adapters](./adapters.md). A plugin that implements this but
registers nothing gets a warning, not a failure.

### `CLIProvider`

```go
type CLIProvider interface{ RegisterCLI(CLIRegistry) error }

type CLIRegistry interface {
	Root(CLIRoot) error
	Add(CLICommand) error
}
```

`Root` is called once, before any `Add`. A command is a **declaration**, never a handler: it
names a route the plugin registered, and `mcharbor` calls that route like any other client.

| Type | Fields / values |
|---|---|
| `CLIRoot` | `Name`, `Short`, `Aliases`. The host refuses a name that collides with a core command or another plugin's root (`pluginruntime/cli_reserved.go`). |
| `CLICommand` | `Path` (words under the root), `Kind`, `Method`, `Route` (the exact string given to `RegisterRoutes`), `Args` (path placeholders in order), `Flags`, `Body`, `Needs` (must equal the route's declared permission), `Risk`, `Columns`, `GuestKind`, `Short`, `Long`, `Example`, `EnvScoped` (adds `?env=`) |
| `CLIKind` | `list`, `get`, `action`, `text`, `upload`, `download`, `attach` |
| `CLIBody` | `none`, `flags`, `file`, `both` |
| `CLIRisk` | `safe`, `disruptive`, `irreversible`, which map onto the CLI's confirmation rules |
| `CLIFlag` | name, `CLIFlagType` (`string`, `int`, `bool`, `duration`, `stringSlice`, `file`), `CLITarget` (`query`, `body`, `path`) |
| `CLIColumn` | `Header`, `Path` (a dotted lookup into the response), `Format` (for example `bool`, `age`) |
| `CLISpecVersion` | `1`. The contract version the host serves. |

From `addon-compute/backend/cli.go`:

```go
func (p *ComputePlugin) RegisterCLI(reg plugin.CLIRegistry) error {
	if err := reg.Root(plugin.CLIRoot{Name: "compute", Short: "Manage compute hosts, storage pools, guest networks and images"}); err != nil {
		return err
	}
	return reg.Add(plugin.CLICommand{
		Path:    []string{"environment", "list"},
		Kind:    plugin.CLIList,
		Method:  http.MethodGet,
		Route:   "/api/compute/environments",
		Needs:   "compute.view",
		Risk:    plugin.RiskSafe,
		Short:   "List the hosts compute knows about",
		Example: "mcharbor compute environment list",
		Columns: []plugin.CLIColumn{
			{Header: "ENVIRONMENT", Path: "environmentId"},
			{Header: "NAME", Path: "name"},
			{Header: "PROBED", Path: "probedAt", Format: "age"},
		},
	})
}
```

### `SCIMHandler`

```go
type SCIMHandler interface {
	Get(resourceType, id string) (map[string]any, error)
	List(resourceType string, filter string, startIndex, count int) ([]map[string]any, int, error)
	Create(resourceType string, attrs map[string]any) (map[string]any, error)
	Patch(resourceType, id string, patch map[string]any) (map[string]any, error)
	Delete(resourceType, id string) error
}
```

Declared for SCIM operations on a plugin's own resource types. Nothing dispatches it yet. Its
doc comment names `modules/scim/dispatcher.go`, which does not exist.

## Loader errors

| Error | Package | Means |
|---|---|---|
| `ErrScopeViolation` | `contract` | `DB.Exec` / `DB.Query` touched a table outside `plugin_<slug>_` |
| `ErrPermissionExists`, `ErrSchemaExists` | `host`, re-exported by `core/plugin` | An install-time conflict in the `AccessRegistry` |
| `ErrUnknownHostAction`, `ErrPlatformUnsupported` | `contract` | Reserved for future host-side surfaces |
