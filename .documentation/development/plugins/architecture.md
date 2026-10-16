# Plugin Architecture

How the plugin system is put together on the host side: what a plugin actually is at
runtime, which packages it may import, where its files live, how it is trusted, and which
parts of it the host mounts. [Overview](../../plugins/overview.md) covers the lifecycle from an
operator's point of view; this page is for people changing the host or debugging a plugin
that will not load.

## A plugin is a Go shared object in the host process

Each plugin is compiled with `-buildmode=plugin` into a `.so` that exports one symbol,
`Plugin`. The host calls `plugin.Open` on it and looks that symbol up. Three consequences
follow, and all three surface as support questions sooner or later:

- **There is no sandbox.** A plugin runs in the host process with the host's privileges.
  A panic in a plugin goroutine takes down McHarbor. The manifest's scope tokens are a
  declaration of intent the host checks against a known set. They are not an enforcement
  boundary at the Go level.
- **Linux only.** Go's plugin support does not exist on Windows, so the plugin system is
  live only in the shipped Linux container.
- **The toolchain has to match.** The Go version, the module graph and the compiled ABI
  of every shared package must line up between host and plugin. A mismatch is rejected at
  load with `plugin was built with a different version of package …`.

### Why `core/plugin` and `core/pluginruntime` are separate

`plugin.Open` compares a per-package ABI hash for every package the host and the plugin
share. The plugin linker prunes package symbols it cannot prove reachable from the
plugin's exported entry points, so if the host-side loader lived in `core/plugin`, the
plugin's view of that package would be a strict subset of the host's, the hashes would
differ, and every `.so` would be rejected even when the source is identical.

The split keeps `core/plugin` small, stable, and reachable *as a whole* from a plugin:

| Package | Who imports it | What is in it |
|---|---|---|
| `core/plugin` | Plugins and the host | The shim that re-exports everything below, for compatibility |
| `core/plugin/contract` | Both | `Plugin`, `Manifest`, the registries, `HostContext`, `AdapterContributor` |
| `core/plugin/services` | Both | The host handles: DB, encryption, pools, i18n, audit, RBAC, AI, session issuing, snapshot RPC |
| `core/plugin/types` | Both | `PermissionDecl`, `ConfigFieldDecl`, `WidgetDescriptor`, `NodeDescriptor`, ABAC and SCIM decls |
| `core/plugin/specs/<family>` | Both | One package per adapter family: storage, notification, identity, ai, snapshot, logging, scanner, reporting, alerting, compute |
| `core/plugin/host` | Host only | Manifest schema validation, trust keys, the label registry, adapter sources |
| `core/pluginruntime` | Host only | The loader: reads `appstore_plugins`, opens each `.so`, walks the lifecycle, mounts routes, dispatches adapter calls |

`core/plugin/shim.go` still re-exports every pre-refactor symbol as an alias, so the
shipped plugins compile unchanged. New plugin code should import the specific package;
the shim is scheduled to go.

## On disk

| Path | What it holds |
|---|---|
| `$DATA_DIR/plugins/<slug>@<version>/` | The unpacked plugin: `plugin.yaml`, `backend/<slug>.so`, `frontend/`, `i18n/`, `icons/`, `migrations/` |
| `$DATA_DIR/plugin-catalog/` | The local dev catalog: tarballs under `files/`, and optionally `signing-key.pub` |
| `MCHARBOR_PLUGIN_CATALOG_DIRS` | Comma-separated extra catalog directories the App Store reads |

The version is part of the directory name, so two versions can exist side by side during
an update and the previous one is still on disk if the new one fails to load.

## Trust and signing

The installer checks an ed25519 signature over the tarball before unpacking, and
**records the result rather than enforcing it**. An unsigned plugin (one nobody signed,
or one signed by a key this host does not trust) installs, and says so at every step:
a warning on the install stream, a `plugin.install` audit entry carrying `unsigned: true`,
`signed: false` on the installed row, and an **Unsigned** badge in the App Store. The
install dialog asks the operator to confirm the risk before it starts.

