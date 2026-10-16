## mcharbor plugin

Installed plugins and the plugin store

```
mcharbor plugin [flags]
```

### Options

```
  -h, --help   help for plugin
```

### Options inherited from parent commands

```
      --ca-file string     PEM root to trust for the server certificate ($MCHARBOR_CA_FILE)
      --confirm string     name the target of an irreversible change to confirm it non-interactively
      --context string     context to use instead of the current one ($MCHARBOR_CONTEXT)
      --env string         environment id for scoped commands ($MCHARBOR_ENV)
      --insecure           skip certificate verification; for labs only ($MCHARBOR_INSECURE=true)
      --json               shorthand for --output json
  -o, --output string      output format: table, json, yaml or tsv (default "table")
      --timeout duration   give up on a request after this long; 0 waits forever (default 1m0s)
      --url string         McHarbor URL, overriding the context ($MCHARBOR_URL)
      --yaml               shorthand for --output yaml
      --yes                skip the prompt for disruptive but reversible changes
```

### SEE ALSO

* [mcharbor](mcharbor.md)	 - Operate McHarbor from a terminal or a CI job
* [mcharbor plugin catalog](mcharbor_plugin_catalog.md)	 - List plugins available in the store
* [mcharbor plugin cli](mcharbor_plugin_cli.md)	 - The plugin-contributed command tree this context offers
* [mcharbor plugin dev](mcharbor_plugin_dev.md)	 - Author a plugin locally: scaffold, build, package, sign and validate
* [mcharbor plugin get](mcharbor_plugin_get.md)	 - Show a plugin
* [mcharbor plugin install](mcharbor_plugin_install.md)	 - Install a plugin from the catalog; its signature and checksum are verified by the server
* [mcharbor plugin list](mcharbor_plugin_list.md)	 - List installed plugins
* [mcharbor plugin reconcile](mcharbor_plugin_reconcile.md)	 - Pin a plugin to an exact catalog version: install, update or keep it
* [mcharbor plugin status](mcharbor_plugin_status.md)	 - What the plugin runtime loaded, and what it refused
* [mcharbor plugin uninstall](mcharbor_plugin_uninstall.md)	 - Uninstall a plugin; its settings are kept unless --purge-data
* [mcharbor plugin update](mcharbor_plugin_update.md)	 - Update an installed plugin to the catalog's version; its settings are kept

