## mcharbor support bundle download

Download a bundle archive to a file

### Synopsis

download streams the archive to a file named by the server. It never writes the archive to a terminal; pass -f - to force stdout for a pipe. It refuses to overwrite an existing file without --force. It needs support.manage.

```
mcharbor support bundle download <bundle-id> [flags]
```

### Options

```
      --force                overwrite the destination file if it exists
  -h, --help                 help for download
  -f, --output-file string   file to write; defaults to the name the server sends, - for stdout
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

