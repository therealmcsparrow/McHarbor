## mcharbor plugin uninstall

Uninstall a plugin; its settings are kept unless --purge-data

### Synopsis

uninstall removes the plugin but keeps its settings and secrets, so installing it again restores them. --purge-data deletes those too and cannot be undone, so it needs the plugin named with --confirm.

```
mcharbor plugin uninstall <slug> [flags]
```

### Options

```
  -h, --help         help for uninstall
      --purge-data   also delete the plugin's settings and secrets (irreversible)
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

* [mcharbor plugin](mcharbor_plugin.md)	 - Installed plugins and the plugin store

