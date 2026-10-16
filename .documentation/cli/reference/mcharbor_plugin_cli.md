## mcharbor plugin cli

The plugin-contributed command tree this context offers

```
mcharbor plugin cli [flags]
```

### Options

```
  -h, --help   help for cli
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
* [mcharbor plugin cli explain](mcharbor_plugin_cli_explain.md)	 - Show what one plugin command does and which route it calls
* [mcharbor plugin cli list](mcharbor_plugin_cli_list.md)	 - List the cached plugin commands and the cache's age
* [mcharbor plugin cli refresh](mcharbor_plugin_cli_refresh.md)	 - Fetch the plugin command spec now

