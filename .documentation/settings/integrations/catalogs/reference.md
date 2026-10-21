# Catalogs: Reference

| Setting | Default | What it does |
|---|---|---|
| **Name** | — | Your label |
| **Kind** | — | Apps, Plugins or Themes |
| **Source URL or path** | — | An index URL (`https://example.com/index.json`) or a local directory inside the container |

Deleting a catalog: "*Community plugins* is removed and its entries disappear from the App
Store. **Installed items are not touched.**" What you lose is the ability to install more
from it, and to see updates it would have advertised.

`APPSTORE_CATALOG_URL`, `APPSTORE_SYNC_CRON` and `MCHARBOR_PLUGIN_CATALOG_DIRS` configure
catalogs from the environment instead.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| An App Store tab is empty | No catalog of that kind is configured | "No catalogs of this kind" is the explanation |
| A catalog will not load | One of the five test steps fails | Run **Test catalog**; it names which step |
| A built-in catalog cannot be edited | By design | "Built-in catalogs cannot be edited" |
| Adding a catalog is refused as already configured | That kind and URL already have a row, often disabled | Enable the row the message names instead of adding a second one |
| Plugin cards from a remote catalog show a generic icon | The catalog has no `icons/<slug>/<file>` beside its index, or the file is not SVG or PNG up to 256 KiB | Publish the icon there; a miss is retried after an hour |
| A catalog on `localhost` or `127.0.0.1` stopped loading | Catalog indexes may no longer be read from loopback | Serve it on a LAN address or a hostname; see [Which addresses McHarbor will fetch from](./README.md#which-addresses-mcharbor-will-fetch-from) |
| A catalog fails after redirecting from `https` to `http` | A redirect may not leave `https` | Publish the catalog at its final `https` URL |
| Removing a catalog broke an installed app | It did not; installs survive | Only future installs and update notices are affected |

## Related

- [Catalogs](./README.md): what the section is for
- [Settings](../../README.md): the other groups
