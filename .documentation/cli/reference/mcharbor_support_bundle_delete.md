## mcharbor support bundle delete

Delete a bundle and its archive (irreversible)

### Synopsis

delete removes the archive from the server. It cannot be undone, so --yes is not enough: pass --confirm <bundle-id> or type the id at the prompt. It needs support.manage.

```
mcharbor support bundle delete <bundle-id> [flags]
```

### Options

```
  -h, --help   help for delete
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

* [mcharbor support bundle](mcharbor_support_bundle.md)	 - Support bundles

