# The plugin manifest

`plugin.yaml` is the installer's source of truth, not the Go code or the folder contents.
The loader refuses to mount a route that is not declared here, and refuses to
register a contribution that has no matching source file. Both directions are checked.

## Skeleton

```yaml
schema: "mcharbor.plugin/v1"

slug:     "my-plugin"
name:     "My Plugin"
version:  "2026.09.01"
author:   "You <you@example.com>"
license:  "Apache-2.0"
homepage: "https://example.com/my-plugin"
description: >-
  One or two sentences. This is the catalog card copy.

mcharbor: ">=2026.09.01"
go:       ">=1.25"
```

| Field | Rules |
|---|---|
| `schema` | The manifest schema version. Currently `mcharbor.plugin/v1` |
| `slug` | `^[a-z][a-z0-9-]{1,40}$`, and must equal the directory name. `host` is reserved (it is the workflow trigger's fire source for the host's own events) and is refused at install, at load and by `mcharbor plugin dev validate` |
| `version` | `yyyy.MM.<sequential>` for bundled plugins, which share the host's version; any `N.N.N` for a third-party plugin. Leading zeros are permitted, so this is not strict semver. The host refuses a downgrade |
| `mcharbor` | The host version range this plugin supports. Checked at install |
| `go` | The Go toolchain range. A mismatch is rejected at load, not at build |
| `table_prefix` | Optional. The prefix uninstall drops tables by, when it is not `plugin_<slug>_`. A plugin that was renamed keeps its old prefix and declares it here |
| `requires` | Optional. Slugs of plugins this one cannot work without; see [Dependencies](#dependencies) |

## Dependencies

```yaml
requires:
  - addon-compute
```

A plugin that builds on another declares it here. At install the host pulls in every named
plugin that is missing, then installs the one that asked for it; at uninstall it removes a
dependency again once the last plugin requiring it is gone. Installing **Virtual Machines**
or **System Containers** therefore brings **Compute Foundation** with it, and removing both
takes it away again.

A dependency the operator installed deliberately is never removed. Each install records
whether it was pulled in as a dependency, and only those are cleaned up.

Two things this is deliberately not:

- **Not a version resolver.** Slugs only: no ranges, no constraints. A plugin family that
  ships together moves together.
- **Not a hard gate.** A dependency that fails to install is a warning, not a failed
  install: the plugin that asked for it is already on disk with its migrations run, and it
  reports the missing dependency itself. Chains are bounded, so a manifest that names
  itself, or a pair that name each other, stops rather than installing forever.

## Scope tokens

```yaml
permissions:
  - menu.sidebar.add
  - topbar.item.add
  - page.contextmenu.add
  - page.tab.add
  - dashboard.widget.add
  - workflow.node.add
  - i18n.namespace.add:my
  - agent.rpc
  - rbac.permission.register
  - db.migrations.run
  - audit.emit
  - alerting.adapter.register
  - notification.adapter.register
  - encryption.handle.read
  - docker.socket.read
```

These say what the plugin needs in order to register itself. Each is checked against an
allow-list at install time and an unrecognised token fails the install. Some take an argument
after a colon: `i18n.namespace.add:my` claims one namespace. `agent.rpc` (formerly `compute.rpc`) is what lets a
plugin send messages to an agent. It works together with an `agent-rpc` capability that names
the messages; with only one of the two the plugin is handed nothing and has to cope. See
[Agent RPC](../../development/contracts/agent-rpc/README.md).

Ask for what you use. A plugin holding `docker.socket.read` it never exercises is a plugin an
operator has to think about.

## Permissions the plugin adds

```yaml
adds_permissions:
  - key:         "my_plugin.read"
    description: "View data in My Plugin"
    scope:       "env"
    category:    "Plugins"
    presets:     [viewer, editor, operator]
  - key:         "my_plugin.manage"
    description: "Create and edit in My Plugin"
    scope:       "env"
    category:    "Plugins"
    presets:     [operator]
```

| Field | Meaning |
|---|---|
| `key` | The permission key. Must be unique across the host and every other plugin |
| `description` | What the role editor shows |
| `scope` | `global` (default), `env` (per environment; shown as the host's `environment` scope) or `stack`. `environment`, `owner` and `account` are host-only and refused |
| `category` | The group the role editor files it under |
| `presets` | Which built-in role presets receive the key at install: `viewer`, `editor`, `operator`. Admin holds a wildcard and is never listed |

**The key grammar is narrow, and it is enforced at install.** A key must match:

```
^[a-z][a-z0-9_.]{0,62}[a-z0-9]$
```

Lowercase letters, digits, underscore and dot. **No hyphens, and no colons**, even though a
scope token in `permissions:` may carry a `:argument` suffix, and even though plugin slugs
routinely contain hyphens. A slug of `my-plugin` therefore takes keys like `my_plugin.read`,
not `my-plugin.read`.

Getting this wrong fails the install outright, after download and signature verification, with
`adds_permissions[0].key: invalid permission key "…"`. All eleven communication plugins
shipped with a `channels:use-comm-<name>` key and none of them could be installed until the
keys were renamed.

**Omitting `presets` has a consequence.** The host adds the key to the named presets at
install and strips every plugin key from every role at uninstall. A key in no preset is
reachable only by an administrator until an operator edits a role by hand, which is
sometimes right for a destructive capability and usually wrong for a read.

Keys are origin-tagged with the plugin slug. Two plugins cannot share a key, and a plugin
cannot shadow a host key; the host skips it.

```yaml
consumes_permissions:
  - rbac.environment.read
  - audit.read
```

Host keys the plugin relies on. They must already exist, or the install fails.

## ABAC rules

```yaml
adds_abac_rules:
  - id:   "my-plugin.deny-on-prod"
    when: "request.perm == 'my_plugin.manage' && request.env.labels['env'] == 'prod'"
    then: "deny"
```

Origin-tagged like permissions, and removed on uninstall.

## Configuration schema

```yaml
config_schema:
  - key:      "endpoint_url"
    type:     "string"
    required: false
    default:  "https://example.com"
  - key:      "api_token"
    type:     "secret"
    required: true
```

The host renders this as the plugin's settings form and stores the values. A `secret` field
is encrypted at rest and redacted in API responses, which is why a plugin's own edit dialog
reads secrets back through a dedicated endpoint rather than from the ordinary read.

## Contributions

### Routes

```yaml
contributes:
  routes:
    - method: GET
      path:   "/api/my-plugin/items"
      requires_permission: "my_plugin.read"
    - method: POST
      path:   "/api/my-plugin/items"
      requires_permission: "my_plugin.manage"
      audit_required:    true
      audit_action:      "plugin.my-plugin.item.created"
      audit_entity_type: "item"
```

| Field | Meaning |
|---|---|
| `method`, `path` | The route. The path must start `/api/<slug>/` |
| `requires_permission` | The key the host wraps the route in, before the handler is reached |
| `public` | Mounts the route without authentication. Use only for something externally authenticated |
| `audit_required` | The host records an audit entry for this route |
| `audit_action` | The action string in the entry. Convention is `plugin.<slug>.<entity>.<verb>` |
| `audit_entity_type` | The entity type in the entry |
| `audit_severity` | The severity a successful call's entry is stored at: `info`, `success`, `warning` or `error`. Omitted, the host infers it from the action. A failed call is stored at `warning` (or `error` when declared). Any other value refuses the manifest at install and load, and `mcharbor plugin dev validate` reports it. A route that returns a secret in the clear declares `warning` |

Declaring a route here is what mounts it. A Go handler with no manifest entry is dead code;
a manifest entry with no handler fails the install.

### Menu entries

```yaml
  menu:
    - parent:              "reporting"
      label_ns:            "my-plugin"
      label_key:           "menu.root"
      path:                "/my-plugin"
      icon:                "IconChartBar"
      required_permission: "my_plugin.read"
```

| Field | Meaning |
|---|---|
| `parent` | A sidebar section id. Entries sharing one render as their own named section rather than as loose items under **Plugins**. Empty means top-level under Plugins |
| `label_ns` + `label_key` | Resolved at render time, so the plugin keeps ownership of its own label |
| `path` | The SPA route the host auto-creates for the plugin page |
| `icon` | **A Tabler component name**, such as `IconChartBar`, not an SVG filename. The host looks the string up in a registry of imported components and falls back to a puzzle-piece icon for anything it does not recognise |
| `required_permission` | Hides the link entirely when the user lacks it. Navigation is the one place hiding is right: a dead-end page is not informative |

Gate the entry on the permission the page's *first* action needs, not on the weakest one it
uses. An entry gated on `read` that opens a page where every button needs `generate` walks the
user to a wall.

### Topbar, context menus, tabs

```yaml
  topbar:
    - slot:       "left"        # "left" | "right"
      label_ns:   "my-plugin"
      label_key:  "topbar.open"
      icon:       "IconChartBar"
      visibility: "permission:my_plugin.read"   # or "always"

  context_menus:
    - page:                "container-detail"
      label_ns:            "my-plugin"
      label_key:           "context.runAction"
      requires_permission: "my_plugin.manage"
      when:                "container.state == 'running'"

  tabs:
    - page:                "container-detail"
      key:                 "my-plugin-panel"
      label_ns:            "my-plugin"
      label_key:           "tabs.panel"
      requires_permission: "my_plugin.read"
```

- `slot` picks which header mount point the button lands in: left, next to the environment
  selector, or right, before the global search.
- `page` must be a host-defined page id. An unknown id fails the install.
- `when` is an optional short JavaScript boolean evaluated against the page's context object,
  in a sandbox. Keep it to one condition.
- A tab `key` must be unique per plugin-and-page pair. The host composes the full id.

See [Frontend](./frontend.md) for what the plugin mounts into each of these.

### Widgets, nodes, migrations, namespaces

```yaml
  widgets:
    - "my-counter"          # must match widgets/<key>/ and the exported widgets map
  nodes:
    - "my-action"           # must match nodes/<key>/index.ts
  migrations:
    - "migrations/0001_init.sql"
  i18n_namespaces:
    - "my-plugin"           # matches i18n/<lang>/<namespace>.json
```

## Validation the installer applies

In roughly this order:

1. The schema version is one the host knows.
2. The slug matches the pattern **and** the directory name.
3. The version is semver, and newer than any installed version.
4. The host version satisfies `mcharbor`; the toolchain satisfies `go`.
5. Every scope token is on the allow-list.
6. Every added permission key is well-formed and not a duplicate of a host key, of another
   plugin's key, or of another key in the same manifest.
7. Every consumed permission already exists.
8. Every declared page id for a tab or context menu is known.
9. Every declared widget, node and migration has a matching file.
10. Every declared i18n namespace has bundles.

A manifest error names the exact field (`adds_permissions[2].key`) rather than failing
vaguely. Read the field path; it is almost always literally the problem.

## Related

- [Build a plugin](./README.md): the end-to-end path
- [Backend](./backend.md): what the declared routes connect to
- [Frontend](./frontend.md): what mounts into the declared UI hooks
- [Contract permissions](../permissions/README.md): the permission model in full
