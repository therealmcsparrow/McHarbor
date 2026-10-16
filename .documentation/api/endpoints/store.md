# App store, plugins and catalogs

44 endpoints: the Docker application catalog, the plugin store, the plugin runtime's
introspection surface, and the catalog sources feeding both.

| Key | Covers |
|---|---|
| `store_apps.view` / `store_apps.manage` | The Docker app catalog and its installations |
| `settings.view` / `settings.manage` | Catalog sources, and forcing a catalog sync |
| `plugins.view` / `plugins.manage` / `plugins.delete` | The plugin store, and the runtime reload |
| — | The read-only `/api/plugins/loader/*` contribution surface |

## Permissions on the plugin routes

| Gate | Routes |
|---|---|
| `plugins.view` or `plugins.manage` | Reads: installed list, catalog, one entry, its config |
| `plugins.manage` | Install, install stream, update stream, configure, write config, runtime reload |
| `plugins.manage` or `plugins.delete` | Uninstall |

These were authenticated-only until recently: a correctly gated mount existed in the module's
`routes.go` and nothing called it, so the host mounted an ungated variant and any signed-in
account could install, configure or uninstall a plugin. It now delegates to the gated mount,
and the ungated one is gone.

The **read** routes under `/api/plugins/loader/` are the exception and stay
authenticated-only on purpose: the frontend loader fetches the menu, translations, widgets,
themes and bundles for every signed-in user, so gating them would leave a user without
`plugins.view` facing an unrenderable interface rather than a smaller one. The one mutating
route there, `reload`, takes `plugins.manage`.

## Things worth knowing first

**Apps and plugins are different systems that look alike.** An app is a catalog entry that
installs as a Compose stack, under `/api/app-store/`. A plugin extends McHarbor itself with
Go handlers, React bundles, widgets, nodes and adapters, under `/api/appstore/plugins/`. They
have deliberately parallel install-stream shapes so one frontend progress component renders
both.

**Installing takes a URL or a tarball.** Either `url`, or `tarball_b64` with a `checksum`.
The streaming variants emit progress events; the plain ones block for the whole install.

**Updating a plugin is an uninstall followed by an install** at the catalog version, because the
install pipeline refuses an already-installed slug. Configuration survives the cycle, because
the installation row drives enable and disable state, not the config rows.

**Configuration is stored per plugin, with secrets encrypted.** `PUT .../config` replaces the
value set wholesale, and `POST .../configure` takes the same body shape.

**The loader surface is introspection, not control.** `/api/plugins/loader/` reports what the
runtime currently has registered: routes, menu entries, widgets, themes, nodes, translations,
tabs, context menus, topbar items, form extensions, and one endpoint per adapter family:
notification, storage, AI, identity, snapshot. The frontend uses it to render install-aware
pickers. `bundle`, `icon` and `asset` serve files, with slug and filename validated against a
strict pattern so a query string cannot escape the data directory; `asset` currently allows
CSS theme files only.

**`POST /api/plugins/loader/reload` answers 503 when the build does not support reloading.**