The confirmation is enforced by the server, not only by the dialog. An install, update or
reconcile request for a tarball no trust key verifies is refused unless its body carries
`"allow_unsigned": true` (`err.plugin.unsigned_confirmation_required` on the install routes
and streams), and nothing is extracted. An update checks the signature before it uninstalls
the current version. A dependency pulled in by an install inherits that install's consent,
so a signed plugin never silently brings an unsigned one with it. The CLI passes the field
with `--allow-unsigned`. Recommended settings for an internet-facing host are in
[Security posture](../../getting-started/deployment/security-posture/README.md#plugin-signing).

That is deliberate. A self-hosted host that can only run what its vendor signed is not
self-hosted, and third-party plugins are the point of the plugin system. Recording the trust
decision keeps what refusing bought: knowing what is running.

- `DefaultPinnedKeys` is **empty by design**. Nothing is baked into the binary at build
  time, so a stock McHarbor has no pinned publisher.
- With no trust keys configured, the installer **logs a warning and proceeds**. Every
  install on such a host is recorded as unsigned. If you care about plugin provenance,
  configure keys before installing anything you did not build.
- `MCHARBOR_PLUGIN_REQUIRE_SIGNATURE=true` restores the strict policy: an unsigned or
  untrusted tarball is refused with `plugin signature invalid`. Set it on a host that
  should only ever run tarballs signed by a pinned key. With it set and **no** trust keys
  configured, every install fails. The combination is a misconfiguration, and the host
  logs it as one rather than quietly letting everything through.
- Operators add keys with `MCHARBOR_PLUGIN_TRUST_KEYS=/path/to/keys.pem`, or at build
  time through `WithExtraKeys()`. The file may hold several PEM blocks; both `PUBLIC KEY`
  (SubjectPublicKeyInfo) and `ED25519 PUBLIC KEY` are accepted.
- With neither configured, the loader falls back to
  `$DATA_DIR/plugin-catalog/signing-key.pub`, so a fresh clone with a mounted dev catalog
  verifies its own tarballs. Omit that file and the fallback is inert.
- Keys are read once at construction. Rotation means a restart.
- `MCHARBOR_PLUGIN_RELAXED_SCHEMA` loosens manifest schema validation. It exists for
  plugin development; leave it unset in production.

## What the host mounts

The manifest is the source of truth, not the code:

- **Routes.** Each entry in `contributes.routes` is wrapped in
  `router.PluginPermissionMiddleware` with that entry's `requires_permission` and mounted
  under the plugin router. A handler the plugin registers but the manifest does not
  declare is unreachable. An entry with no `requires_permission` is session-protected
  only. Docker Swarm's `capability` probe is the one shipped example, and it returns a
  boolean and a node role.
- **Menu, widgets, nodes, forms, tabs, context menus, topbar entries.** Declared in the
  manifest, registered in the corresponding `Register*` call, and served to the frontend
  from the loader endpoints.
- **Permissions.** Keys in `adds_permissions` are inserted into `plugin_permissions` with
  `origin = <slug>` and merged into the catalog the role editor reads. A key a plugin
  never declares is a key no role can be granted.
- **Migrations.** Applied once per install, in order, in a transaction, and confined to
  the plugin's own table prefix (`plugin_<slug>_*`, or the manifest's `table_prefix`). A
  migration that tries to touch a host table is refused.

The plugin router is mounted **last**, after every host route, so a plugin cannot shadow
a core endpoint by declaring the same path.

### Dependencies

A manifest may name other plugins in `requires`. The installer pulls each missing one in
before the plugin that asked for it, and uninstall removes a dependency once nothing
requires it any more, but only one the host installed itself, which is recorded per
install. Installing **Virtual Machines** or **System Containers** brings **Compute
Foundation** with it.

Slugs only: there is no version resolver, and a dependency that fails to install is a
warning rather than a failed install, because the plugin is already on disk with its
migrations run and reports the gap itself.

## Boot order

For each plugin, every boot and after an install or reload:

1. `Manifest()`
2. `RegisterRoutes`, `RegisterMenu`, `RegisterWidgets`, `RegisterNodes`,
   `RegisterPermissions`
3. `RegisterAdapters`, when the plugin implements `AdapterContributor`
4. `Init(ctx, host)` with the host handles

