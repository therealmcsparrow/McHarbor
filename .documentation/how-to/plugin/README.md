# Build a plugin

A plugin is a signed, installable unit that extends McHarbor with a Go backend, a React
bundle, dashboard widgets, workflow nodes, database tables, translations, and hooks into the
host's own interface. It is the only supported way to add a feature without forking.

This page is the end-to-end path. The four pages beside it are the reference for each layer:

| Page | Covers |
|---|---|
| [Manifest](./manifest.md) | Every field in `plugin.yaml`, and what the installer does with it |
| [Backend](./backend.md) | The Go side: the `Plugin` symbol, the registries, the host handles |
| [Frontend](./frontend.md) | The UMD bundle, and the four places it can mount into the host UI |
| [Packaging](./packaging.md) | Build, sign, publish, install, and the loop you work in |
| [ABI and libc](./abi-and-libc.md) | Why a plugin only loads into the host build it was compiled against, and what keeps that true |
| [Isolation and families](./isolation-and-families.md) | Why plugins share no code, the family bookkeeping in the duplication check, and what a fix to one family member owes its siblings |

## The one rule

**A plugin owns its whole feature.** The route, the handler, the service, the SQL, the page,
the query hook, the strings, the icon, the permission key: all of it lives under the
plugin's folder.

The host never grows a route, a page, a component, an i18n key or a table for a
plugin-owned feature. If you are adding `src/backend/modules/<feature>/` or
`src/frontend/modules/<feature>/` for something a plugin should own, stop and move it.

## Where plugins live

```
src/plugins/Plugins/<slug>/
```

Scaffold one with `mcharbor plugin dev new <slug>`. It writes a minimal working plugin (a
no-op adapter, one config field, lifecycle audit events) from a template embedded in the CLI
binary, with the slug already rewritten everywhere the loader compares it.

Do not start a plugin by copying a sibling. A sibling carries its vendor quirks, its bugs and
whatever pattern was current when it was written. Read one as a worked example by all means;
scaffold the new one from the command. A plugin never imports another plugin or a shared
plugin SDK; see [Isolation and families](./isolation-and-families.md).

## Anatomy

```
src/plugins/Plugins/<slug>/
├── plugin.yaml                 required — the manifest, and the installer's source of truth
├── README.md                   required — what it does, how to build it
├── LICENSE
├── go.mod                      required when backend/ exists; replace-directive to src/backend
├── backend/
│   ├── main.go                 exports `var Plugin plugin.Plugin`
│   ├── handler.go              thin handlers
│   └── service.go              the logic and the SQL
├── frontend/
│   ├── src/index.tsx           UMD entry — exports mount / unmount / widgets
│   ├── src/<Page>.tsx          the page
│   ├── src/hooks/useX.ts       TanStack Query hooks
│   └── vite-plugin.config.ts
├── i18n/<lang>/<namespace>.json
├── widgets/<key>/<Widget>.tsx
├── nodes/<key>/index.ts
├── icons/<name>.svg
└── migrations/NNNN_<name>.sql
```

The directory name **must** equal `slug` in the manifest. The installer refuses anything that
disagrees.

## The path, start to finish

### 1. Decide what the plugin is

Three shapes, and they want different amounts of the layout above:

| Shape | Ships | Example |
|---|---|---|
| **Adapter** | A Go backend registering one adapter, a config schema, no page | `storage-sftp`, `comm-slack`, `ai-provider-openai` |
| **Feature** | Backend, page, tables, menu entry, often widgets and nodes | `addon-reporting`, `addon-kubernetes` |
| **Contribution-only** | Widgets or nodes and their strings, no backend | rare, but valid |

An adapter plugin is much less work, because the host already owns the surface it plugs
into: the destination picker, the channel dialog, the provider list. If what you are adding
fits an existing family, fill that slot rather than building a page.

### 2. Write the manifest

`plugin.yaml` declares everything: the slug, the version, the host version range, the scope
tokens the plugin needs, the permission keys it adds, the routes, the menu entries, the
widgets, the nodes, the migrations, the i18n namespaces.

The installer validates it before anything is loaded, and the loader refuses to mount a route
that is not declared. See [Manifest](./manifest.md).

### 3. Write the backend

```go
package main

import "github.com/therealmcsparrow/mcharbor/core/plugin"

type MyPlugin struct{ /* state */ }

var Plugin plugin.Plugin = &MyPlugin{}
```

The host opens the compiled `.so`, looks up the `Plugin` symbol, calls `Manifest()`, then the
`Register*` methods in order, then `Init(ctx, host)`. Handlers reach the database, the audit
log, encryption and the Docker pool through the `host` handle stored during `Init`.

See [Backend](./backend.md).

### 4. Write the frontend

