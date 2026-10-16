# Catalogs

**Settings → Integrations → Catalogs** · `?tab=catalogs`

The sources McHarbor reads app and plugin definitions from.

## Permissions

| Key | What it unlocks |
|---|---|
| `settings.manage` | Add, edit, test and delete catalogs; this section has no dedicated key of its own |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- Catalog sources per kind: **Apps**, **Plugins** and **Themes**.
- Whether a source is a remote index URL or a local directory.

## Built-in catalogs

Two rows are seeded on every install and marked **Built-in**: their name and URL cannot be
edited, only enabled or disabled.

| Name | Kind | URL |
|---|---|---|
| **McHarbor Apps** | Apps | `https://www.mcharbor.eu/catalog/apps/index.json` |
| **McHarbor Plugins** | Plugins | `https://www.mcharbor.eu/catalog/plugins/index.json` |

Each start rewrites a built-in row's name and URL by its fixed id and keeps its enabled
state, so an install that still pointed the app catalog at the old
`https://www.mcharbor.eu/addons/apps` location moves to `/catalog/apps/index.json` by itself.
A hand-added row already sitting on the new URL is absorbed first, so the repair cannot clash
with it.

A local plugin or theme catalog directory, when one is mounted and holds `.json` files, is
also registered automatically, as an ordinary row you can edit.

## Adding a catalog

A catalog is identified by its kind and URL together. Adding a URL that is already configured
for that kind is refused rather than reported as added: "This URL is already configured as
“&lt;name&gt;”. Enable that catalog instead of adding it again." A disabled row is the usual
reason a catalog seems to be missing.

## Plugin icons from a remote catalog

A remote plugin catalog names each icon by file name only. McHarbor reads it from
`icons/<slug>/<file>` beside the catalog's `index.json` and serves it through
`GET /api/appstore/plugins/{slug}/icon` (`plugins.view` or `plugins.manage`), so the
browser never loads it from the catalog host directly. Only SVG and PNG up to 256 KiB are
accepted. A fetched icon is cached for 24 hours, a missing one is remembered for an hour, and a
cached copy is still served while the catalog host is down. The dark-theme variant is
`<name>_dark.<ext>` when the catalog has one.

## Testing a catalog

**Test catalog** walks the source end to end and reports each step separately
(**Configuration**, **Transport**, **Reachability**, **Payload**, **Content**), each passing,
warning, failing or skipped, with a duration in milliseconds, ending in a verdict such as
"42 plugin entries found".

A per-step result is what makes a broken catalog diagnosable: reachability passing and
content failing is a different problem from transport failing, and the distinction is the
difference between fixing a URL and fixing a firewall.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Store](../../../screens/store/README.md): what these catalogs populate
- [Plugins](../../../plugins/README.md): the plugin side, including signing and trust keys
- [Getting started → Configuration](../../../getting-started/configuration/README.md): the catalog environment variables
