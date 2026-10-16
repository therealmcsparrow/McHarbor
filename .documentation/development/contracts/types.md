# Shared Types and the Manifest

The declaration types a plugin fills in and hands to the host, from
`src/backend/core/plugin/types/types.go` and `src/backend/core/plugin/contract/manifest.go`. They
are re-exported by `core/plugin` under the same names. The JSON Schema for the manifest is
`src/backend/core/plugin/manifest-v1.schema.json`.

## `Manifest`: `plugin.yaml`

The parsed manifest. The plugin also returns it from `Manifest()`, and the four version copies
(database row, install directory, this file, the `.so` symbol) must agree.

| Field | YAML key | Holds |
|---|---|---|
| `Schema` | `schema` | `mcharbor.plugin/v1` |
| `Slug` | `slug` | Starts with a family prefix: `addon-`, `ai-provider-`, `comm-`, `identity-`, `logging-`, `scanner-`, `snapshot-`, `storage-` |
| `Name`, `Author`, `License`, `Homepage`, `Desc` | `name`, `author`, `license`, `homepage`, `description` | Identity |
| `Version` | `version` | `yyyy.MM.<seq>`, the plugin's own sequence |
| `Mcharbor` | `mcharbor` | The host range, for example `>=2026.09.01` |
| `Go` | `go` | The Go toolchain it was built with |
| `TablePrefix` | `table_prefix` | Only for a renamed plugin whose tables predate its slug |
| `Requires` | `requires` | Other plugin slugs it needs. The installer adds them and removes them with the last dependent. |
| `Perms` | `permissions` | Host capabilities it asks for |
| `AddsPerms` | `adds_permissions` | Its own permission keys ([`PermissionDecl`](#permissiondecl)) |
| `Consumes` | `consumes_permissions` | Host keys it relies on |
| `ABACRules` | `adds_abac_rules` | [`ABACRuleDecl`](#abacruledecl) |
| `Config` | `config_schema` | Plugin settings ([`ConfigFieldDecl`](#configfielddecl)) |
| `Contribs` | `contributes` | [`Contributes`](#contributes) |
| `Caps` | `capabilities` | [`Capabilities`](#capabilities) |
| `Signature`, `Checksum` | `signature`, `checksum` | Filled in by packaging |

### `Contributes`

| Field | YAML key | Element |
|---|---|---|
| `Routes` | `routes` | `RouteDecl{Method, Path, Requires (requires_permission), Public, AuditRequired, AuditAction, AuditEntityType}` |
| `Menu` | `menu` | `MenuItem{Parent, LabelNS, LabelKey, Path, Icon, Required, Priority}`. The sidebar itself is built from `RegisterMenu`. |
| `Widgets`, `Nodes` | `widgets`, `nodes` | Keys |
| `Themes` | `themes` | `ThemeDecl{ID, Name, Description, CSSPath, SwatchHue, DefaultVariant, HasDark, HasLight}` |
| `Catalogs` | `catalogs` | `CatalogDecl{ID, Kind, Name, URL}`. `Kind` must be one the host already knows. |
| `Migrations` | `migrations` | SQL files, run once each and recorded in a ledger |
| `I18nNS` | `i18n_namespaces` | The plugin's namespaces |
| `SCIMSchemas` | `scim_schemas` | [`SCIMSchema`](#scimschema) |
| `Forms` | `forms` | `FormDecl{Resource, Kind, Key}`. `Resource` is `channel`, `email_server`, `identity_provider`, `storage_location` or `ai_provider`. `Kind` is `config_form`, `setup_guide` or `selector`. |

A mutating route declares `audit_action` and `audit_entity_type`, or `audit_required: false`.
`RouteDecl.AuditSeverity` (`audit_severity`) is optional and must be `info`, `success`, `warning` or
`error`; `host.KnownAuditSeverity` is the check, and the audit fallback middleware stores a
successful call at that severity.

### `Capabilities`

| Field | YAML key | Enforcement |
|---|---|---|
| `NetBind` | `net_bind` | Always denied |
| `FSWrite` | `fs_write` | Denied outside the plugin's data directory |
| `OutboundHTTP`, `OutboundTCP`, `Subprocess` | `outbound_http`, `outbound_tcp`, `subprocess` | Informational |

## `PermissionDecl`

```go
type PermissionDecl struct {
	Key         string   // <category>.<action>, lowercase, no hyphens or colons
	Description string
	Scope       string   // "global" | "env" | "stack" (host catalog word for env: "environment")
	Category    string
	Presets     []string // any of "viewer", "editor", "operator"
}
```

`Presets` names the built-in roles that receive the key at install. Every plugin key is stripped
from every role at uninstall. A key with no presets is reachable only by admins until an operator
edits a role.

## `ABACRuleDecl`

```go
type ABACRuleDecl struct {
	ID     string
	When   string
	Then   string // "allow" | "deny"
	Reason string
}
```

Inserted into `rbac_policies` with `origin = <slug>`, so uninstall drops the plugin's rules.

## `ConfigFieldDecl`

One field of a plugin's settings, or of an adapter's `ConfigSchema`, which the host renders in
its own form.

```go
type ConfigFieldDecl struct {
	Key         string
	Type        string // "string" | "int" | "bool" | "secret"
	Label       string
	Default     any
	Secret      bool
	Min, Max    *int
	Required    bool
	Help        string
	Placeholder string
	LabelKey    string // key in the plugin namespace translating Label
	HelpKey     string // key in the plugin namespace translating Help
}
```

`LabelKey` and `HelpKey` travel as `label_key` and `help_key`. The English `Label` and `Help` remain the fallback for the CLI, the API and a missing key.

A `secret` field is stored encrypted. On edit the host sends it back blank, and a blank secret
keeps the stored value.

## `OAuthDefaults`

```go
type OAuthDefaults struct {
	ClientID, DeviceURL, TokenURL, Scopes string
}
```

The non-secret half of an OAuth device flow, shipped so the operator signs in without typing a
client id or endpoints. Values the operator enters by hand win. A client **secret** never goes
here. If a provider needs one, it is a `secret` config field.

## `SCIMSchema`

```go
type SCIMSchema struct {
	URI, Name, Description string
	Attributes             []SCIMAttributeDecl // {Name, Type, Required}
}
```

Added to the SCIM ServiceProviderConfig at install and removed at uninstall.

## `WidgetDescriptor`

```go
type WidgetDescriptor struct {
	Title    string
	Path     string         // "/widgets/<key>/<Widget>.tsx", relative to the plugin root
	Width    int            // grid columns
	Height   int            // grid rows
	Settings map[string]any // description, icon, min_width, min_height
}
```

The bundle exports the widget as `widgets[key] = { mount, unmount?, update? }`.

## `NodeDescriptor`

```go
type NodeDescriptor struct {
	Category, DisplayName, Description, Icon string
	Inputs, Outputs []string
	Config          map[string]any   // legacy: only a "fields" list is honoured
	Fields          []map[string]any // the config schema, in display order
	Requires        string           // permission checked before the node shows in the palette
	Execute         NodeExecFunc     // nil: palette-only or a plugin-fired trigger
}

type NodeExecFunc func(ctx context.Context, in NodeExecInput) (NodeExecOutput, error)

type NodeExecInput struct {
	Key           string         // the node key, so one executor can serve several
	Config        map[string]any // the node's saved configuration
	Msg           map[string]any // the incoming msg: clone it, never mutate it in place
	EnvironmentID string         // the node's environment, or the run's fallback
	UserID        string         // set for a manual run, empty otherwise
	WorkflowID    string
}

type NodeExecOutput struct {
	Port string         // one of Outputs: "output" or "error" by convention
	Msg  map[string]any
}
```

Returning an error fails the node. Returning `Port: "error"` with a payload routes the failure
down the error port instead, which is usually what a node wants.
