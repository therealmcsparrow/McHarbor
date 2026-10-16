# Plugins: Overview

What a plugin is, what has to be inside one, and the two shapes they come in: feature
plugins that own a capability end to end, and adapter plugins that supply one
implementation of a family the host already administers. The manifest is the source of
truth for all of it.

## What a plugin is

A plugin is one directory, packaged as a `tar.gz` (signed where a trust key exists, and
installable unsigned with an explicit risk confirmation) that the McHarbor host loads
into its own process at boot.

| Part | Required | What it is |
|---|---|---|
| `plugin.yaml` | Yes | The manifest: identity, compatibility, the capabilities it asks for, the permission keys it adds, and everything it contributes |
| `backend/<slug>.so` | Yes | A Go plugin compiled with `-buildmode=plugin` that exports one symbol, `Plugin` |
| `frontend/<slug>.umd.cjs` | No | A React bundle in UMD format for pages, forms, widgets, nodes and tabs |
| `i18n/<lang>/<ns>.json` | When the UI has strings | Translation bundles the host merges at boot |
| `migrations/NNNN_*.sql` | No | SQL applied once per install, in order, inside a transaction |
| `widgets/`, `nodes/`, `icons/` | No | Dashboard widgets, workflow nodes and SVG icons |

The manifest is the source of truth. The host mounts only routes declared in it,
registers only the widgets and nodes it lists, and refuses a manifest asking for
capabilities outside the known set.

## Two shapes

**Feature plugins** own a capability end to end: their own API routes, their own pages
under `/plugins/<slug>`, their own tables, and often widgets and workflow nodes. The six
shipped ones each have a section of their own, and [Add-ons](./addons.md) is what they have in common as plugins.

**Adapter plugins** implement one of the host's pluggable families. The host owns the
administration screen, the database rows and the scheduling; the plugin supplies only the
code that talks to the external system. An adapter plugin usually has no routes and no
page of its own.

| Family | Registry | What the host asks it to do | Where it appears |
|---|---|---|---|
| [Storage](./storage/README.md) | `Storage()` | Test, upload, download, delete, list, and optionally OAuth consent | Settings → Integrations → Storage; backup destinations |
| [Communication](./communication/README.md) | `Notification()` | Test, and send a title, message, level and attachments | Settings → Integrations → Communications |
| [Identity](./identity/README.md) | `Identity()` | Begin a login, handle the callback, fetch groups, serve SP metadata | Security → Identity Providers; the login page |
| [Logging](./logging/README.md) | `Logging()` | Forward pending audit events, report delivery history | Settings → Integrations → Logging |
| [Scanner](./scanners/README.md) | `Scanner()` | Scan an image reference and return findings | Security → Scanners |
| [Snapshot](./snapshots/README.md) | `Snapshot()` | Probe capability, create and remove filesystem snapshots | Backup consistency mode on an environment |
| [AI](./ai-providers/README.md) | `AI()` | Test, list models, complete a chat request | Settings → Integrations → AI |
| Alerting | `Alerting()` | Declare conditions and evaluate them on a schedule | Settings → Monitoring → Alerts |
| Reporting | `Reporting()` | Declare report types and build report sections | The Reporting plugin's report studio |

Catalog naming follows the family: `storage-*`, `comm-*`, `identity-*`, `logging-*`,
`scanner-*`, `snapshot-*`, `ai-provider-*`, and `addon-*` for feature plugins.

One package can register in several families. Every compute add-on registers as both a
reporting and an alerting contributor alongside its own routes.

## The convention that explains the rest

**The host owns the admin surface; the plugin owns the work.**

Destinations, channels, providers, scanners and forwarders are all created, listed,
tested and deleted through host routes (`/api/storage-locations`,
`/api/communication-channels`, `/api/identity-providers`, `/api/scans`,
`/api/audit/logging`), even though every adapter behind them is a plugin. Putting that
CRUD in each plugin would give an operator a differently shaped form for the same job,
once per plugin.

That is why most adapter plugins declare no routes at all, and why every family page
answers "where it appears" by naming a host screen.

## Lifecycle

### Install

1. An operator picks a plugin in **App Store → Plugins**, or posts a tarball or URL to
   the install endpoint.
2. The host checks the ed25519 signature against its trust keys. An unsigned plugin is
   allowed: the operator confirms the risk in the install dialog, the request carries
   `allow_unsigned: true` (refused with `err.plugin.unsigned_confirmation_required`
   without it), and the install is badged and audited as unsigned. This does not apply when
   `MCHARBOR_PLUGIN_REQUIRE_SIGNATURE` is set.
3. The manifest is validated: the slug matches the directory, the version is not a
   downgrade, the `mcharbor` range matches the host, and every entry in `permissions` is
   a known scope token.
4. Permission keys from `adds_permissions` are reserved with `origin = <slug>`. Keys
   named in `consumes_permissions` must already exist.
5. Migrations run in a transaction, confined to the plugin's own table prefix.
6. The files are unpacked under the host's data directory and the plugin is loaded.

### Boot

Every boot, and after an install or a reload, the host opens each `.so`, looks up
`Plugin`, and calls:

1. `Manifest()`
2. `RegisterRoutes`, `RegisterMenu`, `RegisterWidgets`, `RegisterNodes`,
   `RegisterPermissions`
3. `RegisterAdapters`, when the plugin implements `AdapterContributor`
4. `Init(ctx, host)` with the host handles

The frontend then fetches the contribution tables from the loader endpoints and renders
what it is told.

### Uninstall

Routes, menu entries, widgets and nodes are withdrawn, the permission keys the plugin
reserved are released, and its files are removed. Tables created by its migrations are
the operator's decision, not the loader's.

## Security model

- Signature verification happens before anything is unpacked.
- Scope tokens in the manifest are checked against a known set, and a plugin asking for
  something outside it is refused at install.
- Credentials a plugin stores go through the host's AES-256-GCM encryption service; the
  plugin sees them decrypted at call time and never holds the key.
- The plugin router is mounted **last**, after every host route, so a plugin cannot
  shadow a core endpoint.
- There is no process sandbox. A plugin runs in the host process with the host's
  privileges; see [Architecture](../development/plugins/architecture.md).

## Where to go next

| You want to | Read |
|---|---|
| Find the plugin you need | [Catalog](./catalog.md) |
| Understand the runtime | [Architecture](../development/plugins/architecture.md) |
| Write one | [Developing a plugin](../development/plugins/developing.md) |
| Configure one you have | Its family README, then its own folder |
