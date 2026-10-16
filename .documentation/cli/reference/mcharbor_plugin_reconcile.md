## mcharbor plugin reconcile

Pin a plugin to an exact catalog version: install, update or keep it

### Synopsis

reconcile brings one plugin to --version from the catalog: it installs it when absent, reinstalls it when another version is installed, and keeps it when it already matches, then sets whether it is enabled. It is how a runbook or a restore puts a plugin back exactly as recorded.

```
mcharbor plugin reconcile <slug> [flags]
```

### Options

```
      --allow-unsigned   accept a release no trusted key verifies (it runs with full host privileges)
      --disabled         leave the plugin installed but disabled
  -h, --help             help for reconcile
      --version string   exact catalog version to pin (required)
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

