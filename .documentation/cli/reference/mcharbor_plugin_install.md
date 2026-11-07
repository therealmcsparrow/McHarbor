## mcharbor plugin install

Install a plugin from the catalog; its signature and checksum are verified by the server

### Synopsis

install fetches the catalog entry and has the server install it. A release no trusted key verifies is refused unless --allow-unsigned is passed, because an unsigned plugin runs as native code with the same host privileges as McHarbor.

```
mcharbor plugin install <slug> [flags]
```

### Options

```
      --allow-unsigned   install even when no trusted key verifies the release (it runs with full host privileges)
  -h, --help             help for install
      --version string   catalog version to install; the newest when omitted
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