**Catalogs are the sources.** A catalog row points at a directory or a URL that publishes
entries. Built-in rows cannot be deleted and accept enable-only updates.
`POST /api/catalogs/{id}/test/stream` probes one and streams a step-by-step report, because a
remote fetch can spend fifteen seconds and the point of the feature is showing which step is
spending it.
## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/app-store` | `store_apps.view` | Returns paginated catalog items with optional category/search filter | `?category=`, `?search=` |
| `GET` | `/api/app-store/categories` | `store_apps.view` | Returns category names with counts | — |
| `POST` | `/api/app-store/install/stream` | `store_apps.manage` | Installs an app from the catalog by creating a Stack, streaming progress via Server-Sent Events. The only install route; the plain `POST /api/app-store/install` no longer exists | — |
| `POST` | `/api/app-store/installations/{id}/uninstall/stream` | `store_apps.manage` | Removes one app installation by tearing down its stack, streaming progress. The only uninstall route; `DELETE /api/app-store/installations/{id}` no longer exists | `{id}` |
| `GET` | `/api/app-store/installed` | `store_apps.view` | Returns all installed apps with stack status | — |
| `POST` | `/api/app-store/sync` | `settings.manage` | Reloads enabled Docker application catalogs | — |
| `GET` | `/api/app-store/sync/status` | `store_apps.view` | Returns the latest sync status | — |
| `GET` | `/api/app-store/{slug}` | `store_apps.view` | Returns a single catalog item by slug | `{slug}` |
| `GET` | `/api/appstore/plugins/catalog` | `plugins.view` or `plugins.manage` | Returns the browsable plugin catalog: every entry in the local catalog directory (mounted at the path configured via MCHARBOR_PLUGIN_CATALOG_DIR) | — |
| `POST` | `/api/appstore/plugins/install/stream` | `plugins.manage` | Installs a plugin from a URL or an uploaded tarball, streaming progress. The only install route; the plain `POST /api/appstore/plugins/install` no longer exists | — |
| `GET` | `/api/appstore/plugins/installed` | `plugins.view` or `plugins.manage` | Lists the installed plugins | — |
| `POST` | `/api/appstore/plugins/uninstall` | `plugins.manage` or `plugins.delete` | Uninstalls a plugin by slug | — |
| `GET` | `/api/appstore/plugins/{slug}` | `plugins.view` or `plugins.manage` | Returns one plugin catalog entry | `{slug}` |
| `GET` | `/api/appstore/plugins/{slug}/config` | `plugins.view` or `plugins.manage` | Returns the merged view of plugin_config rows for the supplied slug | `{slug}` |
| `PUT` | `/api/appstore/plugins/{slug}/config` | `plugins.manage` | Replaces the plugin's config values | `{slug}` |
| `POST` | `/api/appstore/plugins/{slug}/configure` | `plugins.manage` | Writes the plugin's configuration values | `{slug}` |
| `POST` | `/api/appstore/plugins/{slug}/reconcile` | `plugins.manage` | Makes the plugin installed at exactly this catalog version (installing, moving or keeping it), with its role grants intact | `{slug}` |
| `POST` | `/api/appstore/plugins/{slug}/update/stream` | `plugins.manage` | Re-resolves a plugin's catalog entry by slug, checks the new artifact (signature, manifest, compatibility, slug) in staging, and only then replaces the installed plugin. A failed check leaves it untouched; a failed install puts the old version, its enabled flag and role grants back | `{slug}` |
| `GET` | `/api/catalogs` | `settings.view` | Returns every catalog the admin can manage | — |
| `POST` | `/api/catalogs` | `settings.manage` | Adds a new catalog | — |
| `DELETE` | `/api/catalogs/{id}` | `settings.manage` | Removes a catalog | `{id}` |
| `GET` | `/api/catalogs/{id}` | `settings.view` | Returns a single catalog row | `{id}` |
| `PUT` | `/api/catalogs/{id}` | `settings.manage` | Edits the name, URL, or enabled flag of an existing row | `{id}` |
| `POST` | `/api/catalogs/{id}/test/stream` | `settings.manage` | Probes one catalog and streams a step-by-step report over Server-Sent Events | `{id}` |
| `GET` | `/api/plugins/loader/ai-adapters` | — | Returns the runtime-tracked map of registered AI provider adapter specs keyed by ProviderType | — |
| `GET` | `/api/plugins/loader/asset` | — | Serves constrained plugin assets used by host-owned contribution surfaces | `?path=`, `?slug=` |
| `GET` | `/api/plugins/loader/bundle` | — | Serves a plugin's UMD bundle from the data dir | `?slug=` |
| `GET` | `/api/plugins/loader/context-menus` | — | Returns the context-menu entries plugins contribute | — |
| `GET` | `/api/plugins/loader/forms` | — | Returns the per-plugin form-extension list (config_form, setup_guide, selector) keyed by (resource, kind) | — |
| `GET` | `/api/plugins/loader/i18n` | — | I18n | `?lang=` |
| `GET` | `/api/plugins/loader/icon` | — | Serves a plugin's icons/<name> file as image/svg+xml (or image/png when the name ends in .png) | `?name=`, `?slug=`, `?variant=` |
| `GET` | `/api/plugins/loader/identity-adapters` | — | Returns the runtime-tracked map of registered identity provider adapter specs keyed by ProviderType | — |
| `GET` | `/api/plugins/loader/menu` | — | Returns the sidemenu entries plugins contribute | — |
| `GET` | `/api/plugins/loader/nodes` | — | Returns the workflow nodes plugins contribute | — |
| `GET` | `/api/plugins/loader/notification-adapters` | — | Returns the runtime-tracked map of registered notification adapter specs (channelType → spec) | — |
| `POST` | `/api/plugins/loader/reload` | `plugins.manage` | Reloads the plugin runtime. 503 when the build cannot reload | — |
| `GET` | `/api/plugins/loader/routes` | — | Returns the live plugin route table | — |
| `GET` | `/api/plugins/loader/snapshot-adapters` | — | Returns the runtime-tracked map of registered snapshot adapter specs keyed by Driver | — |
| `GET` | `/api/plugins/loader/status` | — | Returns plugin runtime status: what loaded, what failed | — |
| `GET` | `/api/plugins/loader/storage-adapters` | — | Returns the runtime-tracked map of registered storage adapter specs keyed by LocationType | — |
| `GET` | `/api/plugins/loader/tabs` | — | Returns the tabs plugins add to existing pages | — |
| `GET` | `/api/plugins/loader/themes` | — | Returns the themes plugins contribute | — |
| `GET` | `/api/plugins/loader/topbar` | — | Returns the topbar items plugins contribute | — |
| `GET` | `/api/plugins/loader/widgets` | — | Returns the dashboard widgets plugins contribute | — |


## Request bodies

**`POST` `/api/app-store/install/stream`**: `slug`, `name`, `environmentId`?, `ports`?, `volumes`?, `envVars`?, `network`?, `secretFileMode`?

**`POST` `/api/appstore/plugins/install/stream`**: `url`?, `tarball_b64`?, `checksum`?

**`POST` `/api/appstore/plugins/uninstall`**: `slug`

**`PUT` `/api/appstore/plugins/{slug}/config`**: `slug`, `values`

**`POST` `/api/appstore/plugins/{slug}/configure`**: `slug`, `values`

**`POST` `/api/appstore/plugins/{slug}/reconcile`**: `version`, `enabled`?

**`POST` `/api/catalogs`**: `name`, `url`, `kind`

**`PUT` `/api/catalogs/{id}`**: `name`?, `url`?, `enabled`?

## Related

- [Plugins](../../plugins/README.md): the plugin runtime and the shipped plugins
- [Permissions](../conventions/permissions.md): the gap documented above
- [Store screen](../../screens/store/README.md)