`Shutdown(ctx)` runs on host shutdown. It is best effort, so do not put anything in it that
data integrity depends on.

## The loader endpoints

The frontend reads what plugins contribute from `/api/plugins/loader/*`:

| Endpoint | Returns |
|---|---|
| `GET /plugins/loader/status` | Loader state |
| `GET /plugins/loader/routes` | Mounted plugin routes |
| `GET /plugins/loader/menu` | Sidebar entries |
| `GET /plugins/loader/widgets` | Dashboard widgets |
| `GET /plugins/loader/nodes` | Workflow nodes |
| `GET /plugins/loader/themes` | Theme contributions |
| `GET /plugins/loader/i18n` | Merged translation bundles |
| `GET /plugins/loader/tabs` | Tabs injected into host pages |
| `GET /plugins/loader/context-menus` | Context-menu entries |
| `GET /plugins/loader/topbar` | Top-bar entries |
| `GET /plugins/loader/forms` | Declared config forms and setup guides |
| `GET /plugins/loader/notification-adapters` | Registered channel types |
| `GET /plugins/loader/storage-adapters` | Registered destination types |
| `GET /plugins/loader/ai-adapters` | Registered AI provider types |
| `GET /plugins/loader/identity-adapters` | Registered identity provider types |
| `GET /plugins/loader/snapshot-adapters` | Registered snapshot drivers |
| `GET /plugins/loader/bundle` | A plugin's UMD frontend bundle |
| `GET /plugins/loader/icon`, `/asset` | Plugin icons and assets |
| `POST /plugins/loader/reload` | Rescan contributions without restarting |

Install and lifecycle live under the App Store instead:

| Route | Permission |
|---|---|
| `GET /appstore/plugins/installed`, `/catalog`, `/{slug}`, `/{slug}/config` | `plugins.view` or `plugins.manage` |
| `POST /appstore/plugins/install/stream`, `/{slug}/update/stream`, `/{slug}/reconcile`, `/{slug}/configure`, `PUT /{slug}/config` | `plugins.manage` |
| `POST /appstore/plugins/uninstall` | `plugins.manage` or `plugins.delete` |

### Reload carries no permission key

`POST /api/plugins/loader/reload` is session-protected and nothing more. The comment
above the module's `Mount` claims reload requires `plugins.manage` at the handler level,
but the handler does not check it and the route is registered on the protected group with
no middleware, so any authenticated session can trigger a reload. The install and
uninstall routes above *are* gated; reload is the gap.

## Clustering

Adapters that push data outward (the two [audit log forwarders](../../plugins/logging/README.md))
declare `RequiresLeaderElection`. In a clustered install only the elected leader runs
them, so one audit event is forwarded once rather than once per node. An adapter that
does outbound work on a timer and does not declare it will duplicate.

## When a plugin does not load

| Symptom in the log | Cause | Fix |
|---|---|---|
| `plugin built with a different Go toolchain: … was built with goX, host runs goY` | The `.so` predates a Go bump; the loader refuses it before `plugin.Open` so the host keeps running | Rebuild the plugin and the catalog, then **Update** it from the Plugin Store |
| `plugin was built with a different version of package …` | Host and plugin compiled against different module contexts or Go versions | Rebuild the plugin against the host's Go toolchain and module graph |
| `plugin already loaded` | Two directories for the same slug | Only one `<slug>@<version>` may be active; remove the stale one |
| `symbol Plugin not found` | The `.so` does not export `var Plugin plugin.Plugin` | Check the exported variable name and its type |
| Manifest rejected at install | Slug does not match the directory, version is a downgrade, `mcharbor` range does not match, or an unknown scope token | Fix the manifest; the error names the field |
| Installs, but nothing appears in the UI | The contribution is registered in code but missing from `contributes` | The manifest is the source of truth; add the entry |
| Routes 404 after install | The frontend has cached contribution tables | Reload the page, or call the loader reload endpoint |

## Related

- [Overview](../../plugins/overview.md): the operator-facing lifecycle and security model
- [Developing a plugin](./developing.md): the SDK, the manifest reference, the developer loop
- [Catalog](../../plugins/catalog.md): every shipped plugin