The bundle is **UMD**, not ESM, so the host can mount it lazily after install. It exposes a
global carrying `mount` and `unmount`, plus a `widgets` map when the plugin ships tiles.
`react`, `react-dom`, `react-i18next` and `i18next` are externals; the host provides them
once.

The host auto-creates the SPA route for the plugin's menu path. The page does not register
itself in the host router.

See [Frontend](./frontend.md).

### 5. Declare the permissions

A plugin declares its own keys in `adds_permissions`; it does not touch the host catalog. The
host inserts them with the plugin's slug as their origin, merges them into the catalog the
role editor reads, and wraps every declared route in the matching middleware, so declaring
correctly *is* enforcing.

Name presets in the declaration, or the key is reachable only by an administrator until
someone edits a role by hand.

See [Contract permissions](../permissions/README.md).

### 6. Add the migrations

Numbered SQL files, applied in order, each in a transaction. Tables and columns must be
prefixed so uninstall can drop them in one pass: `plugin_<slug>_`, or whatever
`table_prefix` in the manifest declares when the prefix differs from the slug.

A plugin can add tables, indices and triggers. It cannot drop host tables.

### 7. Add the strings

`i18n/<lang>/<namespace>.json`, with the namespace listed in `contributes.i18n_namespaces`.
English is the source of truth; every locale the host ships must have a file, even if it is
still English. The host merges the bundles at install time and the frontend reads them with
`useTranslation('<namespace>')`.

### 8. Build, sign, install

```bash
cd src/plugins/Plugins/<slug>
make build        # backend .so + frontend UMD + tarball
make sign         # ed25519 signature
make verify
docker compose build && docker compose up -d
```

Then install from **App Store → Plugins**. See [Packaging](./packaging.md) for the catalog
and the developer loop.

## What the host does on install

1. Downloads the tarball and its signature.
2. Verifies the ed25519 signature.
3. Checks the manifest against the host version's compatibility range.
4. Reserves the declared permission keys and adds them to the named role presets.
5. Runs the migrations, each in a transaction.
6. Opens the Go plugin and calls `RegisterRoutes`, `RegisterMenu`, `RegisterWidgets`,
   `RegisterNodes`, `RegisterPermissions` in order.
7. Calls `Init` and emits an audit event.
8. The frontend loader fetches the contribution tables from `/api/plugins/loader/*` and the
   bridges wire them into the sidemenu, topbar, context menus, tabs, widget registry and node
   registry.

## What the host does on uninstall

A seven-step gate chain, in order: `Shutdown`; unregister SCIM schemas; remove ABAC rules by
origin; drop the routes from the router and the frontend route table; remove the permissions
(a **hard failure** if a role still references one); drop the plugin's tables; delete the files
and clear the contribution cache.

The failure on step five is deliberate. Silently stripping a key from live roles would change
what people can do without telling anyone.

## Security expectations

A plugin runs in the host process. These are not suggestions.

- **Never open your own database connection.** Use the host handle. The host owns the file
  and the transaction discipline.
- **Never log a secret.** `secret` config fields are encrypted at rest and redacted in
  responses; keep them out of logs, audit payloads and JSX.
- **Never return raw Go error text.** Log it, return a code.
- **Bound every external call** with a timeout: Docker SDK, HTTP, database.
- **Do not roll your own permission check** in place of the declared one. The host wires the
  middleware from the manifest; a handler that also decides for itself will drift.
- **Prefer reading through the host's own API to reading its tables.** The reporting add-on
  does this deliberately: it calls McHarbor over loopback forwarding the requesting user's
  session, so a report can never contain data that user could not already see. Reading the
  database directly would bypass roles, ABAC rules and environment scoping. See
  [Build a reporting plugin](../reporting/README.md).

## When it breaks

| Symptom | First thing to check |
|---|---|
| `manifest slug does not match directory` | The folder name equals `slug` |
| `plugin.Open: different version of package` | Plugin and host must compile the shared packages in one module graph. Rebuild the host image |
| The handler is never called | The route is missing from `contributes.routes`; the loader skips undeclared routes |
| 401 or 403 for a signed-in user | `requires_permission` names a key no role grants. Add it to a preset, or grant it |
| The page renders blank | `index.tsx` must export `mount` and `unmount` **by name**, and the UMD global must match the expected name |
| Cookies are not sent | Call the API through the shared client, not through raw `fetch` |
| A widget never appears | The key in `contributes.widgets` must match both the folder and the exported `widgets` map key |

## Related

- [Plugins](../../plugins/README.md): the runtime, and every shipped plugin
- [Developing a plugin](../../development/plugins/developing.md): the same ground from the runtime's side
- [Plugin architecture](../../development/plugins/architecture.md): what the host does with the result
- [Isolation and families](./isolation-and-families.md): no shared plugin code, and the duplication check that watches the copies
- [App store and plugin endpoints](../../api/endpoints/store.md): including the permission gap on those routes
