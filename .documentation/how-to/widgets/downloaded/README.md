# Ship a downloaded dashboard widget

A downloaded widget is a dashboard tile that arrives as a signed file from a local catalog, not
from the repository and not from a plugin. It runs as third-party UI code in every viewer's
browser, so nothing loads unless it carries a signature that verifies against a key the operator
already trusts. There is no unsigned override.

The operator side is on the Store's **Widgets** tab. This page is the contract for the person
publishing one.

## Catalog layout

Each local plugin catalog directory (`MCHARBOR_PLUGIN_CATALOG_DIRS`, the same directories the
plugin catalog reads) may carry a `widgets/` sub-directory. The plugin catalog reader only reads
top-level `*.json`, so the sub-directory never disturbs it.

```text
<catalog dir>/widgets/<key>.json      the entry
<catalog dir>/widgets/<bundle file>   the bundle the entry names
<catalog dir>/widgets/<bundle>.sig    optional signature sidecar
```

The entry is one JSON object:

| Field | Meaning |
|---|---|
| `key` | The widget id, lowercase letters, digits and `-`. A key that belongs to a built-in widget is refused (`err.widget.key_exists`). |
| `version` | Compared when installing over an installed copy. |
| `label`, `description` | Shown in the catalog and the widget picker. |
| `icon` | A Tabler component name from the host's icon registry. |
| `defaultSize`, `minSize` | Grid sizes as `{w, h}`. |
| `bundle` | Path of the bundle relative to `widgets/`. It must resolve inside that directory and be a regular file of at most 2 MiB. |
| `sha256` | Lowercase hex digest of the bundle. Written by the signer. |
| `signature` | Base64 ed25519 signature. May instead be in `<bundle>.sig`. |
| `translations` | Per language, nested strings the widget uses. A language key that is not valid is skipped. |

## The bundle

A browser script that sets a global and nothing else. The global is `McharborWidget_<key>` with
every `-` in the key written as `_`, and it is the same `{ mount, unmount, update }` object a plugin
widget exports, so the mounting contract is the one in
[Build a dashboard widget](../README.md#the-mount-contract). The host loads it with a script tag from
`/api/widgets/{key}/bundle`, on McHarbor's own origin. The bundle carries its own React.

Widgets are listed in the picker's **Plugins** category as `downloaded:<key>`. A bundle that
does not export `mount`, `unmount` and `update` fails to load.

## What the signature covers

The signature is an ed25519 signature over a manifest, not over the bundle bytes. The format is in
`core/widgetsig` and is the same in the verifier and the signer:

```text
mcharbor-widget-v1
<key>
<version>
<sha256 hex of the bundle>
<sha256 hex of the metadata>
```

Joined with newlines. The metadata is the canonical JSON of `label`, `description`, `icon`,
`defaultSize`, `minSize` and `translations`, in that field order, with map keys sorted, no HTML
escaping and no trailing newline, and an absent `translations` written as `{}`. Its digest is the
last line.

Binding the key and version stops a signed bundle being installed under another key or rolled back.
Binding the metadata stops an unsigned label, icon or translation riding along with a signed
bundle. The version prefix `mcharbor-widget-v1` means a signature made for one layout can never be
read under another. A signature made over only the bundle (`pluginsigner -file`) is refused.

## Signing

```bash
go run ./cmd/pluginsigner -widget <widgets dir | widgets/<key>.json> -key <seed file> [-trust <public key>]
```

`-widget` signs one entry or every entry in a directory, writes `sha256` and `signature`, and with
`-trust` verifies each signature after writing it. Use the same ed25519 key pair the plugin
catalog uses.

The host trusts the keys in `MCHARBOR_PLUGIN_TRUST_KEYS`, then `$DATA_DIR/plugin-catalog/signing-key.pub`.
One key and one setting cover plugins and widgets. No trust key means no widget can be installed.

## Installing and removing

| Route | Key | Audit |
|---|---|---|
| `GET /api/widgets/catalog` | `store_widgets.view` or `widgets.view` | none (read) |
| `POST /api/widgets/catalog/{key}/install` | `dashboard_widgets.install` | `install`, entity `widget`, severity warning |
| `DELETE /api/widgets/{key}` | `dashboard_widgets.install` | `uninstall`, entity `widget` |
| `GET /api/widgets/{key}/bundle` | `store_widgets.view` or `widgets.view` | none (read) |

`dashboard_widgets.install` is global and is held by the administrator role only, because
installing third-party UI code is a trust decision. No preset migration grants it.

The catalog lists each entry with `signed` (verifies against a trust key) and `installed`. On the
Store's **Widgets** tab, the section **Widget catalog** shows an entry that does not verify as
**Not signed by a trusted key**, and its **Install** button is disabled. An installed entry offers
**Update** when the catalog has a newer version, and **Remove**. Both buttons render disabled
without `dashboard_widgets.install`.

Install does this, server-side, before anything is written:

1. Read the bundle, confined to `widgets/` and capped at 2 MiB.
2. Check `sha256` when present.
3. Verify the manifest signature against the trust keys.
4. Refuse an entry older than the installed version (`err.widget.downgrade`, `409`). The service
   can be told to allow it, and no route or control does.
5. Write `$DATA_DIR/widgets/<key>/` (`definition.json`, `bundle.js`, `bundle.js.sig`,
   `i18n/<lang>.json`) through a staging directory and a rename.

Refusals answer `err.widget.signature_invalid`, `err.widget.checksum_mismatch` or
`err.widget.catalog_entry_not_found`.

## What stays verified after install

The installed copy is verified from `definition.json`, the translation files and the bundle
together, and it is verified again on **every** request for the bundle. Editing the label or a
translation on disk, or the bundle, un-verifies it. The browser trusts only the server's
`verified` flag, and a definition that is not verified gets no script tag. The bundle route also
hashes the exact bytes it streams, so a file swapped after verification is not served.

**Remove** on an installed widget asks for confirmation, then removes it from the widget list and from
every dashboard that shows it.

## Related

- [Build a dashboard widget](../README.md)
- [Store](../../../screens/store/README.md)
- [Plugin packaging and signing](../../plugin/packaging.md)
